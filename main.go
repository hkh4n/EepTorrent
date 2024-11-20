package main

/*
A cross-platform I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"eeptorrent/lib/util/logo"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	log          = logrus.StandardLogger()
	maxRetries   = 20
	initialDelay = 2 * time.Second
)

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetLevel(logrus.DebugLevel)
}

// Torrent represents a single torrent download.
type Torrent struct {
	Mi                *metainfo.MetaInfo
	Dm                *download.DownloadManager
	CancelFunc        context.CancelFunc
	Stats             *download.DownloadStats
	Progress          float64
	Status            string
	Speed             float64
	Name              string
	DownloadedBytes   int64
	DownloadSpeedKBps float64
	IsDownloading     bool
}

func main() {
	// Initialize the Fyne application
	myApp := app.New()
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	// Show disclaimer
	showDisclaimer(myApp, myWindow)

	// Variables to manage downloads
	var torrents []*Torrent
	var torrentsMutex sync.Mutex

	// Create the settings form
	downloadDirEntry := widget.NewEntry()
	downloadDirEntry.SetPlaceHolder("Select download directory")
	downloadDirButton := widget.NewButton("Browse", func() {
		dirDialog := dialog.NewFolderOpen(func(list fyne.ListableURI, err error) {
			if err == nil && list != nil {
				downloadDirEntry.SetText(list.Path())
			}
		}, myWindow)
		dirDialog.Show()
	})

	maxConnectionsEntry := widget.NewEntry()
	maxConnectionsEntry.SetText("50") // Default value

	portEntry := widget.NewEntry()
	portEntry.SetText("6881") // Default port

	uploadLimitEntry := widget.NewEntry()
	uploadLimitEntry.SetText("0") // 0 means unlimited

	downloadLimitEntry := widget.NewEntry()
	downloadLimitEntry.SetText("0") // 0 means unlimited

	loggingLevelSelect := widget.NewSelect([]string{"Debug", "Info", "Warning", "Error", "Fatal", "Panic"}, func(value string) {
		// Adjust log level based on selection
		switch value {
		case "Debug":
			log.SetLevel(logrus.DebugLevel)
		case "Info":
			log.SetLevel(logrus.InfoLevel)
		case "Warning":
			log.SetLevel(logrus.WarnLevel)
		case "Error":
			log.SetLevel(logrus.ErrorLevel)
		case "Fatal":
			log.SetLevel(logrus.FatalLevel)
		case "Panic":
			log.SetLevel(logrus.PanicLevel)
		}
	})
	loggingLevelSelect.SetSelected("Debug")

	// Assemble the settings form
	settingsForm := widget.NewForm(
		widget.NewFormItem("Download Directory", container.NewHBox(downloadDirEntry, downloadDirButton)),
		widget.NewFormItem("Max Connections", maxConnectionsEntry),
		widget.NewFormItem("Listening Port", portEntry),
		widget.NewFormItem("Upload Limit (kB/s)", uploadLimitEntry),
		widget.NewFormItem("Download Limit (kB/s)", downloadLimitEntry),
		widget.NewFormItem("Logging Level", loggingLevelSelect),
	)

	// Create the side menu (settings)
	sideMenu := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		settingsForm,
	)

	// Create UI components for the main content
	statusLabel := widget.NewLabel("Ready")
	addTorrentButton := widget.NewButton("Add Torrent", nil)

	// Variables for UI components that need to be accessed elsewhere
	var torrentList *widget.List

	// AddTorrentButton handler
	addTorrentButton.OnTapped = func() {
		// Open file dialog to select torrent file
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			torrentFilePath := reader.URI().Path()
			reader.Close()

			// Load the torrent file
			mi, err := metainfo.LoadFromFile(torrentFilePath)
			if err != nil {
				showError("Failed to load torrent", err, myWindow)
				return
			}

			info, err := mi.Info()
			if err != nil {
				showError("Failed to parse torrent info", err, myWindow)
				return
			}

			// Create a new Torrent struct
			torrent := &Torrent{
				Mi:     &mi,
				Name:   info.Name,
				Status: "Ready",
			}

			// Add the torrent to the list
			torrentsMutex.Lock()
			torrents = append(torrents, torrent)
			torrentsMutex.Unlock()
			torrentList.Refresh()
		}, myWindow)
	}

	// Create a list widget to display the torrents
	torrentList = widget.NewList(
		func() int {
			torrentsMutex.Lock()
			defer torrentsMutex.Unlock()
			return len(torrents)
		},
		func() fyne.CanvasObject {
			// Template for list items
			return container.NewHBox(
				widget.NewLabel("Name"),
				widget.NewProgressBar(),
				widget.NewLabel("Status"),
				widget.NewLabel("Speed"),
				widget.NewButton("Start", nil),
				widget.NewButton("Stop", nil),
				widget.NewButton("Remove", nil),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			torrentsMutex.Lock()
			defer torrentsMutex.Unlock()
			if i >= len(torrents) {
				return
			}
			torrent := torrents[i]
			hbox := o.(*fyne.Container)
			nameLabel := hbox.Objects[0].(*widget.Label)
			progressBar := hbox.Objects[1].(*widget.ProgressBar)
			statusLabel := hbox.Objects[2].(*widget.Label)
			speedLabel := hbox.Objects[3].(*widget.Label)
			startButton := hbox.Objects[4].(*widget.Button)
			stopButton := hbox.Objects[5].(*widget.Button)
			removeButton := hbox.Objects[6].(*widget.Button)

			nameLabel.SetText(torrent.Name)
			progressBar.SetValue(torrent.Progress)
			statusLabel.SetText(torrent.Status)
			speedLabel.SetText(fmt.Sprintf("%.2f KB/s", torrent.Speed))

			if torrent.IsDownloading {
				startButton.Disable()
				stopButton.Enable()
			} else {
				startButton.Enable()
				stopButton.Disable()
			}

			startButton.OnTapped = func() {
				startTorrentDownload(torrent, torrentList, downloadDirEntry, maxConnectionsEntry, myWindow)
			}

			stopButton.OnTapped = func() {
				stopTorrentDownload(torrent)
			}

			removeButton.OnTapped = func() {
				removeTorrent(torrent, &torrents, &torrentsMutex, torrentList)
			}
		},
	)

	// Periodically refresh the list to update progress
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for range ticker.C {
			torrentList.Refresh()
		}
	}()

	// Create the main content area
	mainContent := container.NewBorder(
		container.NewHBox(addTorrentButton),
		statusLabel,
		nil,
		nil,
		torrentList,
	)

	// Layout the UI components with side menu
	content := container.NewHSplit(sideMenu, mainContent)
	content.Offset = 0.3 // Adjust the split ratio

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(1000, 600))

	// Add a menu bar
	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Add Torrent...", func() {
				// Trigger the addTorrentButton action
				addTorrentButton.OnTapped()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() {
				myApp.Quit()
			}),
		),
		fyne.NewMenu("Edit",
			fyne.NewMenuItem("Preferences", func() {
				// Focus on the settings side menu or open a preferences dialog
				// For now, we'll do nothing as settings are always visible
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				showAboutDialog(myApp, myWindow)
			}),
		),
	)
	myWindow.SetMainMenu(menu)

	// Initialize SAM
	err := i2p.InitSAM()
	if err != nil {
		showError("Failed to initialize SAM", err, myWindow)
		return
	}
	defer i2p.CloseSAM()

	// Show the window and start the GUI event loop
	myWindow.ShowAndRun()
}

// Function to start downloading a torrent
func startTorrentDownload(torrent *Torrent, torrentList *widget.List, downloadDirEntry, maxConnectionsEntry *widget.Entry, parent fyne.Window) {
	// Check if already downloading
	if torrent.IsDownloading {
		return
	}

	torrent.IsDownloading = true
	torrent.Status = "Starting..."

	// Validate and apply settings
	downloadDir := downloadDirEntry.Text
	if downloadDir == "" {
		torrent.Status = "Error"
		showError("Invalid Settings", fmt.Errorf("Please select a download directory"), parent)
		return
	}

	maxConnections, err := strconv.Atoi(maxConnectionsEntry.Text)
	if err != nil || maxConnections <= 0 {
		torrent.Status = "Error"
		showError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), parent)
		return
	}

	// Initialize download stats
	stats := download.NewDownloadStats()
	torrent.Stats = stats

	info, err := torrent.Mi.Info()
	if err != nil {
		torrent.Status = "Error"
		showError("Failed to parse torrent info", err, parent)
		return
	}

	// Initialize the file writer
	var outputPath string
	var mode os.FileMode
	if len(info.Files) == 0 {
		// Single-file torrent
		outputPath = filepath.Join(downloadDir, info.Name)
		mode = 0644
	} else {
		// Multi-file torrent
		outputPath = filepath.Join(downloadDir, info.Name)
		mode = 0755
		// Create the directory if it doesn't exist
		err := os.MkdirAll(outputPath, mode)
		if err != nil && !os.IsExist(err) {
			torrent.Status = "Error"
			showError("Failed to create output directory", err, parent)
			return
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)
	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, len(info.Pieces))
	dm.DownloadDir = downloadDir
	torrent.Dm = dm

	progressTicker := time.NewTicker(1 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	torrent.CancelFunc = cancel

	// Goroutine to update progress and speed
	go func() {
		defer progressTicker.Stop()
		var prevDownloaded int64 = 0
		for {
			select {
			case <-progressTicker.C:
				dm.LogProgress()
				torrent.Progress = dm.Progress() / 100

				// Safely load the total downloaded bytes
				currentDownloaded := atomic.LoadInt64(&dm.Downloaded)
				bytesDownloaded := currentDownloaded - prevDownloaded
				prevDownloaded = currentDownloaded

				downloadSpeedKBps := float64(bytesDownloaded) / 1024 // KB/s
				torrent.Speed = downloadSpeedKBps
				torrent.Status = fmt.Sprintf("Downloading (%.2f%%)", dm.Progress())
				// Update GUI
				torrentList.Refresh()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start the download
	go func() {
		defer func() {
			torrent.IsDownloading = false
			if dm.IsFinished() {
				torrent.Status = "Completed"
			} else {
				torrent.Status = "Stopped"
			}
			torrentList.Refresh()
		}()

		// Get peers from tracker
		peers, err := tracker.GetPeersFromSimpTracker(torrent.Mi)
		if err != nil {
			torrent.Status = "Error"
			showError("Failed to get peers from tracker", err, parent)
			return
		}

		// Check if no peers were returned
		if len(peers) == 0 {
			torrent.Status = "No Peers Available"
			torrentList.Refresh()
			dialog.ShowInformation("No Peers Available", "The tracker returned zero peers. The download cannot proceed at this time.", parent)
			return
		}

		// Limit the number of connections based on user settings
		maxPeers := maxConnections
		if len(peers) < maxPeers {
			maxPeers = len(peers)
		}

		var wg sync.WaitGroup
		for i := 0; i < maxPeers; i++ {
			wg.Add(1)
			go func(peerHash []byte, index int) {
				defer wg.Done()
				stats.ConnectionStarted()
				defer stats.ConnectionEnded()
				retryConnect(ctx, peerHash, index, torrent.Mi, dm, maxRetries, initialDelay)
			}(peers[i], i)
		}

		wg.Wait()

		// Implement periodic tracker updates
		// Check if any data has been downloaded or if any peers are connected
		if dm.Downloaded == 0 {
			torrent.Status = "Failed to Connect to Peers"
			torrentList.Refresh()
			dialog.ShowInformation("Failed to Download", "Unable to connect to any peers. The download cannot proceed.", parent)
			return
		}

		// Only proceed with periodic tracker updates if initial connections were successful
		trackerInterval := 30 // Interval in seconds for tracker updates
		trackerTicker := time.NewTicker(time.Duration(trackerInterval) * time.Second)
		defer trackerTicker.Stop()

		for {
			select {
			case <-trackerTicker.C:
				if !torrent.IsDownloading {
					return
				}
				// Update peers from tracker
				newPeers, err := tracker.GetPeersFromSimpTracker(torrent.Mi)
				if err != nil {
					log.WithError(err).Error("Failed to update peers from tracker")
					continue
				}
				// Connect to new peers
				for _, peerHash := range newPeers {
					// Check if already connected to this peer
					// Implement logic to avoid duplicate connections if necessary
					wg.Add(1)
					go func(peerHash []byte) {
						defer wg.Done()
						stats.ConnectionStarted()
						defer stats.ConnectionEnded()
						retryConnect(ctx, peerHash, -1, torrent.Mi, dm, maxRetries, initialDelay)
					}(peerHash)
				}
			case <-ctx.Done():
				return
			}

			// Check if download is finished
			if dm.IsFinished() {
				return
			}
		}
	}()
}

// Function to stop downloading a torrent
func stopTorrentDownload(torrent *Torrent) {
	if !torrent.IsDownloading {
		return
	}
	torrent.Status = "Stopping..."
	if torrent.CancelFunc != nil {
		torrent.CancelFunc()
	}
}

// Function to remove a torrent
func removeTorrent(torrent *Torrent, torrents *[]*Torrent, torrentsMutex *sync.Mutex, torrentList *widget.List) {
	stopTorrentDownload(torrent)
	torrentsMutex.Lock()
	defer torrentsMutex.Unlock()
	for i, t := range *torrents {
		if t == torrent {
			*torrents = append((*torrents)[:i], (*torrents)[i+1:]...)
			break
		}
	}
	torrentList.Refresh()
}

// Helper function to display errors
func showError(title string, err error, parent fyne.Window) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), parent)
}

// Helper function to show the About dialog
func showAboutDialog(app fyne.App, parent fyne.Window) {
	// Adjust as per your versioning system
	version := "0.0.1"
	dialog.ShowCustom("About EepTorrent", "Close",
		container.NewVBox(
			widget.NewLabelWithStyle("EepTorrent", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel(fmt.Sprintf("Version: %s", version)),
			widget.NewLabel("A cross-platform I2P-only BitTorrent client."),
			widget.NewLabel("© 2024 Haris Khan"),
		), parent)
}

// retryConnect attempts to connect to a peer with retry logic.
func retryConnect(ctx context.Context, peerHash []byte, index int, mi *metainfo.MetaInfo, dm *download.DownloadManager, maxRetries int, initialDelay time.Duration) {
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, stopping retries for peer %d", index)
			return
		default:
		}

		err := peer.ConnectToPeer(ctx, peerHash, index, mi, dm)
		if err == nil {
			log.Infof("Successfully connected to peer %d on attempt %d", index, attempt)
			return
		}

		log.Errorf("Attempt %d to connect to peer %d failed: %v", attempt, index, err)

		if attempt < maxRetries {
			log.Infof("Retrying to connect to peer %d after %v...", index, delay)
			select {
			case <-ctx.Done():
				log.Infof("Context cancelled during delay, stopping retries for peer %d", index)
				return
			case <-time.After(delay):
				delay *= 2
				if delay > 60*time.Second {
					delay = 60 * time.Second
				}
			}
		} else {
			log.Errorf("Exceeded maximum retries (%d) for peer %d", maxRetries, index)
			// Optionally, you can update the torrent status here
		}
	}
}

func showDisclaimer(app fyne.App, parent fyne.Window) {
	// Create the content for the disclaimer
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It may have bugs and other issues.\nCheck back for updates!"),
		widget.NewLabel("EepTorrent Copyright (C) 2024 Haris Khan\nThis program comes with ABSOLUTELY NO WARRANTY.\nThis is free software, and you are welcome to redistribute it under certain conditions. See COPYING for details."),
	)

	// Create the custom confirmation dialog
	dialog := dialog.NewCustomConfirm(
		"Experimental Software",
		"Accept",
		"Decline",
		disclaimerContent,
		func(accepted bool) {
			if !accepted {
				// User declined the disclaimer; exit the application
				app.Quit()
			}
			// If accepted, do nothing and allow the application to continue
		},
		parent,
	)

	// Make the dialog modal (prevents interaction with other windows until closed)
	dialog.SetDismissText("Decline")
	dialog.Show()
}

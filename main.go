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
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"eeptorrent/lib/util"
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
	// Declare samInitOnce at package level
	samInitOnce sync.Once
)

func init() {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetLevel(logrus.DebugLevel)
}

// Torrent represents a single torrent download
type Torrent struct {
	Name            string
	FilePath        string
	Progress        float64
	Status          string
	DownloadSpeed   float64
	DownloadManager *download.DownloadManager
	CancelFunc      context.CancelFunc
}

func main() {
	// Initialize the Fyne application
	myApp := app.New()
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	// Variables to manage download state
	var torrents []*Torrent
	var torrentsMutex sync.Mutex

	// Create UI components for settings
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
		widget.NewFormItem("Logging Level", loggingLevelSelect),
	)
	settingsForm.Resize(fyne.NewSize(600, settingsForm.Size().Height))

	// Create the side menu (settings)
	sideMenu := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		settingsForm,
	)
	var torrentList *widget.List
	// Create the torrent list
	torrentList = widget.NewList(
		func() int {
			torrentsMutex.Lock()
			defer torrentsMutex.Unlock()
			return len(torrents)
		},
		func() fyne.CanvasObject {
			// Create a container for each item
			nameLabel := widget.NewLabel("Name")
			progressBar := widget.NewProgressBar()
			statusLabel := widget.NewLabel("Status")
			speedLabel := widget.NewLabel("Speed")
			startButton := widget.NewButton("Start", nil)
			pauseButton := widget.NewButton("Pause", nil)
			removeButton := widget.NewButton("Remove", nil)
			itemContainer := container.NewHBox(
				nameLabel,
				progressBar,
				statusLabel,
				speedLabel,
				startButton,
				pauseButton,
				removeButton,
			)
			return itemContainer
		},
		func(i int, o fyne.CanvasObject) {
			torrentsMutex.Lock()
			defer torrentsMutex.Unlock()
			if i >= len(torrents) {
				return
			}
			torrent := torrents[i]
			itemContainer := o.(*fyne.Container)
			nameLabel := itemContainer.Objects[0].(*widget.Label)
			progressBar := itemContainer.Objects[1].(*widget.ProgressBar)
			statusLabel := itemContainer.Objects[2].(*widget.Label)
			speedLabel := itemContainer.Objects[3].(*widget.Label)
			startButton := itemContainer.Objects[4].(*widget.Button)
			pauseButton := itemContainer.Objects[5].(*widget.Button)
			removeButton := itemContainer.Objects[6].(*widget.Button)

			nameLabel.SetText(torrent.Name)
			progressBar.SetValue(torrent.Progress / 100)
			statusLabel.SetText(torrent.Status)
			speedLabel.SetText(fmt.Sprintf("%.2f KB/s", torrent.DownloadSpeed))

			// Set button actions
			startButton.OnTapped = func() {
				go startTorrentDownload(torrent, myWindow, downloadDirEntry.Text, maxConnectionsEntry.Text, torrentList)
			}
			pauseButton.OnTapped = func() {
				if torrent.CancelFunc != nil {
					torrent.CancelFunc()
					torrent.Status = "Paused"
					torrentList.Refresh()
				}
			}
			removeButton.OnTapped = func() {
				if torrent.CancelFunc != nil {
					torrent.CancelFunc()
				}
				torrentsMutex.Lock()
				if i < len(torrents) {
					torrents = append(torrents[:i], torrents[i+1:]...)
				}
				torrentsMutex.Unlock()
				torrentList.Refresh()
			}
		},
	)

	// Add "Add Torrent" button
	addTorrentButton := widget.NewButton("Add Torrent", func() {
		// Open file dialog to select torrent file
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}

			torrentFilePath := reader.URI().Path()
			reader.Close()

			// Create a new Torrent instance
			torrent := &Torrent{
				Name:     filepath.Base(torrentFilePath),
				FilePath: torrentFilePath,
				Status:   "Initialized",
			}

			torrentsMutex.Lock()
			torrents = append(torrents, torrent)
			torrentsMutex.Unlock()
			torrentList.Refresh()
		}, myWindow)
	})

	// Create the main content area
	mainContent := container.NewBorder(
		container.NewHBox(addTorrentButton),
		nil,
		nil,
		nil,
		torrentList,
	)

	// Layout the UI components with side menu
	content := container.NewBorder(nil, nil, sideMenu, nil, mainContent)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))

	// Add a menu bar
	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Add Torrent", func() {
				addTorrentButton.OnTapped()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() {
				myApp.Quit()
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				showAboutDialog(myApp, myWindow)
			}),
		),
	)
	myWindow.SetMainMenu(menu)

	showDisclaimer(myApp, myWindow)

	myWindow.ShowAndRun()
}

func startTorrentDownload(torrent *Torrent, myWindow fyne.Window, downloadDir string, maxConnectionsStr string, torrentList *widget.List) {
	if torrent.Status == "Downloading" {
		return
	}

	// Validate and apply settings
	if downloadDir == "" {
		showError("Invalid Settings", fmt.Errorf("Please select a download directory"), myWindow)
		return
	}

	maxConnections, err := strconv.Atoi(maxConnectionsStr)
	if err != nil || maxConnections <= 0 {
		showError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), myWindow)
		return
	}

	// Initialize download stats
	stats := download.NewDownloadStats()

	// Initialize SAM if not already initialized
	var samInitError error
	samInitOnce.Do(func() {
		err := i2p.InitSAM()
		if err != nil {
			samInitError = err
			return
		}
	})
	if samInitError != nil {
		showError("Failed to initialize SAM", samInitError, myWindow)
		return
	}

	// Load the torrent file
	mi, err := metainfo.LoadFromFile(torrent.FilePath)
	if err != nil {
		showError("Failed to load torrent", err, myWindow)
		return
	}

	info, err := mi.Info()
	if err != nil {
		showError("Failed to parse torrent info", err, myWindow)
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
			showError("Failed to create output directory", err, myWindow)
			return
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)
	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, len(info.Pieces))
	dm.DownloadDir = downloadDir
	torrent.DownloadManager = dm

	// Update torrent status
	torrent.Status = "Downloading"
	torrentList.Refresh()

	progressTicker := time.NewTicker(1 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	torrent.CancelFunc = cancel

	go func() {
		defer progressTicker.Stop()
		var prevDownloaded int64 = 0
		for {
			select {
			case <-progressTicker.C:
				dm.LogProgress()
				progress := dm.Progress()
				torrent.Progress = progress

				// Update GUI elements
				currentDownloaded := atomic.LoadInt64(&dm.Downloaded)
				bytesDownloaded := currentDownloaded - prevDownloaded
				prevDownloaded = currentDownloaded

				downloadSpeedKBps := float64(bytesDownloaded) / 1024 // KB/s
				torrent.DownloadSpeed = downloadSpeedKBps

				torrentList.Refresh()

			case <-ctx.Done():
				if dm.IsFinished() {
					torrent.Status = "Completed"
				} else {
					torrent.Status = "Stopped"
				}
				torrentList.Refresh()
				return
			}
		}
	}()

	// Get peers from trackers
	var allPeers [][]byte

	peersPostman, err := tracker.GetPeersFromPostmanTracker(&mi)
	if err != nil {
		log.WithError(err).Warn("Failed to get peers from Postman Tracker")
	} else {
		allPeers = append(allPeers, peersPostman...)
	}

	peersSimp, err := tracker.GetPeersFromSimpTracker(&mi)
	if err != nil {
		log.WithError(err).Warn("Failed to get peers from Simp Tracker")
	} else {
		allPeers = append(allPeers, peersSimp...)
	}

	peersDg2, err := tracker.GetPeersFromDg2Tracker(&mi)
	if err != nil {
		log.WithError(err).Warn("Failed to get peers from Dg2 Tracker")
	} else {
		allPeers = append(allPeers, peersDg2...)
	}

	if len(allPeers) == 0 {
		showError("Failed to get peers from any tracker", fmt.Errorf("No peers found"), myWindow)
		torrent.Status = "Error"
		torrentList.Refresh()
		cancel()
		return
	}
	uniquePeers := removeDuplicatePeers(allPeers)

	// Start connecting to peers
	var wg sync.WaitGroup
	maxPeers := maxConnections
	if len(uniquePeers) < maxPeers {
		maxPeers = len(uniquePeers)
	}

	for i := 0; i < maxPeers; i++ {
		wg.Add(1)
		go func(peerHash []byte, index int) {
			defer wg.Done()
			stats.ConnectionStarted()
			defer stats.ConnectionEnded()
			retryConnect(ctx, peerHash, index, &mi, dm, maxRetries, initialDelay)
		}(uniquePeers[i], i)
	}

	wg.Wait()

	cancel()

	if dm.IsFinished() {
		torrent.Status = "Completed"
		dialog.ShowInformation("Download Complete", fmt.Sprintf("Downloaded %s successfully.", info.Name), myWindow)
	} else {
		torrent.Status = "Incomplete"
		dialog.ShowInformation("Download Incomplete", "The download did not complete successfully.", myWindow)
	}

	torrentList.Refresh()
}

func showError(title string, err error, parent fyne.Window) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), parent)
}

func showAboutDialog(app fyne.App, parent fyne.Window) {
	gitCommitDisplay := util.GitCommit
	dialog.ShowCustom("About EepTorrent", "Close",
		container.NewVBox(
			widget.NewLabelWithStyle("EepTorrent", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel(fmt.Sprintf("Version: %s-%s", util.Version, gitCommitDisplay)),
			widget.NewLabel("A cross-platform I2P-only BitTorrent client."),
			widget.NewLabel("© 2024 Haris Khan"),
		), parent)
}

// retryConnect attempts to connect to a peer with retry logic.
// maxRetries: Maximum number of retry attempts.
// initialDelay: Initial delay before the first retry.
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
				// Exponential backoff: double the delay for the next attempt
				delay *= 2
				if delay > 60*time.Second {
					delay = 60 * time.Second // Cap the delay to 60 seconds
				}
			}
		}
	}

	log.Errorf("Exceeded maximum retries (%d) for peer %d", maxRetries, index)
}

func removeDuplicatePeers(peers [][]byte) [][]byte {
	peerSet := make(map[string]struct{})
	uniquePeers := make([][]byte, 0, len(peers))

	for _, peer := range peers {
		peerStr := string(peer)
		if _, exists := peerSet[peerStr]; !exists {
			peerSet[peerStr] = struct{}{}
			uniquePeers = append(uniquePeers, peer)
		}
	}
	return uniquePeers
}

func showDisclaimer(app fyne.App, parent fyne.Window) {
	// Create the content for the disclaimer
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It may have bugs, faulty GUIs, and other issues.\nHowever, it will be updated frequently. Check back for updates!"),
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

	dialog.SetDismissText("Decline")
	dialog.Show()
}

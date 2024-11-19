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
	"eeptorrent/lib/util"
	"eeptorrent/lib/util/logo"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/sirupsen/logrus"
	"image/color"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-i2p/go-i2p-bt/metainfo"
	"os"
)

/*
	//Trackers

"ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p //tracker2.postman.i2p
"w7tpbzncbcocrqtwwm3nezhnnsw4ozadvi2hmvzdhrqzfxfum7wa.b32.i2p // opentracker.dg2.i2p
"afuuortfaqejkesne272krqvmafn65mhls6nvcwv3t7l2ic2p4kq.b32.i2p // lyoko.i2p
"s5ikrdyjwbcgxmqetxb3nyheizftms7euacuub2hic7defkh3xhq.b32.i2p // tracker.thebland.i2p
"nfrjvknwcw47itotkzmk6mdlxmxfxsxhbhlr5ozhlsuavcogv4hq.b32.i2p // torrfreedom.i2p
*/
var (
	log          = logrus.New()
	maxRetries   = 5
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

func main() {
	// Initialize the Fyne application
	myApp := app.New()
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	//Set background
	// Create the background image
	background := canvas.NewImageFromResource(logo.ResourceLogoPng)
	background.FillMode = canvas.ImageFillContain // Adjust as needed: FillOriginal, FillContain, FillFill, FillStretch

	// Optionally, set a semi-transparent overlay if needed
	overlay := canvas.NewRectangle(color.NRGBA{0, 0, 0, 100}) // Semi-transparent black
	overlay.SetMinSize(myWindow.Canvas().Size())
	// Show disclaimer
	showDisclaimer(myApp, myWindow)

	// Create UI components for the main content
	progressBar := widget.NewProgressBar()
	statusLabel := widget.NewLabel("Ready")
	startButton := widget.NewButton("Start Download", nil)
	stopButton := widget.NewButton("Stop Download", nil)
	stopButton.Disable()

	// Variables to manage download state
	var dm *download.DownloadManager
	var wg sync.WaitGroup
	var downloadInProgress bool
	var downloadCancel context.CancelFunc

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
	loggingLevelSelect.SetSelected("Info")

	// Assemble the settings form
	settingsForm := widget.NewForm(
		widget.NewFormItem("Download Directory", container.NewHBox(downloadDirEntry, downloadDirButton)),
		widget.NewFormItem("Max Connections", maxConnectionsEntry),
		widget.NewFormItem("Listening Port", portEntry),
		widget.NewFormItem("Upload Limit (kB/s)", uploadLimitEntry),
		widget.NewFormItem("Download Limit (kB/s)", downloadLimitEntry),
		widget.NewFormItem("Logging Level", loggingLevelSelect),
	)
	settingsForm.Resize(fyne.NewSize(600, settingsForm.Size().Height))

	// Create the side menu (settings)
	sideMenu := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		settingsForm,
	)

	downloadSpeedLabel := widget.NewLabel("Speed: 0 KB/s")

	// Create the main content area
	mainContent := container.NewVBox(
		progressBar,
		downloadSpeedLabel,
		statusLabel,
		container.NewHBox(startButton, stopButton),
	)

	// Layout the UI components with side menu
	content := container.NewBorder(nil, nil, sideMenu, nil, mainContent)
	/*
		content := container.NewStack(
			background,
			sideMenu,
			mainContent, // Your main UI components
		)

	*/

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))

	// Add a menu bar
	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Torrent File...", func() {
				// Trigger the start button action
				startButton.OnTapped()
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

	// Start button handler
	startButton.OnTapped = func() {
		if downloadInProgress {
			return
		}

		// Validate and apply settings
		downloadDir := downloadDirEntry.Text
		if downloadDir == "" {
			showError("Invalid Settings", fmt.Errorf("Please select a download directory"), myWindow)
			return
		}

		maxConnections, err := strconv.Atoi(maxConnectionsEntry.Text)
		if err != nil || maxConnections <= 0 {
			showError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), myWindow)
			return
		}

		port, err := strconv.Atoi(portEntry.Text)
		if err != nil || port <= 0 || port > 65535 {
			showError("Invalid Settings", fmt.Errorf("Port must be a valid TCP port number"), myWindow)
			return
		}

		uploadLimit, err := strconv.Atoi(uploadLimitEntry.Text)
		if err != nil || uploadLimit < 0 {
			showError("Invalid Settings", fmt.Errorf("Upload Limit must be 0 or a positive integer"), myWindow)
			return
		}

		downloadLimit, err := strconv.Atoi(downloadLimitEntry.Text)
		if err != nil || downloadLimit < 0 {
			showError("Invalid Settings", fmt.Errorf("Download Limit must be 0 or a positive integer"), myWindow)
			return
		}

		// Open file dialog to select torrent file
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}

			torrentFilePath := reader.URI().Path()
			reader.Close()

			downloadInProgress = true
			startButton.Disable()
			stopButton.Enable()
			statusLabel.SetText("Initializing download...")

			// Run the download in a separate goroutine
			go func() {
				defer func() {
					downloadInProgress = false
					startButton.Enable()
					stopButton.Disable()
					statusLabel.SetText("Download completed")
				}()

				// Initialize download stats
				stats := download.NewDownloadStats()

				// Initialize SAM
				err := i2p.InitSAM()
				if err != nil {
					showError("Failed to initialize SAM", err, myWindow)
					return
				}
				defer i2p.CloseSAM()

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
				dm = download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, len(info.Pieces))
				progressTicker := time.NewTicker(1 * time.Second)
				ctx, cancel := context.WithCancel(context.Background())
				downloadCancel = cancel

				go func() {
					var prevDownloaded int64 = 0
					for {
						select {
						case <-progressTicker.C:
							dm.LogProgress()
							progress := dm.Progress() / 100

							// Safely load the total downloaded bytes
							currentDownloaded := atomic.LoadInt64(&dm.Downloaded)
							bytesDownloaded := currentDownloaded - prevDownloaded
							prevDownloaded = currentDownloaded

							downloadSpeedKBps := float64(bytesDownloaded) / 1024 // KB/s

							// Update GUI elements on the main thread
							//fyne.CurrentApp().Driver().RunOnMain(func() {
							progressBar.SetValue(progress)
							statusLabel.SetText(fmt.Sprintf("Downloading: %.2f%%", dm.Progress()))
							downloadSpeedLabel.SetText(fmt.Sprintf("Speed: %.2f KB/s", downloadSpeedKBps))
							//})
						case <-ctx.Done():
							progressTicker.Stop()
							return
						}
					}
				}()
				defer progressTicker.Stop()

				// Get peers from tracker
				peers, err := tracker.GetPeersFromSimpTracker(&mi)
				if err != nil {
					showError("Failed to get peers from tracker", err, myWindow)
					return
				}

				// Limit the number of connections based on user settings
				maxPeers := maxConnections
				if len(peers) < maxPeers {
					maxPeers = len(peers)
				}

				for i := 0; i < maxPeers; i++ {
					wg.Add(1)
					go func(peerHash []byte, index int) {
						defer wg.Done()
						stats.ConnectionStarted()
						defer stats.ConnectionEnded()
						//peer.ConnectToPeer(ctx, peerHash, index, &mi, dm)
						retryConnect(ctx, peerHash, index, &mi, dm, maxRetries, initialDelay)
					}(peers[i], i)
				}

				wg.Wait()

				cancel()

				if dm.IsFinished() {
					dialog.ShowInformation("Download Complete", fmt.Sprintf("Downloaded %s successfully.", info.Name), myWindow)
				} else {
					dialog.ShowInformation("Download Incomplete", "The download did not complete successfully.", myWindow)
				}
			}()
		}, myWindow)
	}

	// Stop button handler
	stopButton.OnTapped = func() {
		if !downloadInProgress {
			return
		}
		statusLabel.SetText("Stopping download...")
		if downloadCancel != nil {
			downloadCancel()
		}
	}

	// Show the window and start the GUI event loop
	myWindow.ShowAndRun()
}

// Helper function to display errors
func showError(title string, err error, parent fyne.Window) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), parent)
}

// Helper function to show the About dialog
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

func showDisclaimer(app fyne.App, parent fyne.Window) {
	// Create the content for the disclaimer
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It will have bugs, faulty GUIs and other things.\nBut at the same time will be updated frequently, check back for updates!"),
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

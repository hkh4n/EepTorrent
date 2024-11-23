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
	"bytes"
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
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/go-i2p/sam3"
	"github.com/sirupsen/logrus"
	"io"
	"net"
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
	log          = logrus.StandardLogger()
	maxRetries   = 20
	initialDelay = 2 * time.Second
	logFile      *os.File
	logFileMux   sync.Mutex
	logBuffer    bytes.Buffer
)

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetOutput(io.MultiWriter(os.Stderr, &logBuffer))
	log.SetLevel(logrus.DebugLevel)
}

func main() {
	// Initialize the Fyne application
	myApp := app.New()
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	// Set background (optional)
	background := canvas.NewImageFromResource(logo.ResourceLogoPng)
	background.FillMode = canvas.ImageFillContain // Adjust as needed: FillOriginal, FillContain, FillFill, FillStretch

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

	downloadSpeedLabel := widget.NewLabel("Download Speed: 0 KB/s")
	uploadSpeedLabel := widget.NewLabel("Upload Speed: 0 KB/s")
	totalUploadedLabel := widget.NewLabel("Total Uploaded: 0 MB")

	// Create the main content area
	mainContent := container.NewVBox(
		progressBar,
		downloadSpeedLabel,
		uploadSpeedLabel,
		totalUploadedLabel,
		statusLabel,
		container.NewHBox(startButton, stopButton),
	)

	// Layout the UI components with side menu
	content := container.NewBorder(nil, nil, sideMenu, nil, mainContent)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))

	// Add a menu bar
	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Torrent File...", func() {
				// Trigger the start button action
				startButton.OnTapped()
			}),
			fyne.NewMenuItem("Save Logs to File...", func() {
				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						showError("Save Logs Error", err, myWindow)
						return
					}
					if writer == nil {
						// User canceled the dialog
						return
					}
					logFilePath := writer.URI().Path()
					writer.Close() // Close immediately after getting the path

					if logFilePath == "" {
						showError("Invalid File Path", fmt.Errorf("No file path selected"), myWindow)
						return
					}

					// Open the selected log file
					file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					if err != nil {
						showError("Failed to Open Log File", err, myWindow)
						return
					}

					// Safely update the log output
					logFileMux.Lock()
					defer logFileMux.Unlock()

					// Write the contents of logBuffer to the file
					_, err = file.Write(logBuffer.Bytes())
					if err != nil {
						showError("Failed to Write Logs to File", err, myWindow)
						file.Close()
						return
					}

					// If a log file was previously open, close it
					if logFile != nil {
						logFile.Close()
					}

					logFile = file
					// Set Logrus to write to os.Stderr, logBuffer, and the file
					log.SetOutput(io.MultiWriter(os.Stderr, &logBuffer, logFile))
					log.Info("Logging to file enabled")
					dialog.ShowInformation("Logging Enabled", fmt.Sprintf("Logs are being saved to:\n%s", logFilePath), myWindow)
				}, myWindow)

				// Set the default file name
				saveDialog.SetFileName("eeptorrent.log")

				// Show the save dialog
				saveDialog.Show()
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

	// Ensure log file is closed on application exit
	myWindow.SetOnClosed(func() {
		logFileMux.Lock()
		if logFile != nil {
			logFile.Close()
		}
		logFileMux.Unlock()
	})

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
				_ = stats

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
				dm.DownloadDir = downloadDir
				progressTicker := time.NewTicker(10 * time.Second)
				ctx, cancel := context.WithCancel(context.Background())
				downloadCancel = cancel

				// Start the listener for incoming connections (seeding)
				go func() {
					err := startPeerListener(dm, &mi)
					if err != nil {
						log.WithError(err).Error("Failed to start peer listener")
					}
				}()

				// Progress updater
				go func() {
					var prevDownloaded int64 = 0
					var prevUploaded int64 = 0
					for {
						select {
						case <-progressTicker.C:
							dm.LogProgress()
							progress := dm.Progress() / 100

							currentDownloaded := atomic.LoadInt64(&dm.Downloaded)
							currentUploaded := atomic.LoadInt64(&dm.Uploaded)
							bytesDownloaded := currentDownloaded - prevDownloaded
							bytesUploaded := currentUploaded - prevUploaded
							prevDownloaded = currentDownloaded
							prevUploaded = currentUploaded

							downloadSpeedKBps := float64(bytesDownloaded) / 1024
							uploadSpeedKBps := float64(bytesUploaded) / 1024

							// Update GUI elements on the main thread

							progressBar.SetValue(progress)
							statusLabel.SetText(fmt.Sprintf("Downloading: %.2f%%", dm.Progress()))
							downloadSpeedLabel.SetText(fmt.Sprintf("Download Speed: %.2f KB/s", downloadSpeedKBps))
							uploadSpeedLabel.SetText(fmt.Sprintf("Upload Speed: %.2f KB/s", uploadSpeedKBps))
							totalUploadedLabel.SetText(fmt.Sprintf("Total Uploaded: %.2f MB", float64(dm.Uploaded)/1024/1024))

						case <-ctx.Done():
							progressTicker.Stop()
							return
						}
					}
				}()
				defer progressTicker.Stop()

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

				peersSkank, err := tracker.GetPeersFromSkankTracker(&mi)
				if err != nil {
					log.WithError(err).Warn("Failed to get peers from Skank Tracker")
				} else {
					allPeers = append(allPeers, peersSkank...)
				}

				if len(allPeers) == 0 {
					showError("Failed to get peers from any tracker", fmt.Errorf("No peers found"), myWindow)
					return
				}
				uniquePeers := removeDuplicatePeers(allPeers)

				// Limit the number of connections based on user settings
				maxPeers := maxConnections
				if len(uniquePeers) < maxPeers {
					maxPeers = len(uniquePeers)
				}

				for i := 0; i < maxPeers; i++ {
					wg.Add(1)
					go func(peerHash []byte, index int) {
						defer wg.Done()
						//stats.ConnectionStarted() // Assuming you have stats tracking
						//defer stats.ConnectionEnded()
						retryConnect(ctx, peerHash, index, &mi, dm, maxRetries, initialDelay)
					}(uniquePeers[i], i)
				}

				wg.Wait()

				cancel()

				if dm.IsFinished() {
					dialog.ShowInformation("Download Complete", fmt.Sprintf("Downloaded %s successfully.", info.Name), myWindow)
					statusLabel.SetText("Seeding...")
				} else {
					dialog.ShowInformation("Download Incomplete", "The download did not complete successfully.", myWindow)
					statusLabel.SetText("Download Incomplete")
				}
			}()
		}, myWindow)
	}
	// Ensure log file is closed on application exit
	defer func() {
		logFileMux.Lock()
		if logFile != nil {
			logFile.Close()
		}
		logFileMux.Unlock()
	}()

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
		widget.NewLabel("EepTorrent is experimental software. It will have bugs, faulty GUIs and other things. Please note that metrics may be inaccurate as this program is in flux.\nBut at the same time will be updated frequently, check back for updates!"),
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

// In startPeerListener function
func startPeerListener(dm *download.DownloadManager, mi *metainfo.MetaInfo) error {
	// Generate keys for the listener session
	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		return fmt.Errorf("Failed to generate keys for listener session: %v", err)
	}

	// Create the listener session
	listenerSession, err := i2p.GlobalSAM.NewStreamSession("listenerSession", keys, sam3.Options_Default)
	if err != nil {
		return fmt.Errorf("Failed to create listener session: %v", err)
	}

	// Start listening for incoming connections
	listener, err := listenerSession.Listen()
	if err != nil {
		return fmt.Errorf("Failed to start listening: %v", err)
	}

	log.Info("Started listener for incoming connections")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Error("Error accepting connection")
			continue
		}

		go handleIncomingConnection(conn, dm, mi)
	}
}

func handleIncomingConnection(conn net.Conn, dm *download.DownloadManager, mi *metainfo.MetaInfo) {
	defer conn.Close()

	// Generate peer ID
	peerIDMeta := util.GeneratePeerIdMeta()

	// Perform handshake
	err := peer.PerformHandshake(conn, mi.InfoHash().Bytes(), string(peerIDMeta[:]))
	if err != nil {
		log.WithError(err).Error("Handshake failed with incoming peer")
		return
	}

	// Wrap the connection
	pc := pp.NewPeerConn(conn, peerIDMeta, mi.InfoHash())
	pc.Timeout = 30 * time.Second

	// Add the peer to the DownloadManager
	dm.AddPeer(pc)
	defer dm.RemovePeer(pc)

	// Start handling messages
	err = peer.HandlePeerConnection(context.Background(), pc, dm)
	if err != nil {
		log.WithError(err).Error("Error handling incoming peer connection")
	}
}

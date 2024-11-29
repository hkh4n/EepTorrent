package gui

import (
	"bytes"
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"eeptorrent/lib/upload"
	"eeptorrent/lib/util"
	"eeptorrent/lib/util/logo"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	log          = logrus.StandardLogger()
	maxRetries   = 100
	initialDelay = 2 * time.Second
	logFile      *os.File
	logFileMux   sync.Mutex
	logBuffer    bytes.Buffer
)

// RunApp initializes and runs the GUI application.
func RunApp() {
	myApp := app.NewWithID("com.i2p.EepTorrent")
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	ShowDisclaimer(myApp, myWindow)

	var dm *download.DownloadManager
	var wg sync.WaitGroup
	var downloadInProgress bool
	var downloadCancel context.CancelFunc

	progressBar := widget.NewProgressBar()
	statusLabel := widget.NewLabel("Ready")
	startButton := widget.NewButton("Start Download", nil)
	stopButton := widget.NewButton("Stop Download", nil)
	stopButton.Disable()

	// Speed and upload labels
	downloadSpeedLabel := widget.NewLabel("Download Speed: 0 KB/s")
	uploadSpeedLabel := widget.NewLabel("Upload Speed: 0 KB/s")
	totalUploadedLabel := widget.NewLabel("Total Uploaded: 0 MB")

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
	maxConnectionsEntry.SetText("50")

	loggingLevelSelect := widget.NewSelect([]string{"Debug", "Info", "Warning", "Error", "Fatal", "Panic"}, func(value string) {
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

	settingsForm := widget.NewForm(
		widget.NewFormItem("Download Directory", container.NewHBox(downloadDirEntry, downloadDirButton)),
		widget.NewFormItem("Max Connections", maxConnectionsEntry),
		widget.NewFormItem("Logging Level", loggingLevelSelect),
	)
	settingsForm.Resize(fyne.NewSize(600, settingsForm.Size().Height))

	// Define content for other menu items
	uploadsContent := widget.NewLabel("Uploads content goes here.")
	peersContent := widget.NewLabel("Peers content goes here.")
	logsContent := widget.NewLabel("")
	logsContent.Wrapping = fyne.TextWrapWord
	metricsContent := container.NewVBox()

	// Periodically update logsContent with logBuffer
	go func() {
		const maxLogLength = 3600 // Define maximum log length
		var previousLogs string
		for {
			time.Sleep(1 * time.Second) // Adjust the interval as needed
			logFileMux.Lock()
			currentLogs := logBuffer.String()
			if currentLogs != previousLogs {
				// Trim the log to the last maxLogLength characters if necessary
				if len(currentLogs) > maxLogLength {
					currentLogs = currentLogs[len(currentLogs)-maxLogLength:]
				}
				logsContent.SetText(currentLogs)
				previousLogs = currentLogs
			}
			logFileMux.Unlock()
		}
	}()

	// Initialize ChartData
	chartData := NewChartData(30)

	// Initialize Chart Image
	chartImage := canvas.NewImageFromImage(nil)
	chartImage.FillMode = canvas.ImageFillOriginal
	chartImage.Resize(fyne.NewSize(600, 400))

	// Create Metrics Content with the chart
	metricsContent = container.NewVBox(
		chartImage,
	)

	mainContent := container.NewMax()
	menuItems := []string{"Settings", "Downloads", "Uploads", "Peers", "Logs", "Metrics"}

	menuList := widget.NewList(
		func() int {
			return len(menuItems)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(menuItems[i])
		},
	)

	menuList.OnSelected = func(id widget.ListItemID) {
		selectedItem := menuItems[id]
		switch selectedItem {
		case "Settings":
			mainContent.Objects = []fyne.CanvasObject{settingsForm}
		case "Downloads":
			downloadsVBox := container.NewVBox(
				progressBar,
				downloadSpeedLabel,
				uploadSpeedLabel,
				totalUploadedLabel,
				statusLabel,
				container.NewHBox(startButton, stopButton),
			)
			mainContent.Objects = []fyne.CanvasObject{downloadsVBox}
		case "Uploads":
			mainContent.Objects = []fyne.CanvasObject{uploadsContent}
		case "Peers":
			mainContent.Objects = []fyne.CanvasObject{peersContent}
		case "Logs":
			logsContainer := container.NewVBox(
				logsContent,
			)
			mainContent.Objects = []fyne.CanvasObject{logsContainer}
		case "Metrics":
			mainContent.Objects = []fyne.CanvasObject{metricsContent}
		}
		mainContent.Refresh()
	}

	menuList.Select(0)

	scrollableMenu := container.NewVScroll(menuList)
	scrollableMenu.SetMinSize(fyne.NewSize(150, 0))

	content := container.NewBorder(nil, nil, scrollableMenu, nil, mainContent)

	// Initially, hide main content until SAM is initialized
	mainContent.Hide()

	// Display SAM Settings Dialog
	ShowSAMSettingsDialog(myApp, myWindow, func(success bool, err error) {
		if !success {
			if err != nil {
				ShowError("SAM Initialization Failed", err, myWindow)
			}
			myApp.Quit()
			return
		}
		mainContent.Show()
	})

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))

	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Torrent File...", func() {
				startButton.OnTapped()
			}),
			fyne.NewMenuItem("Save Logs to File...", func() {
				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						ShowError("Save Logs Error", err, myWindow)
						return
					}
					if writer == nil {
						// User canceled the dialog
						return
					}
					logFilePath := writer.URI().Path()
					writer.Close() // Close immediately after getting the path

					if logFilePath == "" {
						ShowError("Invalid File Path", fmt.Errorf("No file path selected"), myWindow)
						return
					}

					// Open the selected log file
					file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					if err != nil {
						ShowError("Failed to Open Log File", err, myWindow)
						return
					}

					// Safely update the log output
					logFileMux.Lock()
					defer logFileMux.Unlock()

					// Write the contents of logBuffer to the file
					_, err = file.Write(logBuffer.Bytes())
					if err != nil {
						ShowError("Failed to Write Logs to File", err, myWindow)
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
				// Focus on the settings side menu
				menuList.Select(0) // Select "Settings"
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				ShowAboutDialog(myApp, myWindow)
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

	// Initialize ChartData and start a goroutine to update the chart periodically
	/*
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Replace with actual download and upload speeds
					downloadSpeed := rand.Float64() * 100
					uploadSpeed := rand.Float64() * 50
					chartData.AddPoint(downloadSpeed, uploadSpeed)

					// Update the chart on the main thread
					UpdateMetricsChart(chartData, chartImage, myApp)
				}
			}
		}()
	*/

	// Start button handler
	startButton.OnTapped = func() {
		if downloadInProgress {
			return
		}

		// Validate and apply settings
		downloadDir := downloadDirEntry.Text
		if downloadDir == "" {
			ShowError("Invalid Settings", fmt.Errorf("Please select a download directory"), myWindow)
			return
		}

		maxConnections, err := strconv.Atoi(maxConnectionsEntry.Text)
		if err != nil || maxConnections <= 0 {
			ShowError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), myWindow)
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
					if dm.IsFinished() {
						statusLabel.SetText("Seeding...")
					} else {
						statusLabel.SetText("Download Incomplete")
					}
				}()

				// Initialize download stats
				stats := download.NewDownloadStats()
				_ = stats

				// Load the torrent file
				mi, err := metainfo.LoadFromFile(torrentFilePath)
				if err != nil {
					ShowError("Failed to load torrent", err, myWindow)
					return
				}

				info, err := mi.Info()
				if err != nil {
					ShowError("Failed to parse torrent info", err, myWindow)
					return
				}
				totalPieces := info.CountPieces()
				log.Warnf("Torrent Info: Total Length = %d bytes, Piece Length = %d bytes, Total Pieces = %d",
					info.TotalLength(), info.PieceLength, totalPieces)
				// Initialize the file writer
				var outputPath string
				var mode os.FileMode
				if len(info.Files) == 0 {
					// Single-file torrent
					outputPath = filepath.Join(downloadDir, info.Name) // Correctly set to file path
					mode = 0644
				} else {
					// Multi-file torrent
					outputPath = filepath.Join(downloadDir, info.Name)
					mode = 0755
					// Create the directory if it doesn't exist
					err := os.MkdirAll(outputPath, mode)
					if err != nil && !os.IsExist(err) {
						ShowError("Failed to create output directory", err, myWindow)
						return
					}
				}

				writer := metainfo.NewWriter(outputPath, info, mode)
				dm = download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, info.CountPieces(), downloadDir)
				pm := peer.NewPeerManager(dm)
				defer pm.Shutdown()

				// If we're a seeder, set all pieces as complete
				/*
					dm.Mu.Lock()
					for i := 0; i < dm.TotalPieces; i++ {
						dm.Bitfield.Set(uint32(i))
						piece := dm.Pieces[i]
						for j := range piece.Blocks {
							piece.Blocks[j] = true
						}
					}
					dm.Mu.Unlock()

				*/

				//pm := peer.NewPeerManager(dm)
				progressTicker := time.NewTicker(1 * time.Second)
				ctx, cancel := context.WithCancel(context.Background())
				downloadCancel = cancel

				// Start the listener for incoming connections (seeding)
				go func() {
					err := startPeerListener(&mi, downloadDir)
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

							// Update the chart with real speeds
							chartData.AddPoint(downloadSpeedKBps, uploadSpeedKBps)
							UpdateMetricsChart(chartData, chartImage, myApp)

						case <-ctx.Done():
							progressTicker.Stop()
							return
						}
					}
				}()
				defer progressTicker.Stop()

				// Get peers from trackers
				allPeers, err := getAllPeers(&mi)
				if err != nil {
					ShowError("Failed to get peers from any tracker", err, myWindow)
					return
				}

				uniquePeers := peer.RemoveDuplicatePeers(allPeers)

				// Limit the number of connections based on user settings
				maxPeers := maxConnections
				if len(uniquePeers) < maxPeers {
					maxPeers = len(uniquePeers)
				}

				for i := 0; i < maxPeers; i++ {
					wg.Add(1)
					go func(peerHash []byte, index int) {
						defer wg.Done()
						retryConnect(ctx, peerHash, index, &mi, dm, pm, maxRetries, initialDelay)
					}(uniquePeers[i], i)
				}

				wg.Wait()

				// cancel() // Do Not Cancel the Seeding Context

				if dm.IsFinished() {

					dialog.ShowInformation("Download Complete", fmt.Sprintf("Downloaded %s successfully.", info.Name), myWindow)
					statusLabel.SetText("Seeding...")
					select {
					case <-ctx.Done():
						return
					}

				} else {
					dialog.ShowInformation("Download Incomplete", "The download did not complete successfully.", myWindow)
					statusLabel.SetText("Download Incomplete")

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
	// Show the window but keep main content hidden until SAM is initialized
	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))
	myWindow.Show()

	// Show the window and start the GUI event loop
	myWindow.ShowAndRun()
}

func ShowDisclaimer(app fyne.App, parent fyne.Window) {
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It will have bugs, faulty GUIs and other things. Please note that metrics may be glitchy as this program is in flux.\nBut at the same time will be updated frequently, check back for updates!"),
		widget.NewLabel("EepTorrent Copyright (C) 2024 Haris Khan\nThis program comes with ABSOLUTELY NO WARRANTY.\nThis is free software, and you are welcome to redistribute it under certain conditions. See COPYING for details."),
	)

	dialog := dialog.NewCustomConfirm(
		"Experimental Software",
		"Accept",
		"Decline",
		disclaimerContent,
		func(accepted bool) {
			if !accepted {
				app.Quit()
			}
		},
		parent,
	)

	dialog.SetDismissText("Decline")
	dialog.Show()
}

func ShowError(title string, err error, parent fyne.Window) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), parent)
}

func ShowAboutDialog(app fyne.App, parent fyne.Window) {
	gitCommitDisplay := util.GitCommit
	dialog.ShowCustom("About EepTorrent", "Close",
		container.NewVBox(
			widget.NewLabelWithStyle("EepTorrent", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel(fmt.Sprintf("Version: %s-%s", util.Version, gitCommitDisplay)),
			widget.NewLabel("A cross-platform I2P-only BitTorrent client."),
			widget.NewLabel("© 2024 Haris Khan"),
		), parent)
}

// showSAMSettingsDialog displays a popup for SAM configuration and invokes a callback upon completion.
func ShowSAMSettingsDialog(myApp fyne.App, myWindow fyne.Window, onComplete func(success bool, err error)) {
	modeGroup := widget.NewRadioGroup([]string{"Use Default Settings", "Custom Settings"}, func(selected string) {})
	modeGroup.SetSelected("Use Default Settings")

	inboundLengthEntry := widget.NewEntry()
	inboundLengthEntry.SetPlaceHolder("Inbound Length")
	inboundLengthEntry.SetText("1")

	outboundLengthEntry := widget.NewEntry()
	outboundLengthEntry.SetPlaceHolder("Outbound Length")
	outboundLengthEntry.SetText("1")

	inboundQuantityEntry := widget.NewEntry()
	inboundQuantityEntry.SetPlaceHolder("Inbound Quantity")
	inboundQuantityEntry.SetText("3")

	outboundQuantityEntry := widget.NewEntry()
	outboundQuantityEntry.SetPlaceHolder("Outbound Quantity")
	outboundQuantityEntry.SetText("3")

	inboundBackupQuantityEntry := widget.NewEntry()
	inboundBackupQuantityEntry.SetPlaceHolder("Inbound Backup Quantity")
	inboundBackupQuantityEntry.SetText("1")

	outboundBackupQuantityEntry := widget.NewEntry()
	outboundBackupQuantityEntry.SetPlaceHolder("Outbound Backup Quantity")
	outboundBackupQuantityEntry.SetText("1")

	inboundLengthVarianceEntry := widget.NewEntry()
	inboundLengthVarianceEntry.SetPlaceHolder("Inbound Length Variance")
	inboundLengthVarianceEntry.SetText("0")

	outboundLengthVarianceEntry := widget.NewEntry()
	outboundLengthVarianceEntry.SetPlaceHolder("Outbound Length Variance")
	outboundLengthVarianceEntry.SetText("0")

	inboundLengthEntry.Disable()
	outboundLengthEntry.Disable()
	inboundQuantityEntry.Disable()
	outboundQuantityEntry.Disable()
	inboundBackupQuantityEntry.Disable()
	outboundBackupQuantityEntry.Disable()
	inboundLengthVarianceEntry.Disable()
	outboundLengthVarianceEntry.Disable()

	modeGroup.OnChanged = func(selected string) {
		if selected == "Custom Settings" {
			inboundLengthEntry.Enable()
			outboundLengthEntry.Enable()
			inboundQuantityEntry.Enable()
			outboundQuantityEntry.Enable()
			inboundBackupQuantityEntry.Enable()
			outboundBackupQuantityEntry.Enable()
			inboundLengthVarianceEntry.Enable()
			outboundLengthVarianceEntry.Enable()
		} else {
			inboundLengthEntry.Disable()
			outboundLengthEntry.Disable()
			inboundQuantityEntry.Disable()
			outboundQuantityEntry.Disable()
			inboundBackupQuantityEntry.Disable()
			outboundBackupQuantityEntry.Disable()
			inboundLengthVarianceEntry.Disable()
			outboundLengthVarianceEntry.Disable()
		}
	}

	samSettingsForm := widget.NewForm(
		widget.NewFormItem("Mode", modeGroup),
		widget.NewFormItem("Inbound Length", inboundLengthEntry),
		widget.NewFormItem("Outbound Length", outboundLengthEntry),
		widget.NewFormItem("Inbound Quantity", inboundQuantityEntry),
		widget.NewFormItem("Outbound Quantity", outboundQuantityEntry),
		widget.NewFormItem("Inbound Backup Quantity", inboundBackupQuantityEntry),
		widget.NewFormItem("Outbound Backup Quantity", outboundBackupQuantityEntry),
		widget.NewFormItem("Inbound Length Variance", inboundLengthVarianceEntry),
		widget.NewFormItem("Outbound Length Variance", outboundLengthVarianceEntry),
	)

	content := container.NewVBox(
		samSettingsForm,
	)

	dialog.ShowCustomConfirm("SAM Settings", "Save", "Cancel", content, func(confirm bool) {
		if confirm {
			var cfg i2p.SAMConfig
			if modeGroup.Selected == "Use Default Settings" {
				cfg = i2p.DefaultSAMConfig()
			} else {
				// Validate and parse custom settings
				inboundLength, err := strconv.Atoi(inboundLengthEntry.Text)
				if err != nil || inboundLength < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Length must be a non-negative integer"), myWindow)
					return
				}

				outboundLength, err := strconv.Atoi(outboundLengthEntry.Text)
				if err != nil || outboundLength < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Length must be a non-negative integer"), myWindow)
					return
				}

				inboundQuantity, err := strconv.Atoi(inboundQuantityEntry.Text)
				if err != nil || inboundQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Quantity must be a non-negative integer"), myWindow)
					return
				}

				outboundQuantity, err := strconv.Atoi(outboundQuantityEntry.Text)
				if err != nil || outboundQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Quantity must be a non-negative integer"), myWindow)
					return
				}

				inboundBackupQuantity, err := strconv.Atoi(inboundBackupQuantityEntry.Text)
				if err != nil || inboundBackupQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Backup Quantity must be a non-negative integer"), myWindow)
					return
				}

				outboundBackupQuantity, err := strconv.Atoi(outboundBackupQuantityEntry.Text)
				if err != nil || outboundBackupQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Backup Quantity must be a non-negative integer"), myWindow)
					return
				}

				inboundLengthVariance, err := strconv.Atoi(inboundLengthVarianceEntry.Text)
				if err != nil || inboundLengthVariance < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Length Variance must be a non-negative integer"), myWindow)
					return
				}

				outboundLengthVariance, err := strconv.Atoi(outboundLengthVarianceEntry.Text)
				if err != nil || outboundLengthVariance < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Length Variance must be a non-negative integer"), myWindow)
					return
				}

				cfg = i2p.SAMConfig{
					InboundLength:          inboundLength,
					OutboundLength:         outboundLength,
					InboundQuantity:        inboundQuantity,
					OutboundQuantity:       outboundQuantity,
					InboundBackupQuantity:  inboundBackupQuantity,
					OutboundBackupQuantity: outboundBackupQuantity,
					InboundLengthVariance:  inboundLengthVariance,
					OutboundLengthVariance: outboundLengthVariance,
				}
			}

			// Initialize SAM with the selected configuration
			err := i2p.InitSAM(cfg)
			if err != nil {
				ShowError("SAM Initialization Failed", err, myWindow)
				return
			}

			// Invoke the callback to proceed to the main interface
			onComplete(true, nil)
		} else {
			// Handle cancellation, e.g., exit the application
			dialog.ShowConfirm("Exit Application", "Are you sure you want to exit?", func(confirmed bool) {
				if confirmed {
					onComplete(false, fmt.Errorf("User canceled SAM settings"))
				}
			}, myWindow)
		}
	}, myWindow)
}

// getAllPeers retrieves peers from multiple trackers.
func getAllPeers(mi *metainfo.MetaInfo) ([][]byte, error) {
	var allPeers [][]byte
	timeout := time.Second * 15

	// List of tracker functions
	trackerFuncs := []func(*metainfo.MetaInfo, time.Duration) ([][]byte, error){
		tracker.GetPeersFromEepTorrentTracker,
		tracker.GetPeersFromPostmanTracker,
		tracker.GetPeersFromSimpTracker,
		tracker.GetPeersFromDg2Tracker,
		tracker.GetPeersFromSkankTracker,
		tracker.GetPeersFromOmitTracker,
		tracker.GetPeersFrom6kw6Tracker,
	}

	for _, getPeers := range trackerFuncs {
		peers, err := getPeers(mi, timeout)
		if err != nil {
			log.WithError(err).Warn("Failed to get peers from tracker")
		} else {
			allPeers = append(allPeers, peers...)
		}
		time.Sleep(1 * time.Second) // Throttle requests
	}

	if len(allPeers) == 0 {
		return nil, fmt.Errorf("No peers found")
	}
	return allPeers, nil
}

// retryConnect attempts to connect to a peer with retry logic.
// maxRetries: Maximum number of retry attempts.
// initialDelay: Initial delay before the first retry.
func retryConnect(ctx context.Context, peerHash []byte, index int, mi *metainfo.MetaInfo, dm *download.DownloadManager, pm *peer.PeerManager, maxRetries int, initialDelay time.Duration) {
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, stopping retries for peer %d", index)
			return
		default:
		}

		err := peer.ConnectToPeer(ctx, peerHash, index, mi, dm, pm)
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

func startPeerListener(mi *metainfo.MetaInfo, downloadDir string) error {
	listenerSession := i2p.GlobalStreamSession

	listener, err := listenerSession.Listen()
	if err != nil {
		return fmt.Errorf("Failed to start listening: %v", err)
	}
	defer listener.Close()

	log.Info("Started seeding listener on address: ", listenerSession.Addr().Base32())

	// Initialize the file writer for the seeder
	var outputPath string
	var mode os.FileMode
	info, err := mi.Info()
	if err != nil {
		return fmt.Errorf("Failed to get torrent info: %v", err)
	}

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
			return fmt.Errorf("Failed to create output directory: %v", err)
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)

	// Initialize the UploadManager for the seeder
	um := upload.NewUploadManager(writer, info, downloadDir)

	// Initialize the PeerManager for uploading
	pm := peer.NewPeerManager(nil) // Pass nil since we're not downloading

	// Start accepting incoming connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Error("Error accepting connection")
			continue
		}

		go func(conn net.Conn) {
			defer conn.Close()

			// Set appropriate timeouts
			conn.SetDeadline(time.Now().Add(30 * time.Second))

			// Generate peer ID for this connection
			peerId := util.GeneratePeerIdMeta()

			// Perform handshake
			err := peer.PerformHandshake(conn, mi.InfoHash().Bytes(), string(peerId[:]))
			if err != nil {
				log.WithError(err).Error("Failed handshake with seeding peer")
				return
			}

			// Create peer connection
			pc := pp.NewPeerConn(conn, peerId, mi.InfoHash())
			pc.Timeout = 30 * time.Second

			// Add peer to UploadManager
			um.AddPeer(pc)
			defer um.RemovePeer(pc)

			// Handle the peer connection in seeding mode
			err = handleSeedingPeer(pc, um, pm)
			if err != nil {
				log.WithError(err).Error("Error handling seeding peer")
			}
		}(conn)
	}
}

// handleSeedingPeer manages an incoming seeding peer connection.
// It handles requests for pieces, manages upload interactions, and ensures robust error handling.
func handleSeedingPeer(pc *pp.PeerConn, um *upload.UploadManager, pm *peer.PeerManager) error {
	log := log.WithField("peer", pc.RemoteAddr().String())
	log.Info("Handling new seeding connection")

	// Add peer to PeerManager
	pm.Mu.Lock()
	if pm.Peers == nil {
		pm.Peers = make(map[*pp.PeerConn]*peer.PeerState)
	}
	pm.Peers[pc] = peer.NewPeerState()
	pm.Mu.Unlock()

	defer func() {
		pm.Mu.Lock()
		delete(pm.Peers, pc)
		pm.Mu.Unlock()
	}()

	// Send bitfield to the peer
	err := pc.SendBitfield(um.Bitfield)
	if err != nil {
		log.WithError(err).Error("Failed to send bitfield to peer")
		return fmt.Errorf("failed to send bitfield: %v", err)
	}

	// Unchoke the peer to allow requests
	err = pc.SendUnchoke()
	if err != nil {
		log.WithError(err).Error("Failed to send unchoke to peer")
		return fmt.Errorf("failed to send unchoke: %v", err)
	}

	// Main message handling loop
	for {
		msg, err := pc.ReadMsg()
		if err != nil {
			if err.Error() == "EOF" {
				log.Info("Peer closed the connection")
			} else {
				log.WithError(err).Error("Failed to read message from peer")
			}
			return fmt.Errorf("failed to read message: %v", err)
		}

		switch msg.Type {
		case pp.MTypeRequest:
			// Handle piece requests from the peer
			blockData, err := um.GetBlock(msg.Index, msg.Begin, msg.Length)
			if err != nil {
				log.WithError(err).Error("Failed to retrieve requested block")
				continue
			}

			// Send the piece to the peer
			err = pc.SendPiece(msg.Index, msg.Begin, blockData)
			if err != nil {
				log.WithError(err).Error("Failed to send piece to peer")
				continue
			}

			// Update upload stats
			uploadSize := int64(len(blockData))
			um.Mu.Lock()
			um.Uploaded += uploadSize
			um.Mu.Unlock()
			pm.UpdatePeerStats(pc, 0, uploadSize)

			log.WithFields(logrus.Fields{
				"index": msg.Index,
				"begin": msg.Begin,
				"size":  len(blockData),
			}).Debug("Successfully sent piece to peer")

		case pp.MTypeInterested:
			// Handle interested messages
			pm.OnPeerInterested(pc)
			log.Debug("Peer is interested in downloading")

		case pp.MTypeNotInterested:
			// Handle not interested messages
			pm.OnPeerNotInterested(pc)
			log.Debug("Peer is not interested in downloading")

		case pp.MTypeHave:
			// Update peer's bitfield
			if int(msg.Index) < len(pc.BitField) {
				pc.BitField.Set(msg.Index)
				log.WithField("piece_index", msg.Index).Debug("Updated peer's bitfield with new piece")
			} else {
				log.WithField("piece_index", msg.Index).Warn("Received 'Have' for invalid piece index")
			}

		case pp.MTypeBitField:
			// Update peer's bitfield
			pc.BitField = msg.BitField
			log.WithField("pieces", pc.BitField.String()).Debug("Received peer's bitfield")

		case pp.MTypeCancel:
			// Handle cancel requests if needed
			log.Debug("Received 'Cancel' message from peer (ignored in seeding)")

		default:
			// Log unhandled message types
			log.WithField("message_type", msg.Type.String()).Debug("Received unhandled message type from peer")
		}
	}
}

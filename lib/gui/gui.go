package gui

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
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/sirupsen/logrus"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	log                = logrus.StandardLogger()
	maxRetries         = 100
	initialDelay       = 2 * time.Second
	logFile            *os.File
	logFileMux         sync.Mutex
	logBuffer          bytes.Buffer
	torrentListBinding binding.UntypedList
	addButton          *widget.Button
	myWindow           fyne.Window
	selectedTorrent    *TorrentItem
	myApp              fyne.App
	torrentListView    *widget.List
	removeButton       *widget.Button
	logsContent        *widget.Label
	uiUpdateChan       = make(chan func())
	uiUpdateWG         sync.WaitGroup
	samTabInstance     *SAMTab
)

type TorrentItem struct {
	Name            string
	Status          string
	Progress        float64
	DownloadSpeed   float64
	UploadSpeed     float64
	Downloaded      int64
	Uploaded        int64
	Size            int64
	ETA             string
	Peers           int
	dm              *download.DownloadManager
	pm              *peer.PeerManager
	ctx             context.Context
	cancelFunc      context.CancelFunc
	progressBinding binding.Float
	removed         bool
	mu              sync.Mutex
}

var torrentList []*TorrentItem
var torrentListLock sync.Mutex

type SAMTab struct {
	configForm          *widget.Form
	startButton         *widget.Button
	restartButton       *widget.Button
	stopButton          *widget.Button
	statusLabel         *widget.Label
	connected           bool
	currentSAMConfig    i2p.SAMConfig
	configLock          sync.Mutex
	samStatusUpdateChan chan bool
}

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetOutput(io.MultiWriter(os.Stderr, &logBuffer))
	log.SetLevel(logrus.DebugLevel)
}

// RunApp initializes and runs the GUI application.
func RunApp() {
	// Initialize the UI update channel and processor
	uiUpdateChan = make(chan func())
	uiUpdateWG.Add(1)
	go func() {
		defer uiUpdateWG.Done()
		for updateFunc := range uiUpdateChan {
			updateFunc()
		}
	}()
	myApp = app.NewWithID("com.i2p.EepTorrent")

	myApp.Settings().SetTheme(&CustomTheme{})

	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow = myApp.NewWindow("EepTorrent")

	ShowDisclaimer(myApp, myWindow)

	// Initialize SAMTab once
	samTabInstance = &SAMTab{
		connected:           false,
		samStatusUpdateChan: make(chan bool),
	}

	// Settings Form
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
	logsContent = widget.NewLabel("")
	logsContent.Wrapping = fyne.TextWrapWord
	metricsContent := container.NewVBox()

	// Periodically update logsContent with logBuffer
	go updateLogsContent()

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

	// Start updating the chart
	go updateMetricsChart(chartData, chartImage)

	// Initialize torrent list binding
	torrentListBinding = binding.NewUntypedList()

	// Create the torrent list view
	torrentListView = createTorrentListView()

	// Main content container
	mainContent := container.NewMax()
	menuItems := []string{"Settings", "SAM", "Torrents", "Uploads", "Peers", "Logs", "Metrics"}

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
		case "SAM":
			mainContent.Objects = []fyne.CanvasObject{createSAMTab()}
		case "Torrents":
			// Create toolbar
			toolbar := createToolbar(downloadDirEntry, maxConnectionsEntry)
			// Create headers
			headers := createListHeaders()
			// Combine toolbar, headers, and list
			torrentsVBox := container.NewVBox(
				toolbar,
				headers,
				torrentListView,
			)
			mainContent.Objects = []fyne.CanvasObject{torrentsVBox}
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

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))

	menu := fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Add Torrent...", func() {
				// Trigger the Add Torrent button
				if addButton != nil && addButton.OnTapped != nil {
					addButton.OnTapped()
				} else {
					dialog.ShowError(fmt.Errorf("Add Torrent button is not available"), myWindow)
				}
			}),
			fyne.NewMenuItem("Save Logs to File...", func() {
				saveLogsToFile()
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

		close(uiUpdateChan)
		uiUpdateWG.Wait()
	})

	myWindow.ShowAndRun()
}

// createToolbar creates the toolbar with Add and Remove buttons
func createToolbar(downloadDirEntry *widget.Entry, maxConnectionsEntry *widget.Entry) *fyne.Container {
	addButton = widget.NewButton("Add Torrent", func() {
		// Open file dialog to select torrent file
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}

			torrentFilePath := reader.URI().Path()
			reader.Close()

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

			// Start the torrent download
			addTorrent(torrentFilePath, downloadDir, maxConnections)
		}, myWindow)
	})

	removeButton = widget.NewButton("Remove Torrent", func() {
		torrentListLock.Lock()
		defer torrentListLock.Unlock()

		if selectedTorrent != nil {
			// Safely set the 'removed' flag
			selectedTorrent.mu.Lock()
			selectedTorrent.removed = true
			selectedTorrent.mu.Unlock()

			// Cancel the torrent's context to stop all associated goroutines
			selectedTorrent.cancelFunc()

			// Remove the torrent from the list
			for i, t := range torrentList {
				if t == selectedTorrent {
					torrentList = append(torrentList[:i], torrentList[i+1:]...)
					log.Infof("Removed torrent: %s at index %d", t.Name, i)
					break
				}
			}

			// Update the binding and UI
			torrentListBinding.Set(torrentListToAnySlice(torrentList))
			selectedTorrent = nil

			// Refresh the UI on the main thread
			uiUpdateChan <- func() {
				torrentListView.Refresh()
			}
		} else {
			uiUpdateChan <- func() {
				dialog.ShowInformation("No Torrent Selected", "Please select a torrent to remove.", myWindow)
				log.Warn("Attempted to remove a torrent without selection.")
			}
		}
	})

	toolbar := container.NewHBox(addButton, removeButton)
	return toolbar
}

// createListHeaders creates headers for the torrent list
func createListHeaders() *fyne.Container {
	headers := container.NewHBox(
		widget.NewLabelWithStyle("Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Progress", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("DL Speed", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("UL Speed", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("ETA", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(""),
	)
	return headers
}

// createTorrentListView creates the torrent list view
func createTorrentListView() *widget.List {
	torrentListView := widget.NewListWithData(
		torrentListBinding,
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("")
			statusLabel := widget.NewLabel("")
			progressBar := widget.NewProgressBar()
			progressBar.Min = 0
			progressBar.Max = 1
			downloadSpeedLabel := widget.NewLabel("")
			uploadSpeedLabel := widget.NewLabel("")
			etaLabel := widget.NewLabel("")
			actionButton := widget.NewButton("...", nil)
			return container.NewHBox(
				nameLabel,
				statusLabel,
				progressBar,
				downloadSpeedLabel,
				uploadSpeedLabel,
				etaLabel,
				actionButton,
			)
		},
		func(item binding.DataItem, o fyne.CanvasObject) {
			value, err := item.(binding.Untyped).Get()
			if err != nil {
				log.WithError(err).Error("Failed to get item from binding")
				return
			}
			torrent := value.(*TorrentItem)
			row := o.(*fyne.Container)

			nameLabel := row.Objects[0].(*widget.Label)
			statusLabel := row.Objects[1].(*widget.Label)
			progressBar := row.Objects[2].(*widget.ProgressBar)
			downloadSpeedLabel := row.Objects[3].(*widget.Label)
			uploadSpeedLabel := row.Objects[4].(*widget.Label)
			etaLabel := row.Objects[5].(*widget.Label)
			actionButton := row.Objects[6].(*widget.Button)

			nameLabel.SetText(torrent.Name)
			statusLabel.SetText(torrent.Status)
			progressBar.SetValue(torrent.Progress)
			downloadSpeedLabel.SetText(fmt.Sprintf("%.2f KB/s", torrent.DownloadSpeed))
			uploadSpeedLabel.SetText(fmt.Sprintf("%.2f KB/s", torrent.UploadSpeed))
			etaLabel.SetText(torrent.ETA)
			actionButton.OnTapped = func() {
				// Show torrent details or context menu
				showTorrentDetails(torrent, myWindow)
			}
		},
	)

	torrentListView.OnSelected = func(id widget.ListItemID) {
		torrentListLock.Lock()
		defer torrentListLock.Unlock()
		if id >= 0 && id < len(torrentList) {
			selectedTorrent = torrentList[id]
		} else {
			selectedTorrent = nil
		}
	}
	return torrentListView
}

// updateLogsContent periodically updates the logs content label
func updateLogsContent() {
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
			// Schedule UI update
			uiUpdateChan <- func() {
				logsContent.SetText(currentLogs)
			}
			previousLogs = currentLogs
		}
		logFileMux.Unlock()
	}
}

// updateMetricsChart updates the metrics chart periodically
func updateMetricsChart(chartData *ChartData, chartImage *canvas.Image) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Aggregate download and upload speeds from all torrents
			var totalDownloadSpeed float64
			var totalUploadSpeed float64

			torrentListLock.Lock()
			for _, torrent := range torrentList {
				totalDownloadSpeed += torrent.DownloadSpeed
				totalUploadSpeed += torrent.UploadSpeed
			}
			torrentListLock.Unlock()

			chartData.AddPoint(totalDownloadSpeed, totalUploadSpeed)

			// Update the chart on the main thread

			UpdateMetricsChart(chartData, chartImage, myApp)

		}
	}
}

func torrentListToAnySlice(torrents []*TorrentItem) []any {
	items := make([]any, len(torrents))
	for i, t := range torrents {
		items[i] = t
	}
	return items
}

// addTorrent starts the download of a torrent and adds it to the torrent list
func addTorrent(torrentFilePath string, downloadDir string, maxConnections int) {
	// Load the torrent file
	mi, err := metainfo.LoadFromFile(torrentFilePath)
	if err != nil {
		uiUpdateChan <- func() {
			ShowError("Failed to load torrent", err, myWindow)
		}
		return
	}

	info, err := mi.Info()
	if err != nil {
		uiUpdateChan <- func() {
			ShowError("Failed to parse torrent info", err, myWindow)
		}
		return
	}

	totalPieces := info.CountPieces()
	log.Warnf("Torrent Info: Name=%s, Total Length=%d bytes, Piece Length=%d bytes, Total Pieces=%d",
		info.Name, info.TotalLength(), info.PieceLength, totalPieces)

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
			uiUpdateChan <- func() {
				ShowError("Failed to create output directory", err, myWindow)
			}
			return
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)
	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, info.CountPieces(), downloadDir)
	pm := peer.NewPeerManager(dm)

	progressBinding := binding.NewFloat()

	ctx, cancel := context.WithCancel(context.Background())

	torrentItem := &TorrentItem{
		Name:            info.Name,
		Status:          "Initializing",
		Progress:        0,
		DownloadSpeed:   0,
		UploadSpeed:     0,
		Downloaded:      0,
		Uploaded:        0,
		Size:            info.TotalLength(),
		ETA:             "",
		Peers:           0,
		dm:              dm,
		pm:              pm,
		ctx:             ctx,
		cancelFunc:      cancel,
		progressBinding: progressBinding,
	}

	// Add to torrent list
	torrentListLock.Lock()
	uiUpdateChan <- func() {
		torrentList = append(torrentList, torrentItem)
		torrentListBinding.Set(torrentListToAnySlice(torrentList))

		// Automatically select the newly added torrent
		newIndex := len(torrentList) - 1
		torrentListView.Select(newIndex)
	}
	torrentListLock.Unlock()

	// Start the download in a separate goroutine
	go func(torrent *TorrentItem) {
		defer func() {
			if torrent.dm.IsFinished() {
				torrent.Status = "Seeding"
				uiUpdateChan <- func() {
					torrent.mu.Lock()
					defer torrent.mu.Unlock()
					if torrent.removed {
						return
					}
					myApp.SendNotification(&fyne.Notification{
						Title:   "Download Complete",
						Content: fmt.Sprintf("%s has finished downloading.", torrent.Name),
					})
				}
			} else {
				torrent.Status = "Stopped"
				uiUpdateChan <- func() {
					torrent.mu.Lock()
					defer torrent.mu.Unlock()
					if torrent.removed {
						return
					}
					myApp.SendNotification(&fyne.Notification{
						Title:   "Download Stopped",
						Content: fmt.Sprintf("%s download was stopped.", torrent.Name),
					})
				}
			}
		}()

		// Start the listener for incoming connections (seeding)
		go func() {
			err := peer.StartPeerListener(&mi, downloadDir)
			if err != nil {
				log.WithError(err).Error("Failed to start peer listener")
			}
		}()

		// Progress updater
		progressTicker := time.NewTicker(1 * time.Second)
		defer progressTicker.Stop()
		var prevDownloaded int64 = 0
		var prevUploaded int64 = 0
		for {
			select {
			case <-progressTicker.C:
				torrent.dm.LogProgress()
				progress := torrent.dm.Progress() / 100
				torrent.Progress = progress
				// torrent.progressBinding.Set(progress) // No longer needed with direct updates

				currentDownloaded := atomic.LoadInt64(&torrent.dm.Downloaded)
				currentUploaded := atomic.LoadInt64(&torrent.dm.Uploaded)
				bytesDownloaded := currentDownloaded - prevDownloaded
				bytesUploaded := currentUploaded - prevUploaded
				prevDownloaded = currentDownloaded
				prevUploaded = currentUploaded

				downloadSpeedKBps := float64(bytesDownloaded) / 1024
				uploadSpeedKBps := float64(bytesUploaded) / 1024

				torrent.DownloadSpeed = downloadSpeedKBps
				torrent.UploadSpeed = uploadSpeedKBps
				torrent.Downloaded = currentDownloaded
				torrent.Uploaded = currentUploaded

				// Estimate ETA
				if downloadSpeedKBps > 0 {
					remainingBytes := torrent.Size - currentDownloaded
					etaSeconds := float64(remainingBytes) / (downloadSpeedKBps * 1024)
					torrent.ETA = fmt.Sprintf("%s", time.Duration(etaSeconds)*time.Second)
				} else {
					torrent.ETA = "∞"
				}

				torrent.Status = "Downloading"

				// Update UI
				uiUpdateChan <- func() {
					torrent.mu.Lock()
					defer torrent.mu.Unlock()
					if torrent.removed {
						return
					}
					torrentListBinding.Set(torrentListToAnySlice(torrentList))
					torrentListView.Refresh()
				}

			case <-torrent.ctx.Done():
				return
			}
		}
	}(torrentItem)
	go func() {
		// Get peers from trackers
		allPeers, err := tracker.GetAllPeers(torrentItem.ctx, &mi)
		if err != nil {
			if err == context.Canceled {
				log.Infof("Peer fetching canceled for torrent %s", torrentItem.Name)
				return
			}
			uiUpdateChan <- func() {
				ShowError("Failed to get peers from any tracker", err, myWindow)
			}
			return
		}

		uniquePeers := peer.RemoveDuplicatePeers(allPeers)

		// Limit the number of connections based on user settings
		maxPeers := maxConnections
		if len(uniquePeers) < maxPeers {
			maxPeers = len(uniquePeers)
		}

		var wg sync.WaitGroup
		for i := 0; i < maxPeers; i++ {
			wg.Add(1)
			go func(peerHash []byte, index int) {
				defer wg.Done()
				peer.RetryConnect(torrentItem.ctx, peerHash, index, &mi, torrentItem.dm, torrentItem.pm, maxRetries, initialDelay)
			}(uniquePeers[i], i)
		}

		wg.Wait()

		uiUpdateChan <- func() {
			if torrentItem.dm.IsFinished() {
				dialog.ShowInformation("Download Complete", fmt.Sprintf("Downloaded %s successfully.", torrentItem.Name), myWindow)
				torrentItem.Status = "Seeding"
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:   "Download Complete",
					Content: fmt.Sprintf("Downloaded %s successfully.", torrentItem.Name),
				})
				torrentItem.Status = "Seeding"
			} else {
				dialog.ShowInformation("Download Incomplete", "The download did not complete successfully.", myWindow)
				torrentItem.Status = "Incomplete"
				fyne.CurrentApp().SendNotification(&fyne.Notification{
					Title:   "Download Incomplete",
					Content: fmt.Sprintf("%s did not complete successfully.", torrentItem.Name),
				})
			}
		}
		torrentListBinding.Set(torrentListToAnySlice(torrentList))
		torrentListView.Refresh()
	}()
}

// showTorrentDetails displays detailed information about a torrent
func showTorrentDetails(torrent *TorrentItem, myWindow fyne.Window) {
	// Create a new window or dialog to show torrent details
	dialog.ShowCustom(fmt.Sprintf("Details - %s", torrent.Name), "Close", widget.NewLabel("Torrent details here."), myWindow)
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

// gui.go (continued)

// createSAMTab creates the SAM configuration tab.
func createSAMTab() fyne.CanvasObject {
	if samTabInstance == nil {
		samTabInstance = &SAMTab{
			connected:           false,
			samStatusUpdateChan: make(chan bool),
		}
	}

	samTab := samTabInstance

	// Configuration Form Fields
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

	// Initially disable custom settings fields
	inboundLengthEntry.Disable()
	outboundLengthEntry.Disable()
	inboundQuantityEntry.Disable()
	outboundQuantityEntry.Disable()
	inboundBackupQuantityEntry.Disable()
	outboundBackupQuantityEntry.Disable()
	inboundLengthVarianceEntry.Disable()
	outboundLengthVarianceEntry.Disable()

	// Enable/Disable custom settings based on mode selection
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

	samTab.configForm = widget.NewForm(
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

	// Control Buttons
	samTab.startButton = widget.NewButton("Start SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if samTab.connected {
			dialog.ShowInformation("SAM Already Running", "SAM session is already connected.", myWindow)
			return
		}

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

		samTab.connected = true
		samTab.currentSAMConfig = cfg

		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Connected")
			log.Info("SAM session started.")
			dialog.ShowInformation("SAM Started", "SAM session has been successfully started.", myWindow)
		}
	})

	samTab.restartButton = widget.NewButton("Restart SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if !samTab.connected {
			dialog.ShowInformation("SAM Not Running", "SAM session is not currently running.", myWindow)
			return
		}

		// Close existing SAM session
		i2p.CloseSAM()
		samTab.connected = false
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Disconnected")
			log.Info("SAM session stopped.")
		}

		// Restart SAM with the current configuration
		err := i2p.InitSAM(samTab.currentSAMConfig)
		if err != nil {
			ShowError("SAM Restart Failed", err, myWindow)
			return
		}

		samTab.connected = true
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Connected")
			log.Info("SAM session restarted.")
		}

		// Notify the user
		dialog.ShowInformation("SAM Restarted", "SAM session has been successfully restarted.", myWindow)
	})

	samTab.stopButton = widget.NewButton("Stop SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if !samTab.connected {
			dialog.ShowInformation("SAM Not Running", "SAM session is not currently running.", myWindow)
			return
		}

		// Close SAM session
		i2p.CloseSAM()
		samTab.connected = false
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Disconnected")
			log.Info("SAM session stopped.")
		}

		// Notify the user
		dialog.ShowInformation("SAM Stopped", "SAM session has been successfully stopped.", myWindow)
	})

	// Status Label
	samTab.statusLabel = widget.NewLabel("Disconnected")
	samTab.statusLabel.Wrapping = fyne.TextWrapWord

	// Layout the SAM Tab
	samTabContent := container.NewVBox(
		widget.NewLabelWithStyle("SAM Configuration", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		samTab.configForm,
		container.NewHBox(
			samTab.startButton,
			samTab.restartButton,
			samTab.stopButton,
		),
		container.NewHBox(
			widget.NewLabelWithStyle("Connection Status:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			samTab.statusLabel,
		),
	)

	// Start a goroutine to monitor SAM status if needed
	go samTab.monitorSAMStatus()

	return container.NewScroll(samTabContent)
}

// monitorSAMStatus listens for status updates and updates the UI accordingly.
func (samTab *SAMTab) monitorSAMStatus() {
	for status := range samTab.samStatusUpdateChan {
		if status {
			uiUpdateChan <- func() {
				samTab.statusLabel.SetText("Connected")
			}
		} else {
			uiUpdateChan <- func() {
				samTab.statusLabel.SetText("Disconnected")
			}
		}
	}
}

// drawLine draws a line on an image using Bresenham's algorithm
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if x0 >= 0 && x0 < img.Rect.Max.X && y0 >= 0 && y0 < img.Rect.Max.Y {
			img.Set(x0, y0, col)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func saveLogsToFile() {
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
}

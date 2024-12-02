package gui

import (
	"bytes"
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
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"strconv"
	"sync"
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
	trackerListBinding = binding.NewUntypedList()
)

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetOutput(io.MultiWriter(os.Stderr, &logBuffer))
	log.SetLevel(logrus.DebugLevel)

	if trackerManager := tracker.GlobalTrackerManager; trackerManager != nil {
		trackers := trackerManager.GetTrackers()
		trackerListBinding.Set(trackersToAnySlice(trackers))
	}
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
	menuItems := []string{"Settings", "SAM", "Torrents", "Uploads", "Peers", "Logs", "Metrics", "Trackers"}

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
		case "Trackers":
			mainContent.Objects = []fyne.CanvasObject{createTrackersTab()}
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
		showAddTorrentOptionsDialog(myWindow, downloadDirEntry, maxConnectionsEntry)
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

// showAddFromFileDialog opens the file dialog to select a torrent file
func showAddFromFileDialog(parent fyne.Window, downloadDirEntry *widget.Entry, maxConnectionsEntry *widget.Entry) {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}

		torrentFilePath := reader.URI().Path()
		reader.Close()

		// Validate and apply settings
		downloadDir := downloadDirEntry.Text
		if downloadDir == "" {
			ShowError("Invalid Settings", fmt.Errorf("Please select a download directory"), parent)
			return
		}

		maxConnections, err := strconv.Atoi(maxConnectionsEntry.Text)
		if err != nil || maxConnections <= 0 {
			ShowError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), parent)
			return
		}

		// Start the torrent download
		addTorrent(torrentFilePath, downloadDir, maxConnections)
	}, parent)
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

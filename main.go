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
	"eeptorrent/lib/gui"
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
	"github.com/wcharczuk/go-chart"
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

var (
	log          = logrus.StandardLogger()
	maxRetries   = 20
	initialDelay = 2 * time.Second
	logFile      *os.File
	logFileMux   sync.Mutex
	logBuffer    bytes.Buffer
)

// ChartData holds the data points for the chart.
type ChartData struct {
	DownloadSpeed []float64
	UploadSpeed   []float64
	MaxPoints     int
	mu            sync.Mutex
}

// NewChartData initializes a new ChartData instance with initial data points.
func NewChartData(maxPoints int) *ChartData {
	return &ChartData{
		DownloadSpeed: []float64{10, 20, 30}, // Initial non-zero download speeds
		UploadSpeed:   []float64{5, 15, 25},  // Initial non-zero upload speeds
		MaxPoints:     maxPoints,
	}
}

// AddPoint adds new data points to the chart.
func (cd *ChartData) AddPoint(download, upload float64) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if len(cd.DownloadSpeed) >= cd.MaxPoints {
		cd.DownloadSpeed = cd.DownloadSpeed[1:]
		cd.UploadSpeed = cd.UploadSpeed[1:]
	}
	cd.DownloadSpeed = append(cd.DownloadSpeed, download)
	cd.UploadSpeed = append(cd.UploadSpeed, upload)
}

// GenerateChartPNG creates a PNG chart from the current data.
func (cd *ChartData) GenerateChartPNG() ([]byte, error) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	// Prepare data
	xValues := generateXValues(len(cd.DownloadSpeed))
	downloadSeries := chart.ContinuousSeries{
		Name:    "Download Speed",
		XValues: xValues,
		YValues: cd.DownloadSpeed,
	}
	uploadSeries := chart.ContinuousSeries{
		Name:    "Upload Speed",
		XValues: xValues,
		YValues: cd.UploadSpeed,
	}

	// Create the chart
	graph := chart.Chart{
		Series: []chart.Series{
			downloadSeries,
			uploadSeries,
		},
		XAxis: chart.XAxis{
			Name:      "Time (s)",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
		},
		YAxis: chart.YAxis{
			Name:      "Speed (KB/s)",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
		},
		/*
			Title: chart.Title{
				Text: "Download & Upload Speed",
			},

		*/
	}

	// Render the chart to a buffer
	var buf bytes.Buffer
	err := graph.Render(chart.PNG, &buf)
	if err != nil {
		log.Printf("Failed to render chart: %v", err)
		return nil, err
	}

	// Optionally, write the PNG to a file for debugging
	err = os.WriteFile("generated_chart.png", buf.Bytes(), 0644)
	if err != nil {
		log.Printf("Error writing PNG to file: %v", err)
	}

	return buf.Bytes(), nil
}

// Update the metricsContent with the latest chart
func updateMetricsChart(chartData *ChartData, chartImage *canvas.Image, myApp fyne.App) {
	pngBytes, err := chartData.GenerateChartPNG()
	if err != nil {
		log.Printf("Error generating chart PNG: %v", err)
		return
	}

	if len(pngBytes) == 0 {
		log.Printf("Generated PNG bytes are empty")
		return
	}

	// Create a Fyne resource from the image
	resource := fyne.NewStaticResource("chart.png", pngBytes)

	// Queue the UI update on the main thread

	chartImage.Resource = resource
	chartImage.Refresh()

}

// generateXValues generates X-axis labels based on the number of points.
func generateXValues(numPoints int) []float64 {
	x := make([]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		x[i] = float64(i)
	}
	return x
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

func main() {
	err := i2p.InitSAM()
	if err != nil {
		log.Fatalf("Failed to initialize SAM: %v", err)
	}
	defer i2p.CloseSAM()
	// Initialize the Fyne application with a unique ID to satisfy Preferences API
	myApp := app.NewWithID("com.i2p.EepTorrent")
	myApp.SetIcon(logo.ResourceLogo32Png)

	myWindow := myApp.NewWindow("EepTorrent")

	gui.ShowDisclaimer(myApp, myWindow)

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
	/*
		go func() {
			for {
				time.Sleep(1 * time.Second)
				logFileMux.Lock()
				logsContent.SetText(logBuffer.String())
				logFileMux.Unlock()
			}
		}()

	*/
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
						gui.ShowError("Save Logs Error", err, myWindow)
						return
					}
					if writer == nil {
						// User canceled the dialog
						return
					}
					logFilePath := writer.URI().Path()
					writer.Close() // Close immediately after getting the path

					if logFilePath == "" {
						gui.ShowError("Invalid File Path", fmt.Errorf("No file path selected"), myWindow)
						return
					}

					// Open the selected log file
					file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					if err != nil {
						gui.ShowError("Failed to Open Log File", err, myWindow)
						return
					}

					// Safely update the log output
					logFileMux.Lock()
					defer logFileMux.Unlock()

					// Write the contents of logBuffer to the file
					_, err = file.Write(logBuffer.Bytes())
					if err != nil {
						gui.ShowError("Failed to Write Logs to File", err, myWindow)
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
				gui.ShowAboutDialog(myApp, myWindow)
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
					updateMetricsChart(chartData, chartImage, myApp)
				}
			}
		}()

	*/

	startButton.OnTapped = func() {
		if downloadInProgress {
			return
		}

		// Validate and apply settings
		downloadDir := downloadDirEntry.Text
		if downloadDir == "" {
			gui.ShowError("Invalid Settings", fmt.Errorf("Please select a download directory"), myWindow)
			return
		}

		maxConnections, err := strconv.Atoi(maxConnectionsEntry.Text)
		if err != nil || maxConnections <= 0 {
			gui.ShowError("Invalid Settings", fmt.Errorf("Max Connections must be a positive integer"), myWindow)
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

				// Load the torrent file
				mi, err := metainfo.LoadFromFile(torrentFilePath)
				if err != nil {
					gui.ShowError("Failed to load torrent", err, myWindow)
					return
				}

				info, err := mi.Info()
				if err != nil {
					gui.ShowError("Failed to parse torrent info", err, myWindow)
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
						gui.ShowError("Failed to create output directory", err, myWindow)
						return
					}
				}

				writer := metainfo.NewWriter(outputPath, info, mode)
				dm = download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, len(info.Pieces))
				dm.DownloadDir = downloadDir
				progressTicker := time.NewTicker(1 * time.Second)
				ctx, cancel := context.WithCancel(context.Background())
				downloadCancel = cancel

				// Start the listener for incoming connections (seeding)
				/*
					go func() {
						err := startPeerListener(dm, &mi)
						if err != nil {
							log.WithError(err).Error("Failed to start peer listener")
						}
					}()

				*/

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
							updateMetricsChart(chartData, chartImage, myApp)

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
				time.Sleep(1 * time.Second)

				peersSimp, err := tracker.GetPeersFromSimpTracker(&mi)
				if err != nil {
					log.WithError(err).Warn("Failed to get peers from Simp Tracker")
				} else {
					allPeers = append(allPeers, peersSimp...)
				}
				time.Sleep(1 * time.Second)

				peersDg2, err := tracker.GetPeersFromDg2Tracker(&mi)
				if err != nil {
					log.WithError(err).Warn("Failed to get peers from Dg2 Tracker")
				} else {
					allPeers = append(allPeers, peersDg2...)
				}
				time.Sleep(1 * time.Second)

				peersSkank, err := tracker.GetPeersFromSkankTracker(&mi)
				if err != nil {
					log.WithError(err).Warn("Failed to get peers from Skank Tracker")
				} else {
					allPeers = append(allPeers, peersSkank...)
				}
				time.Sleep(1 * time.Second)

				if len(allPeers) == 0 {
					gui.ShowError("Failed to get peers from any tracker", fmt.Errorf("No peers found"), myWindow)
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

// In startPeerListener function
func startPeerListener(dm *download.DownloadManager, mi *metainfo.MetaInfo) error {
	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		return fmt.Errorf("Failed to generate keys for listener session: %v", err)
	}
	sessionName := fmt.Sprintf("EepTorrent-listenerSession-%d", os.Getpid())
	listenerSession, err := i2p.GlobalSAM.NewStreamSession(sessionName, keys, sam3.Options_Default)
	if err != nil {
		return fmt.Errorf("Failed to create listener session: %v", err)
	}

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

	peerIDMeta := util.GeneratePeerIdMeta()

	err := peer.PerformHandshake(conn, mi.InfoHash().Bytes(), string(peerIDMeta[:]))
	if err != nil {
		log.WithError(err).Error("Handshake failed with incoming peer")
		return
	}

	pc := pp.NewPeerConn(conn, peerIDMeta, mi.InfoHash())
	pc.Timeout = 30 * time.Second

	dm.AddPeer(pc)
	defer dm.RemovePeer(pc)

	err = peer.HandlePeerConnection(context.Background(), pc, dm)
	if err != nil {
		log.WithError(err).Error("Error handling incoming peer connection")
	}
}

package gui

import (
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
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

// createTorrentListView creates the torrent list view
func createTorrentListView() *widget.List {
	torrentListView = widget.NewListWithData(
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

// showAddTorrentOptionsDialog displays a popup with options to add a torrent
func showAddTorrentOptionsDialog(parent fyne.Window, downloadDirEntry *widget.Entry, maxConnectionsEntry *widget.Entry) {
	// Declare the dialog variable
	var optionsDialog *dialog.CustomDialog

	// Create buttons for each option
	addFromFileButton := widget.NewButton("Add from File", func() {
		if optionsDialog != nil {
			optionsDialog.Hide() // Properly hide the dialog
		}
		// Trigger the existing Add from File functionality
		showAddFromFileDialog(parent, downloadDirEntry, maxConnectionsEntry)
	})

	addMagnetLinkButton := widget.NewButton("Add Magnet Link", func() {
		if optionsDialog != nil {
			optionsDialog.Hide() // Properly hide the dialog
		}
		// Show a stub dialog indicating the feature is not implemented
		dialog.ShowInformation("Not Implemented", "Adding torrents via Magnet Link is not yet implemented.", parent)
	})

	addHTTPSLinkButton := widget.NewButton("Add HTTPS Link", func() {
		if optionsDialog != nil {
			optionsDialog.Hide() // Properly hide the dialog
		}
		// Show a stub dialog indicating the feature is not implemented
		dialog.ShowInformation("Not Implemented", "Adding torrents via HTTPS Link is not yet implemented.", parent)
	})

	// Create a container for the buttons
	optionsContainer := container.NewVBox(
		addFromFileButton,
		addMagnetLinkButton,
		addHTTPSLinkButton,
	)

	// Create and show the custom dialog
	optionsDialog = dialog.NewCustom("Add Torrent", "Close", optionsContainer, parent)
	optionsDialog.Resize(fyne.NewSize(300, 150))
	optionsDialog.Show()
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

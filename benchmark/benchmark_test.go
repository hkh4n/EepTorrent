package benchmark

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
	"crypto/sha1"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBench1MB(t *testing.T) {
	// Initialize logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)
	log.Println("Starting Bench1MB test")

	// Initialize SAM
	err := i2p.InitSAM(i2p.DefaultSAMConfig())
	assert.NoError(t, err, "Failed to initialize SAM")
	defer i2p.CloseSAM()

	// Load the test torrent file
	torrentPath := filepath.Join("testdata", "test_1mb.txt.torrent")
	mi, err := metainfo.LoadFromFile(torrentPath)
	assert.NoError(t, err, "Failed to load test torrent")

	info, err := mi.Info()
	assert.NoError(t, err, "Failed to parse torrent info")

	// Create a temporary directory for downloading
	downloadDir, err := ioutil.TempDir("", "eeptorrent-bench1mb-download")
	//downloadDir := os.TempDir()
	assert.NoError(t, err, "Failed to create temporary download directory")
	defer os.RemoveAll(downloadDir)

	// Determine the path for the downloaded file
	var downloadedFilePath string
	if len(info.Files) == 0 {
		// Single-file torrent
		downloadedFilePath = filepath.Join(downloadDir, info.Name)
	} else {
		// Multi-file torrent (for simplicity, assume single file in test)
		// Adjust accordingly if testing multi-file torrents
		downloadedFilePath = filepath.Join(downloadDir, info.Name, info.Files[0].Path(info))
		// Create necessary directories
		err := os.MkdirAll(filepath.Dir(downloadedFilePath), 0755)
		assert.NoError(t, err, "Failed to create directories for multi-file torrent")
	}

	// Initialize the file writer for downloading
	writer := metainfo.NewWriter(downloadedFilePath, info, 0644)
	defer func() {
		err := writer.Close()
		if err != nil {
			log.Printf("Failed to close writer: %v", err)
		}
	}()

	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, info.CountPieces(), downloadDir)
	assert.NotNil(t, dm, "Failed to initialize DownloadManager")
	//defer dm.Shutdown()

	pm := peer.NewPeerManager(dm)
	assert.NotNil(t, pm, "Failed to initialize PeerManager")
	//defer pm.Shutdown()

	log.Println("Fetching peers from EepTorrent Tracker")
	peers, err := tracker.GetPeersFromEepTorrentTracker(&mi)
	assert.NoError(t, err, "Failed to get peers from EepTorrent Tracker")
	assert.NotEmpty(t, peers, "No peers returned from EepTorrent Tracker")
	log.Printf("Fetched %d peers from EepTorrent Tracker", len(peers))

	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Start connecting to peers
	for index, peerHash := range peers {

		if index >= 50 {
			break
		}

		wg.Add(1)
		go func(ph []byte, idx int) {
			defer wg.Done()
			err := peer.ConnectToPeer(ctx, ph, idx, &mi, dm)
			if err != nil {
				log.Printf("Failed to connect to peer %d (%x): %v", idx, ph, err)
			} else {
				log.Printf("Successfully connected to peer %d (%x)", idx, ph)
			}
		}(peerHash, index)
	}

	// Start monitoring download progress in a separate goroutine
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Context cancelled, stopping download monitoring")
				close(progressDone)
				return
			case <-ticker.C:
				progress := dm.Progress()
				downloaded := atomic.LoadInt64(&dm.Downloaded)
				left := atomic.LoadInt64(&dm.Left)
				log.Printf("Download Progress: %.2f%%, Downloaded: %d bytes, Left: %d bytes", progress, downloaded, left)

				if dm.IsFinished() {
					log.Println("Download completed successfully")
					// Cancel the context to stop peer connections
					cancel()
					close(progressDone)
					return
				}
			}
		}
	}()

	// Wait for either download completion or context timeout
	select {
	case <-progressDone:
		// Proceed to verification
	case <-ctx.Done():
		if dm.IsFinished() {
			log.Println("Download completed successfully after timeout")
		} else {
			t.Fatal("Benchmark timed out before download completion")
		}
	}

	// Wait for all peer connection attempts to finish
	dm.Shutdown()
	pm.Shutdown()
	log.Println("Waiting for peer connections to finish...")
	wg.Wait()

	// Final verification of the downloaded file
	if len(info.Files) == 0 {
		// Single-file torrent
		assert.FileExists(t, downloadedFilePath, "Downloaded file does not exist")

		downloadedData, err := ioutil.ReadFile(downloadedFilePath)
		assert.NoError(t, err, "Failed to read downloaded file")

		expectedSize := info.TotalLength()
		actualSize := int64(len(downloadedData))
		assert.Equal(t, expectedSize, actualSize, "Downloaded file size does not match expected size")

		// Verify SHA1 hash of the downloaded data
		actualHash := sha1.Sum(downloadedData)
		expectedHash := info.Pieces[0][:]
		assert.True(t, bytes.Equal(expectedHash, actualHash[:]), "Downloaded file hash does not match expected hash")
		log.Println("Downloaded file verified successfully")
	} else {
		// Multi-file torrent (not implemented in this test)
		t.Skip("Multi-file torrent verification not implemented in TestBench1MB")
	}

	t.Log("TestBench1MB completed successfully")
}

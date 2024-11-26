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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)
	log.Println("Starting Bench1MB test")

	err := i2p.InitSAM(i2p.DefaultSAMConfig())
	assert.NoError(t, err, "Failed to initialize SAM")
	defer i2p.CloseSAM()

	torrentPath := filepath.Join("testdata", "test_1mb.txt.torrent")
	mi, err := metainfo.LoadFromFile(torrentPath)
	assert.NoError(t, err, "Failed to load test torrent")

	info, err := mi.Info()
	assert.NoError(t, err, "Failed to parse torrent info")

	// Create a temporary directory for downloading
	downloadDir, err := ioutil.TempDir("", "eeptorrent-bench1mb-download")
	assert.NoError(t, err, "Failed to create temporary download directory")
	defer os.RemoveAll(downloadDir)

	var downloadedFilePath string
	if len(info.Files) == 0 {
		downloadedFilePath = filepath.Join(downloadDir, info.Name)
	} else {
		downloadedFilePath = filepath.Join(downloadDir, info.Name, info.Files[0].Path(info))
		err := os.MkdirAll(filepath.Dir(downloadedFilePath), 0755)
		assert.NoError(t, err, "Failed to create directories for multi-file torrent")
	}

	writer := metainfo.NewWriter(downloadedFilePath, info, 0644)
	defer func() {
		err := writer.Close()
		assert.NoError(t, err, "Failed to close writer")
	}()

	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, info.CountPieces(), downloadDir)
	assert.NotNil(t, dm, "Failed to initialize DownloadManager")

	pm := peer.NewPeerManager(dm)
	assert.NotNil(t, pm, "Failed to initialize PeerManager")

	log.Println("Fetching peers from EepTorrent Tracker")
	peers, err := tracker.GetPeersFromEepTorrentTracker(&mi)
	assert.NoError(t, err, "Failed to get peers from EepTorrent Tracker")
	assert.NotEmpty(t, peers, "No peers returned from EepTorrent Tracker")
	log.Printf("Fetched %d peers from EepTorrent Tracker", len(peers))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	downloadComplete := make(chan struct{})

	// Progress monitoring
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer close(downloadComplete)

		startTime := time.Now()
		var lastDownloaded int64

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentDownloaded := atomic.LoadInt64(&dm.Downloaded)
				downloadedDiff := currentDownloaded - lastDownloaded
				speed := float64(downloadedDiff) / 5.0
				progress := dm.Progress()
				left := atomic.LoadInt64(&dm.Left)
				elapsed := time.Since(startTime)

				log.Printf("Progress: %.2f%%, Speed: %.2f KB/s, Downloaded: %d bytes, Left: %d bytes, Elapsed: %v",
					progress, speed/1024.0, currentDownloaded, left, elapsed.Round(time.Second))

				lastDownloaded = currentDownloaded

				if dm.IsFinished() {
					log.Printf("Download completed, verifying pieces...")
					// Verify all pieces before declaring completion
					for i := 0; i < info.CountPieces(); i++ {
						if !dm.VerifyPiece(uint32(i)) {
							log.Printf("Piece %d verification failed", i)
							return
						}
					}
					log.Printf("Download completed in %v", elapsed.Round(time.Second))
					return
				}
			}
		}
	}()

	// Start peer connections with timeout handling
	peerCtx, peerCancel := context.WithCancel(ctx)
	defer peerCancel()

	maxPeers := 50
	if len(peers) < maxPeers {
		maxPeers = len(peers)
	}

	for i := 0; i < maxPeers; i++ {
		wg.Add(1)
		go func(peerHash []byte, idx int) {
			defer wg.Done()
			peer.ConnectToPeer(peerCtx, peerHash, idx, &mi, dm)
		}(peers[i], i)
	}

	// Wait for either completion or timeout
	select {
	case <-downloadComplete:
		log.Println("Download completed successfully")
	case <-ctx.Done():
		// Cancel peer connections first
		peerCancel()
		if !dm.IsFinished() {
			t.Fatal("Test timed out before download completion")
		}
	}

	// Clean shutdown with timeout
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()

	// Cancel remaining peer connections
	peerCancel()

	// Wait for connections with timeout
	cleanupDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(cleanupDone)
	}()

	select {
	case <-cleanupDone:
		log.Println("All peer connections cleaned up")
	case <-cleanupCtx.Done():
		t.Log("Warning: Cleanup timed out")
	}

	pm.Shutdown()
	dm.Shutdown()

	// Verify download
	if len(info.Files) == 0 {
		// Verify single file
		assert.FileExists(t, downloadedFilePath, "Downloaded file does not exist")

		downloadedData, err := ioutil.ReadFile(downloadedFilePath)
		assert.NoError(t, err, "Failed to read downloaded file")

		// Verify size
		expectedSize := info.TotalLength()
		actualSize := int64(len(downloadedData))
		assert.Equal(t, expectedSize, actualSize, "Downloaded file size mismatch")

		// Verify hash
		actualHash := sha1.Sum(downloadedData)
		expectedHash := info.Pieces[0][:]
		assert.True(t, bytes.Equal(expectedHash, actualHash[:]),
			"Downloaded file hash mismatch. Expected: %x, Got: %x", expectedHash, actualHash[:])

		log.Printf("File verification successful. Size: %d bytes", actualSize)
	} else {
		// Multi-file verification not implemented
		t.Skip("Multi-file torrent verification not implemented")
	}

	log.Println("TestBench1MB completed successfully")
}

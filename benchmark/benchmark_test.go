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
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var log = logrus.StandardLogger()

func TestBench1MB(t *testing.T) {
	//log.SetFlags(log.LstdFlags | log.Lshortfile)
	//log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic: %v", r)
			i2p.Cleanup()
			os.Exit(1)
		}
	}()
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Received interrupt signal")
		i2p.Cleanup()
		os.Exit(0)
	}()

	log.Println("Starting Bench1MB test")

	err := i2p.InitSAM(i2p.DefaultSAMConfig())
	assert.NoError(t, err, "Failed to initialize SAM")
	defer i2p.CloseSAM()

	torrentPath := filepath.Join("testdata", "test_1mb.txt.torrent")
	mi, err := metainfo.LoadFromFile(torrentPath)
	assert.NoError(t, err, "Failed to load test torrent")

	info, err := mi.Info()
	assert.NoError(t, err, "Failed to parse torrent info")

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
	defer writer.Close()

	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, info.CountPieces(), downloadDir)
	assert.NotNil(t, dm, "Failed to initialize DownloadManager")

	pm := peer.NewPeerManager(dm)
	assert.NotNil(t, pm, "Failed to initialize PeerManager")

	log.Println("Fetching peers from EepTorrent Tracker")
	peers, err := tracker.GetPeersFromEepTorrentTracker(&mi)
	assert.NoError(t, err, "Failed to get peers from EepTorrent Tracker")
	assert.NotEmpty(t, peers, "No peers returned from EepTorrent Tracker")
	log.Printf("Fetched %d peers from EepTorrent Tracker", len(peers))

	// Create context with proper timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	downloadComplete := make(chan struct{})

	// Start progress monitoring
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
	maxPeers := 50
	if len(peers) < maxPeers {
		maxPeers = len(peers)
	}

	for i := 0; i < maxPeers; i++ {
		wg.Add(1)
		go func(peerHash []byte, idx int) {
			defer wg.Done()
			peer.ConnectToPeer(ctx, peerHash, idx, &mi, dm)
		}(peers[i], i)
	}

	// Wait for either completion or timeout
	select {
	case <-downloadComplete:
		log.Println("Download completed successfully")
	case <-ctx.Done():
		if !dm.IsFinished() {
			t.Fatal("Test timed out before download completion")
		}
	}

	log.Println("Starting cleanup sequence...")

	// Signal all goroutines to stop
	cancel()

	// Create cleanup context with shorter timeout
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()

	// Close all connections with timeout
	go func() {
		// Get snapshot of active peers
		activePeers := dm.GetAllPeers()
		log.Printf("Forcibly closing %d peer connections...", len(activePeers))

		// Close each connection with timeout
		for _, p := range activePeers {
			if p != nil {
				p.Close()
				if conn, ok := p.Conn.(net.Conn); ok {
					conn.Close()
				}
			}
		}
	}()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Use select to handle cleanup timeout
	select {
	case <-done:
		log.Println("All peer connections closed successfully")
	case <-cleanupCtx.Done():
		log.Println("Warning: Some connections may not have closed cleanly")
	}

	// Shutdown managers
	log.Println("Shutting down managers...")
	pm.Shutdown()
	dm.Shutdown()

	log.Println("Closing file writer...")
	writer.Close()

	time.Sleep(1 * time.Second)

	// Final verification
	log.Println("Starting final verification...")
	if len(info.Files) == 0 {
		assert.FileExists(t, downloadedFilePath)

		// Read and verify file
		downloadedData, err := ioutil.ReadFile(downloadedFilePath)
		assert.NoError(t, err, "Failed to read downloaded file")

		expectedSize := info.TotalLength()
		actualSize := int64(len(downloadedData))
		assert.Equal(t, expectedSize, actualSize, "File size mismatch")

		// Verify hash of entire file
		actualHash := sha1.Sum(downloadedData)
		expectedHash := info.Pieces[0][:]
		if !bytes.Equal(expectedHash, actualHash[:]) {
			log.Printf("First 32 bytes of file:\t %x", downloadedData[:32])
			log.Printf("Last 32 bytes of file:\t %x", downloadedData[len(downloadedData)-32:])
			t.Errorf("Hash mismatch: Expected %x, got %x", expectedHash, actualHash[:])
		} else {
			log.Printf("File verification successful. Size: %d bytes", actualSize)
		}
	} else {
		t.Skip("Multi-file torrent verification not implemented")
	}

	log.Println("TestBench1MB completed")
}

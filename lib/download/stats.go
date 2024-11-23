package download

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
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

// Add this struct to track download statistics
type DownloadStats struct {
	StartTime            time.Time
	LastProgressUpdate   time.Time
	TotalBytesReceived   int64
	CurrentDownloadSpeed float64
	PeakDownloadSpeed    float64
	TotalBytesUploaded   int64
	CurrentUploadSpeed   float64
	PeakUploadSpeed      float64
	ActiveConnections    int
	mu                   sync.Mutex
}

func NewDownloadStats() *DownloadStats {
	log.Debug("Initializing DownloadStats")
	return &DownloadStats{
		StartTime:          time.Now(),
		LastProgressUpdate: time.Now(),
	}
}

// UpdateProgress updates both download and upload statistics
func (stats *DownloadStats) UpdateProgress(bytesReceived int64, bytesUploaded int64) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	now := time.Now()
	duration := now.Sub(stats.LastProgressUpdate).Seconds()
	if duration > 0 {
		stats.CurrentDownloadSpeed = float64(bytesReceived) / duration
		stats.CurrentUploadSpeed = float64(bytesUploaded) / duration
		if stats.CurrentDownloadSpeed > stats.PeakDownloadSpeed {
			stats.PeakDownloadSpeed = stats.CurrentDownloadSpeed
		}
		if stats.CurrentUploadSpeed > stats.PeakUploadSpeed {
			stats.PeakUploadSpeed = stats.CurrentUploadSpeed
		}
	}

	stats.TotalBytesReceived += bytesReceived
	stats.TotalBytesUploaded += bytesUploaded
	stats.LastProgressUpdate = now

	logrus.WithFields(logrus.Fields{
		"current_download_speed_kBps": stats.CurrentDownloadSpeed / 1024,
		"peak_download_speed_kBps":    stats.PeakDownloadSpeed / 1024,
		"current_upload_speed_kBps":   stats.CurrentUploadSpeed / 1024,
		"peak_upload_speed_kBps":      stats.PeakUploadSpeed / 1024,
		"total_received_MB":           float64(stats.TotalBytesReceived) / 1024 / 1024,
		"total_uploaded_MB":           float64(stats.TotalBytesUploaded) / 1024 / 1024,
		"elapsed_time":                now.Sub(stats.StartTime).String(),
		"active_connections":          stats.ActiveConnections,
	}).Info("Download and Upload statistics update")
}

func (stats *DownloadStats) ConnectionStarted() {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	stats.ActiveConnections++
	log.WithField("active_connections", stats.ActiveConnections).Debug("Peer connection started")
}

func (stats *DownloadStats) ConnectionEnded() {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	stats.ActiveConnections--
	log.WithField("active_connections", stats.ActiveConnections).Debug("Peer connection ended")
}

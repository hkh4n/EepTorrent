package download

/*
An I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan
Copyright (C) 2024 The EepTorrent Developers

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
	StartTime          time.Time
	LastProgressUpdate time.Time
	TotalBytesReceived int64
	CurrentSpeed       float64
	PeakSpeed          float64
	ActiveConnections  int
	mu                 sync.Mutex
}

func NewDownloadStats() *DownloadStats {
	return &DownloadStats{
		StartTime:          time.Now(),
		LastProgressUpdate: time.Now(),
	}
}

func (stats *DownloadStats) UpdateProgress(bytesReceived int64) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	now := time.Now()
	duration := now.Sub(stats.LastProgressUpdate).Seconds()
	if duration > 0 {
		stats.CurrentSpeed = float64(bytesReceived) / duration
		if stats.CurrentSpeed > stats.PeakSpeed {
			stats.PeakSpeed = stats.CurrentSpeed
		}
	}

	stats.TotalBytesReceived += bytesReceived
	stats.LastProgressUpdate = now

	log.WithFields(logrus.Fields{
		"current_speed_kBps": stats.CurrentSpeed / 1024,
		"peak_speed_kBps":    stats.PeakSpeed / 1024,
		"total_received_MB":  float64(stats.TotalBytesReceived) / 1024 / 1024,
		"elapsed_time":       now.Sub(stats.StartTime).String(),
		"active_connections": stats.ActiveConnections,
	}).Info("Download statistics update")
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

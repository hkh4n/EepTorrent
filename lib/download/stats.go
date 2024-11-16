package download

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

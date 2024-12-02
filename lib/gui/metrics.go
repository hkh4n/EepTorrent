package gui

import (
	"fyne.io/fyne/v2/canvas"
	"time"
)

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

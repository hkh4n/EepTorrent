package gui

import (
	"bytes"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/wcharczuk/go-chart"
	"os"
	"sync"
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
		log.Debugf("Failed to render chart: %v", err)
		return nil, err
	}

	// Optionally, write the PNG to a file for debugging
	err = os.WriteFile("generated_chart.png", buf.Bytes(), 0644)
	if err != nil {
		log.Debugf("Error writing PNG to file: %v", err)
	}

	return buf.Bytes(), nil
}

// Update the metricsContent with the latest chart
func UpdateMetricsChart(chartData *ChartData, chartImage *canvas.Image, myApp fyne.App) {
	pngBytes, err := chartData.GenerateChartPNG()
	if err != nil {
		log.Debugf("Error generating chart PNG: %v", err)
		return
	}

	if len(pngBytes) == 0 {
		log.Debugf("Generated PNG bytes are empty")
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

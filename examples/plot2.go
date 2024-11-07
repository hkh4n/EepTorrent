package main

import (
	"image"
	"math/rand"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

type ChartData struct {
	values    []float64
	maxPoints int
}

func NewChartData(maxPoints int) *ChartData {
	return &ChartData{
		values:    make([]float64, 0),
		maxPoints: maxPoints,
	}
}

func (cd *ChartData) AddPoint(value float64) {
	cd.values = append(cd.values, value)
	if len(cd.values) > cd.maxPoints {
		cd.values = cd.values[1:]
	}
}

func (cd *ChartData) GetXYs() plotter.XYs {
	points := make(plotter.XYs, len(cd.values))
	for i, v := range cd.values {
		points[i].X = float64(i)
		points[i].Y = v
	}
	return points
}

func generatePlotImage(data *ChartData) (image.Image, error) {
	p := plot.New()
	p.Title.Text = "Download Speed"
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Speed (KB/s)"

	// Create line plot from data
	line, err := plotter.NewLine(data.GetXYs())
	if err != nil {
		return nil, err
	}

	p.Add(line)
	p.Legend.Add("Speed", line)

	// Set axis ranges
	p.X.Min = 0
	p.X.Max = float64(data.maxPoints - 1)
	p.Y.Min = 0
	p.Y.Max = 500 // Adjust based on your expected data range

	// Create canvas
	c := vgimg.New(6*vg.Inch, 4*vg.Inch)
	dc := draw.New(c)
	p.Draw(dc)

	return c.Image(), nil
}

func main() {
	myApp := app.New()
	window := myApp.NewWindow("Real-time Chart")

	// Create chart data with 30 points maximum
	data := NewChartData(30)

	// Initial image
	img, _ := generatePlotImage(data)
	chartImage := canvas.NewImageFromImage(img)
	chartImage.FillMode = canvas.ImageFillOriginal
	chartImage.Resize(fyne.NewSize(600, 400))

	// Create container
	content := container.NewVBox(chartImage)

	// Update function
	updateChart := func() {
		// Simulate new data (replace with your actual data source)
		newValue := 100 + rand.Float64()*300
		data.AddPoint(newValue)

		// Generate new image
		newImg, err := generatePlotImage(data)
		if err != nil {
			return
		}

		// Update image
		chartImage.Image = newImg
		chartImage.Refresh()
	}

	// Start update loop
	go func() {
		for range time.Tick(time.Second) {
			updateChart()
		}
	}()

	window.SetContent(content)
	window.Resize(fyne.NewSize(800, 600))
	window.ShowAndRun()
}

package main

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

func generatePlotImage() (image.Image, error) {
	p := plot.New()
	p.Title.Text = "Download Speed"
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Speed (KB/s)"

	// Sample data
	pts := plotter.XYs{
		{X: 0, Y: 100},
		{X: 1, Y: 200},
		{X: 2, Y: 150},
		{X: 3, Y: 300},
		{X: 4, Y: 250},
	}

	line, err := plotter.NewLine(pts)
	if err != nil {
		return nil, err
	}

	p.Add(line)
	p.Legend.Add("Speed", line)

	// Create canvas
	c := vgimg.New(6*vg.Inch, 4*vg.Inch)
	dc := draw.New(c)
	p.Draw(dc)

	// Convert to image
	img := c.Image()
	return img, nil
}

func main() {
	myApp := app.New()
	window := myApp.NewWindow("Chart Display")

	img, err := generatePlotImage()
	if err != nil {
		panic(err)
	}

	chartImage := canvas.NewImageFromImage(img)
	chartImage.FillMode = canvas.ImageFillOriginal
	chartImage.Resize(fyne.NewSize(600, 400))

	window.SetContent(chartImage)
	window.Resize(fyne.NewSize(800, 600))
	window.ShowAndRun()
}

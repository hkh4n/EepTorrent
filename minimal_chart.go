package main

import (
	"bytes"
	"fmt"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

func main() {
	// Initialize a new line chart
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Test Download & Upload Speed",
			Subtitle: "Minimal Example",
			Left:     "center",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Name: "Time (s)",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Name: "Speed (KB/s)",
		}),
	)

	// Sample data
	xAxis := []string{"0", "1", "2", "3", "4", "5"}
	downloadData := []opts.LineData{
		{Value: 10},
		{Value: 20},
		{Value: 30},
		{Value: 40},
		{Value: 50},
		{Value: 60},
	}
	uploadData := []opts.LineData{
		{Value: 5},
		{Value: 15},
		{Value: 25},
		{Value: 35},
		{Value: 45},
		{Value: 55},
	}

	// Add series to the chart
	line.SetXAxis(xAxis).
		AddSeries("Download Speed", downloadData).
		AddSeries("Upload Speed", uploadData).
		SetSeriesOptions(
			charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true)}),
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(true)}),
		)

	// Render the chart to a buffer
	var buf bytes.Buffer
	err := line.Render(&buf)
	if err != nil {
		fmt.Printf("Failed to render chart: %v\n", err)
		return
	}

	// Check for <svg> tag
	svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
	svgEnd := bytes.LastIndex(buf.Bytes(), []byte("</svg>"))

	if svgStart == -1 || svgEnd == -1 {
		fmt.Println("SVG content not found in rendered chart")
		return
	}

	svgContent := buf.Bytes()[svgStart : svgEnd+len("</svg>")]
	fmt.Printf("Extracted SVG content length: %d bytes\n", len(svgContent))
	fmt.Println("SVG Content:")
	fmt.Println(string(svgContent))
}

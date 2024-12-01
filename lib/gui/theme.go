package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"image/color"
)

// CustomTheme struct embeds the default theme
type CustomTheme struct{}

// Ensure CustomTheme implements fyne.Theme
var _ fyne.Theme = (*CustomTheme)(nil)

// Override the TextSize method to make text smaller
func (t *CustomTheme) TextSize() int {
	return int(theme.TextSize() - 2) // Decrease the default text size by 2
}

// Optionally, override other methods if needed
func (t *CustomTheme) TextFont() fyne.Resource {
	return theme.DefaultTextFont()
}

func (t *CustomTheme) TextBoldFont() fyne.Resource {
	return theme.DefaultTextBoldFont()
}

func (t *CustomTheme) TextItalicFont() fyne.Resource {
	return theme.DefaultTextItalicFont()
}

func (t *CustomTheme) TextBoldItalicFont() fyne.Resource {
	return theme.DefaultTextBoldItalicFont()
}

func (t *CustomTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, v)
}

func (t *CustomTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *CustomTheme) Font(s fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(s)
}

func (t *CustomTheme) Size(n fyne.ThemeSizeName) float32 {
	size := theme.DefaultTheme().Size(n)
	switch n {
	case theme.SizeNameText:
		return size * 0.8
	default:
		return size
	}
}

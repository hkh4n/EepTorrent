package gui

import (
	"eeptorrent/lib/util"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func ShowDisclaimer(app fyne.App, parent fyne.Window) {
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It will have bugs, faulty GUIs and other things. Please note that metrics may be inaccurate as this program is in flux.\nBut at the same time will be updated frequently, check back for updates!"),
		widget.NewLabel("EepTorrent Copyright (C) 2024 Haris Khan\nThis program comes with ABSOLUTELY NO WARRANTY.\nThis is free software, and you are welcome to redistribute it under certain conditions. See COPYING for details."),
	)

	dialog := dialog.NewCustomConfirm(
		"Experimental Software",
		"Accept",
		"Decline",
		disclaimerContent,
		func(accepted bool) {
			if !accepted {
				app.Quit()
			}
		},
		parent,
	)

	dialog.SetDismissText("Decline")
	dialog.Show()
}

func ShowError(title string, err error, parent fyne.Window) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), parent)
}

func ShowAboutDialog(app fyne.App, parent fyne.Window) {
	gitCommitDisplay := util.GitCommit
	dialog.ShowCustom("About EepTorrent", "Close",
		container.NewVBox(
			widget.NewLabelWithStyle("EepTorrent", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel(fmt.Sprintf("Version: %s-%s", util.Version, gitCommitDisplay)),
			widget.NewLabel("A cross-platform I2P-only BitTorrent client."),
			widget.NewLabel("© 2024 Haris Khan"),
		), parent)
}

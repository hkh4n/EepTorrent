package gui

import (
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/util"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"strconv"
)

func ShowDisclaimer(app fyne.App, parent fyne.Window) {
	disclaimerContent := container.NewVBox(
		widget.NewLabelWithStyle("Disclaimer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("EepTorrent is experimental software. It will have bugs, faulty GUIs and other things. Please note that metrics may be inaccurate as this program is in flux.\nThis client cannot seed at this time!\nBut at the same time will be updated frequently, check back for updates!"),
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

// showSAMSettingsDialog displays a popup for SAM configuration and invokes a callback upon completion.
func ShowSAMSettingsDialog(myApp fyne.App, myWindow fyne.Window, onComplete func(success bool, err error)) {
	modeGroup := widget.NewRadioGroup([]string{"Use Default Settings", "Custom Settings"}, func(selected string) {})
	modeGroup.SetSelected("Use Default Settings")

	inboundLengthEntry := widget.NewEntry()
	inboundLengthEntry.SetPlaceHolder("Inbound Length")
	inboundLengthEntry.SetText("1")

	outboundLengthEntry := widget.NewEntry()
	outboundLengthEntry.SetPlaceHolder("Outbound Length")
	outboundLengthEntry.SetText("1")

	inboundQuantityEntry := widget.NewEntry()
	inboundQuantityEntry.SetPlaceHolder("Inbound Quantity")
	inboundQuantityEntry.SetText("3")

	outboundQuantityEntry := widget.NewEntry()
	outboundQuantityEntry.SetPlaceHolder("Outbound Quantity")
	outboundQuantityEntry.SetText("3")

	inboundBackupQuantityEntry := widget.NewEntry()
	inboundBackupQuantityEntry.SetPlaceHolder("Inbound Backup Quantity")
	inboundBackupQuantityEntry.SetText("1")

	outboundBackupQuantityEntry := widget.NewEntry()
	outboundBackupQuantityEntry.SetPlaceHolder("Outbound Backup Quantity")
	outboundBackupQuantityEntry.SetText("1")

	inboundLengthVarianceEntry := widget.NewEntry()
	inboundLengthVarianceEntry.SetPlaceHolder("Inbound Length Variance")
	inboundLengthVarianceEntry.SetText("0")

	outboundLengthVarianceEntry := widget.NewEntry()
	outboundLengthVarianceEntry.SetPlaceHolder("Outbound Length Variance")
	outboundLengthVarianceEntry.SetText("0")

	inboundLengthEntry.Disable()
	outboundLengthEntry.Disable()
	inboundQuantityEntry.Disable()
	outboundQuantityEntry.Disable()
	inboundBackupQuantityEntry.Disable()
	outboundBackupQuantityEntry.Disable()
	inboundLengthVarianceEntry.Disable()
	outboundLengthVarianceEntry.Disable()

	modeGroup.OnChanged = func(selected string) {
		if selected == "Custom Settings" {
			inboundLengthEntry.Enable()
			outboundLengthEntry.Enable()
			inboundQuantityEntry.Enable()
			outboundQuantityEntry.Enable()
			inboundBackupQuantityEntry.Enable()
			outboundBackupQuantityEntry.Enable()
			inboundLengthVarianceEntry.Enable()
			outboundLengthVarianceEntry.Enable()
		} else {
			inboundLengthEntry.Disable()
			outboundLengthEntry.Disable()
			inboundQuantityEntry.Disable()
			outboundQuantityEntry.Disable()
			inboundBackupQuantityEntry.Disable()
			outboundBackupQuantityEntry.Disable()
			inboundLengthVarianceEntry.Disable()
			outboundLengthVarianceEntry.Disable()
		}
	}

	samSettingsForm := widget.NewForm(
		widget.NewFormItem("Mode", modeGroup),
		widget.NewFormItem("Inbound Length", inboundLengthEntry),
		widget.NewFormItem("Outbound Length", outboundLengthEntry),
		widget.NewFormItem("Inbound Quantity", inboundQuantityEntry),
		widget.NewFormItem("Outbound Quantity", outboundQuantityEntry),
		widget.NewFormItem("Inbound Backup Quantity", inboundBackupQuantityEntry),
		widget.NewFormItem("Outbound Backup Quantity", outboundBackupQuantityEntry),
		widget.NewFormItem("Inbound Length Variance", inboundLengthVarianceEntry),
		widget.NewFormItem("Outbound Length Variance", outboundLengthVarianceEntry),
	)

	content := container.NewVBox(
		samSettingsForm,
	)

	dialog.ShowCustomConfirm("SAM Settings", "Save", "Cancel", content, func(confirm bool) {
		if confirm {
			var cfg i2p.SAMConfig
			if modeGroup.Selected == "Use Default Settings" {
				cfg = i2p.DefaultSAMConfig()
			} else {
				// Validate and parse custom settings
				inboundLength, err := strconv.Atoi(inboundLengthEntry.Text)
				if err != nil || inboundLength < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Length must be a non-negative integer"), myWindow)
					return
				}

				outboundLength, err := strconv.Atoi(outboundLengthEntry.Text)
				if err != nil || outboundLength < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Length must be a non-negative integer"), myWindow)
					return
				}

				inboundQuantity, err := strconv.Atoi(inboundQuantityEntry.Text)
				if err != nil || inboundQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Quantity must be a non-negative integer"), myWindow)
					return
				}

				outboundQuantity, err := strconv.Atoi(outboundQuantityEntry.Text)
				if err != nil || outboundQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Quantity must be a non-negative integer"), myWindow)
					return
				}

				inboundBackupQuantity, err := strconv.Atoi(inboundBackupQuantityEntry.Text)
				if err != nil || inboundBackupQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Backup Quantity must be a non-negative integer"), myWindow)
					return
				}

				outboundBackupQuantity, err := strconv.Atoi(outboundBackupQuantityEntry.Text)
				if err != nil || outboundBackupQuantity < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Backup Quantity must be a non-negative integer"), myWindow)
					return
				}

				inboundLengthVariance, err := strconv.Atoi(inboundLengthVarianceEntry.Text)
				if err != nil || inboundLengthVariance < 0 {
					ShowError("Invalid Input", fmt.Errorf("Inbound Length Variance must be a non-negative integer"), myWindow)
					return
				}

				outboundLengthVariance, err := strconv.Atoi(outboundLengthVarianceEntry.Text)
				if err != nil || outboundLengthVariance < 0 {
					ShowError("Invalid Input", fmt.Errorf("Outbound Length Variance must be a non-negative integer"), myWindow)
					return
				}

				cfg = i2p.SAMConfig{
					InboundLength:          inboundLength,
					OutboundLength:         outboundLength,
					InboundQuantity:        inboundQuantity,
					OutboundQuantity:       outboundQuantity,
					InboundBackupQuantity:  inboundBackupQuantity,
					OutboundBackupQuantity: outboundBackupQuantity,
					InboundLengthVariance:  inboundLengthVariance,
					OutboundLengthVariance: outboundLengthVariance,
				}
			}

			// Initialize SAM with the selected configuration
			err := i2p.InitSAM(cfg)
			if err != nil {
				ShowError("SAM Initialization Failed", err, myWindow)
				return
			}

			// Invoke the callback to proceed to the main interface
			onComplete(true, nil)
		} else {
			// Handle cancellation, e.g., exit the application
			dialog.ShowConfirm("Exit Application", "Are you sure you want to exit?", func(confirmed bool) {
				if confirmed {
					onComplete(false, fmt.Errorf("User canceled SAM settings"))
				}
			}, myWindow)
		}
	}, myWindow)
}

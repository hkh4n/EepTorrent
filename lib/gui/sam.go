package gui

import (
	"eeptorrent/lib/i2p"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"strconv"
	"sync"
)

type SAMTab struct {
	configForm          *widget.Form
	startButton         *widget.Button
	restartButton       *widget.Button
	stopButton          *widget.Button
	statusLabel         *widget.Label
	connected           bool
	currentSAMConfig    i2p.SAMConfig
	configLock          sync.Mutex
	samStatusUpdateChan chan bool
}

// createSAMTab creates the SAM configuration tab.
func createSAMTab() fyne.CanvasObject {
	if samTabInstance == nil {
		samTabInstance = &SAMTab{
			connected:           false,
			samStatusUpdateChan: make(chan bool),
		}
	}

	samTab := samTabInstance

	// Configuration Form Fields
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

	// Initially disable custom settings fields
	inboundLengthEntry.Disable()
	outboundLengthEntry.Disable()
	inboundQuantityEntry.Disable()
	outboundQuantityEntry.Disable()
	inboundBackupQuantityEntry.Disable()
	outboundBackupQuantityEntry.Disable()
	inboundLengthVarianceEntry.Disable()
	outboundLengthVarianceEntry.Disable()

	// Enable/Disable custom settings based on mode selection
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

	samTab.configForm = widget.NewForm(
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

	// Control Buttons
	samTab.startButton = widget.NewButton("Start SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if samTab.connected {
			dialog.ShowInformation("SAM Already Running", "SAM session is already connected.", myWindow)
			return
		}

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

		samTab.connected = true
		samTab.currentSAMConfig = cfg

		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Connected")
			log.Info("SAM session started.")
			dialog.ShowInformation("SAM Started", "SAM session has been successfully started.", myWindow)
		}
	})

	samTab.restartButton = widget.NewButton("Restart SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if !samTab.connected {
			dialog.ShowInformation("SAM Not Running", "SAM session is not currently running.", myWindow)
			return
		}

		// Close existing SAM session
		i2p.CloseSAM()
		samTab.connected = false
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Disconnected")
			log.Info("SAM session stopped.")
		}

		// Restart SAM with the current configuration
		err := i2p.InitSAM(samTab.currentSAMConfig)
		if err != nil {
			ShowError("SAM Restart Failed", err, myWindow)
			return
		}

		samTab.connected = true
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Connected")
			log.Info("SAM session restarted.")
		}

		// Notify the user
		dialog.ShowInformation("SAM Restarted", "SAM session has been successfully restarted.", myWindow)
	})

	samTab.stopButton = widget.NewButton("Stop SAM", func() {
		samTab.configLock.Lock()
		defer samTab.configLock.Unlock()

		if !samTab.connected {
			dialog.ShowInformation("SAM Not Running", "SAM session is not currently running.", myWindow)
			return
		}

		// Close SAM session
		i2p.CloseSAM()
		samTab.connected = false
		uiUpdateChan <- func() {
			samTab.statusLabel.SetText("Disconnected")
			log.Info("SAM session stopped.")
		}

		// Notify the user
		dialog.ShowInformation("SAM Stopped", "SAM session has been successfully stopped.", myWindow)
	})

	// Status Label
	samTab.statusLabel = widget.NewLabel("Disconnected")
	samTab.statusLabel.Wrapping = fyne.TextWrapWord

	// Layout the SAM Tab
	samTabContent := container.NewVBox(
		widget.NewLabelWithStyle("SAM Configuration", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		samTab.configForm,
		container.NewHBox(
			samTab.startButton,
			samTab.restartButton,
			samTab.stopButton,
		),
		container.NewHBox(
			widget.NewLabelWithStyle("Connection Status:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			samTab.statusLabel,
		),
	)

	// Start a goroutine to monitor SAM status if needed
	go samTab.monitorSAMStatus()

	return container.NewScroll(samTabContent)
}

// monitorSAMStatus listens for status updates and updates the UI accordingly.
func (samTab *SAMTab) monitorSAMStatus() {
	for status := range samTab.samStatusUpdateChan {
		if status {
			uiUpdateChan <- func() {
				samTab.statusLabel.SetText("Connected")
			}
		} else {
			uiUpdateChan <- func() {
				samTab.statusLabel.SetText("Disconnected")
			}
		}
	}
}

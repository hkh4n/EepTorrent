package gui

import (
	"eeptorrent/lib/tracker"
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"strings"
)

func trackersToAnySlice(trackers []tracker.TrackerConfig) []any {
	items := make([]any, len(trackers))
	for i, tc := range trackers {
		items[i] = tc
	}
	return items
}

// createTrackersTab creates the GUI for the Trackers tab using a simplified HBox layout.
func createTrackersTab() fyne.CanvasObject {
	trackerList := widget.NewListWithData(
		trackerListBinding,
		func() fyne.CanvasObject {
			nameTitleLabel := widget.NewLabelWithStyle("Name:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			addrTitleLabel := widget.NewLabelWithStyle("Address:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			pathTitleLabel := widget.NewLabelWithStyle("Path:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

			nameLabel := widget.NewLabel("")
			addrLabel := widget.NewLabel("")
			pathLabel := widget.NewLabel("")

			removeButton := widget.NewButton("Remove", nil)

			titlesContainer := container.NewVBox(nameTitleLabel, addrTitleLabel, pathTitleLabel)
			valuesContainer := container.NewVBox(nameLabel, addrLabel, pathLabel)

			return container.NewHBox(
				titlesContainer,
				valuesContainer,
				removeButton,
			)
		},
		func(item binding.DataItem, o fyne.CanvasObject) {
			value, err := item.(binding.Untyped).Get()
			if err != nil {
				log.WithError(err).Error("Failed to get tracker item from binding")
				return
			}

			tc, ok := value.(tracker.TrackerConfig)
			if !ok {
				err := errors.New("invalid tracker configuration")
				log.WithError(err).Error("Failed to cast tracker config")
				dialog.ShowError(err, myWindow)
				return
			}

			row, ok := o.(*fyne.Container)
			if !ok {
				err := errors.New("invalid list item container")
				log.WithError(err).Error("Failed to cast list item to *fyne.Container")
				dialog.ShowError(err, myWindow)
				return
			}

			if len(row.Objects) < 3 {
				err := errors.New("unexpected container structure")
				log.WithError(err).Error("List item container has insufficient objects")
				dialog.ShowError(err, myWindow)
				return
			}

			_, ok = row.Objects[0].(*fyne.Container)
			if !ok {
				err := errors.New("invalid titles container")
				log.WithError(err).Error("Failed to cast titles container")
				dialog.ShowError(err, myWindow)
				return
			}
			valuesContainer, ok := row.Objects[1].(*fyne.Container)
			if !ok {
				err := errors.New("invalid values container")
				log.WithError(err).Error("Failed to cast values container")
				dialog.ShowError(err, myWindow)
				return
			}
			removeButton, ok := row.Objects[2].(*widget.Button)
			if !ok {
				err := errors.New("invalid remove button")
				log.WithError(err).Error("Failed to cast remove button")
				dialog.ShowError(err, myWindow)
				return
			}

			if len(valuesContainer.Objects) < 3 {
				err := errors.New("insufficient labels in values container")
				log.WithError(err).Error("Values container has fewer than 3 labels")
				dialog.ShowError(err, myWindow)
				return
			}

			nameLabel, ok := valuesContainer.Objects[0].(*widget.Label)
			if !ok {
				err := errors.New("invalid name label")
				log.WithError(err).Error("Failed to cast name label")
				dialog.ShowError(err, myWindow)
				return
			}
			addrLabel, ok := valuesContainer.Objects[1].(*widget.Label)
			if !ok {
				err := errors.New("invalid address label")
				log.WithError(err).Error("Failed to cast address label")
				dialog.ShowError(err, myWindow)
				return
			}
			pathLabel, ok := valuesContainer.Objects[2].(*widget.Label)
			if !ok {
				err := errors.New("invalid path label")
				log.WithError(err).Error("Failed to cast path label")
				dialog.ShowError(err, myWindow)
				return
			}

			nameLabel.SetText(tc.Name)
			addrLabel.SetText(tc.TrackerAddr)
			pathLabel.SetText(tc.Path)

			removeButton.OnTapped = func() {
				confirm := dialog.NewConfirm("Remove Tracker", fmt.Sprintf("Are you sure you want to remove tracker '%s'?", tc.Name), func(confirmed bool) {
					if confirmed {
						err := tracker.GlobalTrackerManager.RemoveTracker(tc.Name)
						if err != nil {
							dialog.ShowError(err, myWindow)
							log.WithError(err).Errorf("Failed to remove tracker: %s", tc.Name)
							return
						}
						trackers := tracker.GlobalTrackerManager.GetTrackers()
						trackerListBinding.Set(trackersToAnySlice(trackers))
						log.Infof("Removed tracker: %s", tc.Name)
					}
				}, myWindow)
				confirm.Show()
			}
		},
	)

	addTrackerButton := widget.NewButton("Add Tracker", func() {
		showAddTrackerDialog(myWindow)
	})

	content := container.NewBorder(nil, addTrackerButton, nil, nil, trackerList)
	return container.NewScroll(content)
}

// showAddTrackerDialog presents a form to add a new tracker
func showAddTrackerDialog(parent fyne.Window) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Tracker Name")

	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder("Tracker Address (e.g., tracker.example.i2p)")

	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder("Announce Path (e.g., a, announce.php)")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Name", Widget: nameEntry},
			{Text: "Address", Widget: addrEntry},
			{Text: "Path", Widget: pathEntry},
		},
	}

	onSubmit := func() {
		name := strings.TrimSpace(nameEntry.Text)
		addr := strings.TrimSpace(addrEntry.Text)
		path := strings.TrimSpace(pathEntry.Text)

		if name == "" || addr == "" || path == "" {
			dialog.ShowError(errors.New("All fields are required"), parent)
			return
		}

		tc := tracker.TrackerConfig{
			Name:        name,
			TrackerAddr: addr,
			Path:        path,
		}

		tracker.GlobalTrackerManager.AddTracker(tc)

		trackers := tracker.GlobalTrackerManager.GetTrackers()
		trackerListBinding.Set(trackersToAnySlice(trackers))

		log.Infof("Added new tracker: %s", name)

		dialog.ShowInformation("Tracker Added", fmt.Sprintf("Tracker '%s' has been added successfully.", name), parent)
	}

	onCancel := func() {
		log.Info("Add Tracker dialog canceled")
	}

	onConfirm := func(confirmed bool) {
		if confirmed {
			onSubmit()
		} else {
			onCancel()
		}
	}

	addTrackerDialog := dialog.NewForm(
		"Add New Tracker",
		"Add",
		"Cancel",
		form.Items,
		onConfirm,
		parent,
	)
	addTrackerDialog.Resize(fyne.NewSize(400, 300))
	addTrackerDialog.Show()
}

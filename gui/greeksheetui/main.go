package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func main() {
	uiApp := app.NewWithID("greeksheet")
	myWindow := uiApp.NewWindow("Greek Sheet UI")
	myWindow.Resize(fyne.NewSize(400, 300))
	tabs := container.NewAppTabs(
		defineGoogleSheetsTab(),
		defineExcelTab(myWindow),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	myWindow.SetContent(tabs)
	myWindow.ShowAndRun()
}

func defineGoogleSheetsTab() *container.TabItem {
	ti := container.NewTabItem("Google Sheets", container.NewVBox())
	sheetID := widget.NewEntry()
	folderID := widget.NewEntry()
	title := widget.NewEntry()
	ti.Content = &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Sheet ID", Widget: sheetID},
			{Text: "Folder ID", Widget: folderID},
			{Text: "Title", Widget: title},
		},
		OnSubmit: func() {
			// Handle form submission here
			print("Sheet ID:", sheetID.Text, "\n", "Folder ID:", folderID.Text, "\n", "Title:", title.Text)
		},
	}
	return ti
}

var selectedFile = widget.NewLabel("No file selected")
var selectedFolder = widget.NewLabel("No folder selected")
var fileURI fyne.URI

func defineExcelTab(w fyne.Window) *container.TabItem {
	ti := container.NewTabItem("Excel (.xlsx)", container.NewVBox())
	filePath := widget.NewButton("File Path", func() { showFilePicker(w) })
	folderPath := widget.NewButton("Folder Path", func() { showFolderPicker(w) })

	ti.Content = &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Existing File", Widget: filePath},
			{Text: "Selected File", Widget: selectedFile},
			{Text: "Folder Path", Widget: folderPath},
			{Text: "Selected Folder", Widget: selectedFolder},
		},
		OnSubmit: func() {
			// Handle form submission here
			print("File Path:", fileURI)
		},
	}

	return ti
}

func showFilePicker(w fyne.Window) {
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if uc == nil {
			return
		}
		selectedFile.SetText(uc.URI().Path())
		fileURI = uc.URI()
	}, w)
	// only allow .xlsx files to be selected
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".xlsx"}))
	fd.Show()
}

func showFolderPicker(w fyne.Window) {
	fd := dialog.NewFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if list == nil {
			return
		}
		selectedFolder.SetText(list.Path())
		fileURI = list
	}, w)
	fd.Show()
}

/*
TODO
* remove the globals in favour of a struct that is the function receiver for the tab definition functions
* reference input field with validation
* title input field with validation
* validation for all inputs, but that could be shared with cli
* output should be a link on the form to open the local file/sheet in browser
* make the UI look good
* write tests for the UI code
*/

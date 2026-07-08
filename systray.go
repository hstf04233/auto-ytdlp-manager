package main

import (
	"github.com/getlantern/systray"
	"runtime"
	"autoytdlpmanager/os_tings"
)

func StartSystray() {
	go systray.Run(st_OnReady, st_OnExit)
}


func st_OnReady() {
	// 1. Set the visual properties of the tray icon
	IconFileName := "p_icon.png"
	IconFileIcoName := "p_icon.ico"
	
	if APPLICATION_VERSION_TYPE == "debug" {
		IconFileName = "p_icon_debog.png"
		IconFileIcoName = "p_icon_debog.ico"
	}
	
	systray.SetTemplateIcon(ReadFileFromStatic(IconFileName), ReadFileFromStatic(IconFileIcoName))
	systray.SetTitle("Auto yt-dlp Manager")
	systray.SetTooltip("Auto yt-dlp Manager")
	
	if runtime.GOOS == "windows" {
		mConsoleToggle := systray.AddMenuItem("Toggle console", "Toggle console visibility")
		
		IsVisible := true
		
		go func() {
			for {
				select {
				case <-mConsoleToggle.ClickedCh:
					IsVisible = !IsVisible
					os_tings.ToggleConsoleVisibility(IsVisible)
				}
			}
		}()
		
		L_Printf("Hide this console window by going to the system tray icon -> 'Toggle console'\n")
	}
	
	
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit the whole app")
	
	// 3. Monitor menu item clicks in a separate goroutine
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func st_Quit() {
	systray.Quit()
}

func st_OnExit() {
	// Perform clean up here (e.g., closing files, database connections)
	// L_Printf("Cleaning up and exiting application.\n")
	G_ExitProgram()
}

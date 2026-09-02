//go:build windows

package systray

import (
	"autoytdlpmanager/os_tings"
	"fmt"
	"runtime"

	"github.com/gogpu/systray"
)

var G_Tray *systray.SystemTray

func StartSystray(SystrayIconContent []byte) (bool, error) {
	tray := systray.New()
	G_Tray = tray
	
	ConsoleIsVisible := true
	
	menu := systray.NewMenu()
	if runtime.GOOS == "windows" {
		menu.Add("Toggle console", func() {
			ConsoleIsVisible = !ConsoleIsVisible
			os_tings.ToggleConsoleVisibility(ConsoleIsVisible)
		})
		
		fmt.Printf("Hide this console window by going to the system tray icon -> 'Toggle console'\n")
	}
	menu.AddSeparator()
	menu.Add("Quit", func() {
	    tray.Remove()
	})
	
	tray.SetIcon(SystrayIconContent).
	    SetTooltip("Auto yt-dlp Manager").
	    SetMenu(menu)
	tray.OnDoubleClick(func() {
		if !ConsoleIsVisible {
			ConsoleIsVisible = true
			os_tings.ToggleConsoleVisibility(ConsoleIsVisible)
		}
	})
	tray.Show()
	
	if err := tray.Run(); err != nil {
		return true, err
	}
	
	return true, nil
}

func RemoveSystray() {
	if G_Tray != nil {
		G_Tray.Remove()
		G_Tray = nil
	}
}

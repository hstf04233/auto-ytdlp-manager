//go:build windows

package os_tings

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio" // Only used on Windows
	"golang.org/x/sys/windows"
)

const (
	SW_HIDE = 0
	//SW_SHOW = 5
	SW_RESTORE = 9
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	showWindow       = user32.NewProc("ShowWindow")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetActiveWindow     = user32.NewProc("SetActiveWindow")
)

// This will open a file in read only mode and will not lock the file which means it can be deleted by another process
func OpenFileWithoutLocking(FilePath string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(FilePath)
    if err != nil {
        return nil, err
    }
	
	handle, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|
			windows.FILE_SHARE_WRITE|
			windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	
	File := os.NewFile(uintptr(handle), FilePath)
	
	return File, nil
}

func GetPipeName(PipeName string) string {
	return fmt.Sprintf(`\\.\pipe\%s`, PipeName)
}

func CreatePipe(PipeName string) (io.WriteCloser, error) {
	listener, err := winio.ListenPipe(PipeName, nil)
	if err != nil {
		return nil, err
	}
	// Accept one connection (FFmpeg will connect as client)
	return listener.Accept()
}

func OpenPipe(PipeName string) (io.WriteCloser, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(PipeName, &timeout)
}

func ToggleConsoleVisibility(show bool) {
	// 1. Get the handle of the current console window
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		return // No console window associated with this process
	}
	
	// 2. Set the visibility state
	if show {
		showWindow.Call(hwnd, SW_RESTORE)
		_, _, _ = procSetForegroundWindow.Call(hwnd)
		
		_, _, _ = procSetActiveWindow.Call(hwnd)
	} else {
		showWindow.Call(hwnd, SW_HIDE)
	}
}
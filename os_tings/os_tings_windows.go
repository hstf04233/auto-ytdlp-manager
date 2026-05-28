//go:build windows

package os_tings

import (
	"os"
	"golang.org/x/sys/windows"
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

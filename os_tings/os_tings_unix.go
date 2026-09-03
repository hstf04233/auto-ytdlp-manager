//go:build !windows

package os_tings

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// This will open a file in read only mode and will not lock the file which means it can be deleted by another process
func OpenFileWithoutLocking(FilePath string) (*os.File, error) {
	// chatgpt tells me that on unix systems you can just do this and it won't lock the file... Hopefully it's correct...
	File, err := os.Open(FilePath)
	if err != nil {
		return nil, err
	}
	
	return File, nil
}

func GetPipeName(PipeName string) string {
	// It's a unix system
	TempDir := "/tmp"
	
	TMPDIR := os.Getenv("TMPDIR")  // Termux temp directory.
	if TMPDIR != "" {
		TempDir = TMPDIR
	}
	
	return filepath.Join(TempDir, PipeName)
}

func CreatePipe(PipeName string) (io.WriteCloser, error) {
	// Remove old pipe if exists
	os.Remove(PipeName)
	
	// Create FIFO on Unix
	cmd := exec.Command("mkfifo", PipeName)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create fifo %s: %v", PipeName, err)
	}
	return os.OpenFile(PipeName, os.O_WRONLY, 0666)
}

func OpenPipe(PipeName string) (io.WriteCloser, error) {
	return os.OpenFile(PipeName, os.O_WRONLY, 0666)
}

func ToggleConsoleVisibility(show bool) {
	// Do nothing on unix systems...
	return
}

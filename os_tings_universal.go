package main

import (
	"errors"
	"os"
)

func DoesFileExist(FilePath string) bool {
	_, err := os.Stat(FilePath)
	if err == nil {
		// File exists!
		return true
	}

	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	L_Printf("DoesFileExist error %v\n", err)
	return false
}
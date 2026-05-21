package main

// TODO:

import (
	"fmt"
	"os"
	"sync"
)

var (
	logFile *os.File
	logSync sync.Mutex
)

func L_Printf(format string, a ... any) {
	Msg := fmt.Sprintf(format, a ...)
	
	fmt.Print(Msg)
	if logFile == nil {
		return
	}
	go func() {
		logSync.Lock()
		logFile.Write([]byte(Msg))
		logSync.Unlock()
	}()
}

func InitLogPrint() error {
	var err error
	logFile, err = os.Create("log_current.log")
	if err != nil {
		fmt.Printf("Failed to create 'log_current.log' error: %v\n", err)
		return fmt.Errorf("Failed to create 'log_current.log' error: %v\n", err)
	}
	
	return nil
}
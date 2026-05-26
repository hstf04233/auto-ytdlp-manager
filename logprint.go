package main

// TODO:

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	logFile *os.File
	logSync sync.Mutex
)

func L_Printf(format string, a ... any) {
	TimeNow := time.Now().Local().Format("2006-01-02 15:04:05")
	Msg := fmt.Sprintf(format, a ...)
	
	fmt.Print(Msg)
	if logFile == nil {
		return
	}
	go func() {
		logSync.Lock()
		MsgWithTime := fmt.Sprintf("[%s]: %s", TimeNow, Msg)
		logFile.Write([]byte(MsgWithTime))
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
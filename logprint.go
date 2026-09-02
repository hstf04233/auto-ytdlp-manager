package main

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

const (
	OLDLOGS_DIRECTORY = "./logs"
)

func L_Printf(format string, a ... any) {
	TimeNow := time.Now().Local().Format("2006-01-02 15:04:05")
	Msg := fmt.Sprintf(format, a ...)
	
	fmt.Print(Msg)
	if logFile == nil {
		return
	}
	//go func() {
		logSync.Lock()
		MsgWithTime := fmt.Sprintf("[%s]: %s", TimeNow, Msg)
		logFile.Write([]byte(MsgWithTime))
		logSync.Unlock()
	//}()
}

func MoveCurrentLog() error {
	if DoesFileExist("log_current.log") {
		File, err := os.Open("log_current.log")
		if err != nil {
			File.Close()
			fmt.Printf("Could not open old log file, error: %v\n", err)
			return err
		}
		Info, err := File.Stat()
		if err != nil {
			File.Close()
			fmt.Printf("Could not get file info for old log file, error: %v\n", err)
			return err
		}
		
		ModTime := Info.ModTime()
		File.Close()
		
		err = os.MkdirAll(OLDLOGS_DIRECTORY, 0755)
		if err != nil {
			fmt.Printf("Could not create file directory \"%s\" , error: %v\n", OLDLOGS_DIRECTORY, err)
			return err
		}
		
		NewFilePath := fmt.Sprintf("%s/%s.log", OLDLOGS_DIRECTORY, ModTime.Format("2006-01-02 15-04-05"))
		
		err = os.Rename("log_current.log", NewFilePath)
		if err != nil {
			fmt.Printf("Could not move log file \"log_current.log\" to \"%s\", error: %v\n", NewFilePath, err)
			return err
		}
	}
	
	return nil
}

func InitLogPrint() error {
	MoveCurrentLog()
	
	var err error
	logFile, err = os.Create("log_current.log")
	if err != nil {
		fmt.Printf("Failed to create 'log_current.log' error: %v\n", err)
		return fmt.Errorf("Failed to create 'log_current.log' error: %v\n", err)
	}
	
	return nil
}
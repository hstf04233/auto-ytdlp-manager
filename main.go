package main

import (
	"os"
	"path/filepath"
	"time"

	//"yt-stream-manager/webstatic"

	//"yt-stream-manager/database"
	"fmt"
	"net/http"
	"os/exec"
)

var (
	APPLICATION_VERSION = "release"
	CURRENT_WORKING_DIRECTORY = ""
)
var (
	CMD_YT_DLP = C_CMD_YT_DLP
	CMD_YT_ARCHIVE = C_CMD_YT_ARCHIVE
	CMD_FFMPEG = C_CMD_FFMPEG
)

func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
func FindCommand(cmd string) string {
	if filepath.IsLocal(cmd) {
		LocalCmd := fmt.Sprintf("%s/%s", CURRENT_WORKING_DIRECTORY, cmd)
		if CommandExists(LocalCmd) {
			// Program exists in working directory
			return LocalCmd
		}
	}
	
	if CommandExists(cmd) {
		// Program exists in the path environment
		return cmd
	}
	
	return cmd
}

func StartServer(ServerPort int) {
	Mux := http.NewServeMux()
	
	Mux.HandleFunc("/static/", webstatic_ServeStaticContent)
	
	// This should serve index.html
	Mux.HandleFunc("/", webstatic_ServeStaticContent)
	
	// This should serve favicon.png
	Mux.HandleFunc("/favicon.ico", webstatic_ServeStaticContent)
	Mux.HandleFunc("/api/", ServeApi)
	Mux.HandleFunc("/video-file/", ServeVideoDownload)
	
	L_Printf("Starting server at http://localhost:%d\n", ServerPort)
	
	// TODO: I'm planning on adding an auth system.
	L_Printf("!!! DO NOT HOST THIS PROGRAM TO THE INTERNET! THIS PROGRAM IS IN TESTING PHASE AND IS UNSAFE OUTSIDE OF THE LOCAL NETWORK...\n")
	
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		L_Printf("Cannot start server because: %v\n", err)
		L_Printf("The server port might be currently occupied... Edit the server port in config.json if you need to change the server port!\n")
		panic(err)
	}
}

func main() {
	InitLogPrint()
	
	var err error
	CURRENT_WORKING_DIRECTORY, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	
	ConfigPath := CONFIG_PATH
	if APPLICATION_VERSION == "debug" {
		ConfigPath = CONFIG_PATH_DEBUG
	}
	
	err = OpenConfig(ConfigPath)
	if err != nil {
		panic(err)
	}
	
	CMD_YT_DLP     = FindCommand(C_CMD_YT_DLP)
	CMD_YT_ARCHIVE = FindCommand(C_CMD_YT_ARCHIVE)
	CMD_FFMPEG     = FindCommand(C_CMD_FFMPEG)
	
	Exit := false
	
	if !CommandExists(CMD_YT_DLP) {
		Exit = true
		L_Printf("You need '%s' to run this program! https://github.com/yt-dlp/yt-dlp \n", C_CMD_YT_DLP)
	}
	if !CommandExists(CMD_FFMPEG) {
		Exit = true
		L_Printf("You need '%s' (and possibly 'ffprobe') to run this program! Get both from https://www.ffmpeg.org/ \n", C_CMD_FFMPEG)
	}
	if !CommandExists(CMD_YT_ARCHIVE) {
		Exit = true
		L_Printf("You need '%s' to run this program! https://github.com/dreammu/ytarchive\n", C_CMD_YT_ARCHIVE)
	}
	
	if Exit {
		L_Printf("The program will exit now due to unavailable programs...\n")
		time.Sleep(time.Second * 5)
		return
	}
	
	err = OpenDB()
	if err != nil {
		panic(err)
	}
	
	//CleanUpTasksInDatabase()
	
	go InitDownloading()
	
	L_Printf("APPLICATION_VERSION: %s\n", APPLICATION_VERSION)
	
	// TODO: THIS IS TEMP! go yt_chat_Run("https://www.youtube.com/watch?v=G5oz2dQLi00", "./test-chat-output.json", nil)
	
	StartServer(int(G_Config.ServerPort))
}
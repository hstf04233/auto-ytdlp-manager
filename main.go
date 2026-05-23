package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	//"yt-stream-manager/webstatic"

	//"yt-stream-manager/database"
	"fmt"
	"net/http"
	"os/exec"
)

var (
	APPLICATION_VERSION_NUMBER = "v0.13"
	APPLICATION_VERSION_TYPE = "alpha"
	APPLICATION_VERSION = APPLICATION_VERSION_NUMBER + " " + APPLICATION_VERSION_TYPE
	
	CURRENT_WORKING_DIRECTORY = ""
)

func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
func FindCommand(cmd string) string {
	if runtime.GOOS == "windows" {
		if filepath.Ext(cmd) == "" {
			cmd += ".exe"
		}
	}
	
	if filepath.IsLocal(cmd) {
		LocalCmd := fmt.Sprintf("%s/%s", CURRENT_WORKING_DIRECTORY, cmd)
		if CommandExists(LocalCmd) {
			// Program exists in working directory
			return LocalCmd
		}
	}
	
	if CommandExists(cmd) {
		// Program exists in the system path environment
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
	L_Printf("!!! THIS PROGRAM IS IN TESTING PHASE AND IS UNSAFE OUTSIDE OF THE LOCAL NETWORK!!! PLEASE DO NOT HOST THIS ON A WEBSITE !!!\n")
	
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		L_Printf("Cannot start server because: %v\n", err)
		L_Printf("The server port might be currently occupied... Edit the server port in config.json if you need to change the server port!\n")
		panic(err)
	}
}

func main() {
	fmt.Printf("------- auto yt-dlp manager -------\n")
	InitLogPrint()
	L_Printf("APPLICATION_VERSION: %s\n\n", APPLICATION_VERSION)
	
	var err error
	CURRENT_WORKING_DIRECTORY, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	
	ConfigPath := CONFIG_PATH
	if APPLICATION_VERSION_TYPE == "debug" {
		ConfigPath = CONFIG_PATH_DEBUG
	}
	
	err = OpenConfig(ConfigPath)
	if err != nil {
		panic(err)
	}
	
	G_Config.YtDlp_Path_Real     = FindCommand(G_Config.YtDlp_Path)
	G_Config.YtArchive_Path_Real = FindCommand(G_Config.YtArchive_Path)
	G_Config.FFmpeg_Path_Real    = FindCommand(G_Config.FFmpeg_Path)
	G_Config.Deno_Path_Real      = FindCommand(G_Config.Deno_Path)
	
	Exit := false
	
	if !CommandExists(G_Config.YtDlp_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://github.com/yt-dlp/yt-dlp \n\n", G_Config.YtDlp_Path)
	}
	if !CommandExists(G_Config.YtArchive_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://github.com/dreammu/ytarchive\n\n", G_Config.YtArchive_Path)
	}
	if !CommandExists(G_Config.FFmpeg_Path_Real) {
		Exit = true
		L_Printf("Could not find program '%s' Get the dependency here: https://www.ffmpeg.org/ \n\n", G_Config.FFmpeg_Path)
	}
	
	if !CommandExists(G_Config.Deno_Path_Real) {
		L_Printf("Could not find optional dependency 'deno' (Used by yt-dlp) It's recommended that you get this for yt-dlp to fully work properly.\n")
		L_Printf("If you experience long wait times for yt-dlp or missing formats, then getting deno *might* fix the issue...: https://github.com/denoland/deno/releases\n\n")
	}
	
	if Exit {
		L_Printf("The program will exit now due to unavailable dependencies...\n")
		L_Printf("If you already have these dependencies, edit the paths in ./config.json to the correct location\n")
		time.Sleep(time.Second * 10)
		return
	}
	
	err = OpenDB()
	if err != nil {
		L_Printf("Database failed to open because: %v\n", err)
		panic(err)
	}
	
	//CleanUpTasksInDatabase()
	
	go InitDownloading()
	
	// TODO: THIS IS TEMP! go yt_chat_Run("https://www.youtube.com/watch?v=G5oz2dQLi00", "./test-chat-output.json", nil)
	
	StartServer(int(G_Config.ServerPort))
}
package main

import (
	"os"
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
const (
	APPLICATION_NAME = "Auto yt-dlp Manager"
)
var (
	CMD_YT_DLP = C_CMD_YT_DLP
	CMD_YT_ARCHIVE = C_CMD_YT_ARCHIVE
	CMD_FFMPEG = C_CMD_FFMPEG
)

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
func findCommand(cmd string) string {
	LocalCmd := fmt.Sprintf("%s/%s", CURRENT_WORKING_DIRECTORY, cmd)
	if commandExists(LocalCmd) {
		// Program exists in working directory
		return LocalCmd
	}
	
	if commandExists(cmd) {
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
	
	fmt.Printf("Starting server at http://localhost:%d\n", ServerPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		fmt.Printf("Cannot start server because: %v\n", err)
		panic(err)
	}
}

func main() {
	var err error
	CURRENT_WORKING_DIRECTORY, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	
	CMD_YT_DLP = findCommand(C_CMD_YT_DLP)
	CMD_YT_ARCHIVE = findCommand(C_CMD_YT_ARCHIVE)
	CMD_FFMPEG = findCommand(C_CMD_FFMPEG)
	
	Exit := false
	
	if !commandExists(CMD_YT_DLP) {
		Exit = true
		fmt.Printf("You need '%s' to run this program! https://github.com/yt-dlp/yt-dlp \n", C_CMD_YT_DLP)
	}
	if !commandExists(CMD_FFMPEG) {
		Exit = true
		fmt.Printf("You need '%s' (and possibly 'ffprobe') to run this program! Get both from https://www.ffmpeg.org/ \n", C_CMD_FFMPEG)
	}
	if !commandExists(CMD_YT_ARCHIVE) {
		AlternativeYT_ARCHIVE_Cmds := []string{"ytarchive", "ytarchive_arm64"}
		for _, AltCmd := range(AlternativeYT_ARCHIVE_Cmds) {
			CMD_YT_ARCHIVE = findCommand(AltCmd)
			if commandExists(CMD_YT_ARCHIVE) {
				break
			}
		}
		if !commandExists(CMD_YT_ARCHIVE) {
			Exit = true
			fmt.Printf("You need '%s' to run this program! https://github.com/dreammu/ytarchive\n", C_CMD_YT_ARCHIVE)
		}
	}
	
	if Exit {
		fmt.Printf("The program will exit now due to unavailable programs... Read the readme.txt for more information.\n")
		time.Sleep(time.Second * 5)
		return
	}
	
	err = OpenDB()
	if err != nil {
		panic(err)
	}
	
	CleanUpListingTasksInDatabase()
	
	go StartDownloading()
	
	fmt.Printf("APPLICATION_VERSION: %s\n", APPLICATION_VERSION)
	ServerPort := SERVER_PORT
	if APPLICATION_VERSION == "debug" {
		ServerPort = SERVER_PORT_DEBUG
	}
	
	//go yt_chat_Run("https://www.youtube.com/watch?v=G5oz2dQLi00", "./test-chat-output.json", nil)
	
	StartServer(ServerPort)
}
package main

import (
	"yt-stream-manager/webstatic"
	//"yt-stream-manager/database"
	"fmt"
	"net/http"
)

var (
	APPLICATION_VERSION = "release"
)
const (
	SERVER_PORT = 8867
	SERVER_PORT_DEBUG = 6788
	
	APPLICATION_NAME = "YT Video Manager"
	CMD_YT_DLP = "yt-dlp"
	CMD_YT_ARCHIVE = "ytarchive"
	DEFAULT_DOWNLOAD_DIR = "./downloads"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE = "%(title)s %(id)s.%(ext)s"
)

func StartServer(ServerPort int) {
	Mux := http.NewServeMux()
	
	Mux.HandleFunc("/static/", webstatic.ServeStaticContent)
	
	// This should serve index.html
	Mux.HandleFunc("/", webstatic.ServeStaticContent)
	
	// This should serve favicon.png
	Mux.HandleFunc("/favicon.ico", webstatic.ServeStaticContent)
	Mux.HandleFunc("/api/", ServeApi)
	
	fmt.Printf("Starting server at http://localhost:%d\n", ServerPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		fmt.Printf("Cannot start server because: %v\n", err)
		panic(err)
	}
}

func main() {
	err := OpenDB()
	if err != nil {
		panic(err)
	}
	
	go StartDownloading()
	
	fmt.Printf("APPLICATION_VERSION: %s\n", APPLICATION_VERSION)
	ServerPort := SERVER_PORT
	if APPLICATION_VERSION == "debug" {
		ServerPort = SERVER_PORT_DEBUG
	}
	
	go yt_chat_Run("https://www.youtube.com/watch?v=G5oz2dQLi00", "./test-chat-output.json", nil)
	
	StartServer(ServerPort)
}
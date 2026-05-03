package main

import (
	"yt-stream-manager/webstatic"
	//"yt-stream-manager/database"
	"fmt"
	"net/http"
)

const (
	SERVER_PORT = 8867
	CMD_YT_DLP = "yt-dlp"
	CMD_YT_ARCHIVE = "ytarchive"
	DEFAULT_DOWNLOAD_DIR = "./downloads"
)

func StartServer(ServerPort int) {
	Mux := http.NewServeMux()
	
	Mux.HandleFunc("/static/", webstatic.ServeStaticContent)
	
	// This should serve index.html
	Mux.HandleFunc("/", webstatic.ServeStaticContent)
	
	// This should serve favicon.png
	Mux.HandleFunc("/favicon.ico", webstatic.ServeStaticContent)
	
	fmt.Printf("Starting server at http://localhost:%d\n", ServerPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		fmt.Printf("Cannot start server because: %v\n", err)
		panic(err)
	}
}

func main() {
	OpenDB()
	
	go StartDownloading()
	
	ListVideos("https://www.youtube.com/@iAbuodee/streams", 10)
	StartServer(SERVER_PORT)
}
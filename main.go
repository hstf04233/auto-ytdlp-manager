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
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE = "%(title)s %(id)s.%(ext)s"
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
	err := OpenDB()
	if err != nil {
		panic(err)
	}
	
	NewChannel := &ArchiveChannel{
		Name: "TEST!",
		Url: "https://www.youtube.com/@Freddiebeans101/streams",
		CheckInterval: 60,
		QualitySelect: 480,
		Type: ACHANNEL_TYPE_LIVE,
		Enabled: true,
	}
	err = AddArchiveChannel(&WatchedDownloading, NewChannel)
	if err != nil {
		panic(err)
	}
	
	go StartDownloading()
	
	StartServer(SERVER_PORT)
}
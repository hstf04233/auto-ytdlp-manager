package main

import (
	"yt-stream-manager/webstatic"
	"fmt"
	"net/http"
)

const (
	SERVER_PORT = 8867
)

func StartServer(ServerPort int) {
	Mux := http.NewServeMux()
	
	Mux.HandleFunc("/static/", webstatic.ServeStaticContent)
	Mux.HandleFunc("/", webstatic.ServeStaticContent)		// This should serve index.html
	Mux.HandleFunc("/favicon.ico", webstatic.ServeStaticContent)	// This should serve favicon.png
	
	fmt.Printf("Starting server at http://localhost:%d\n", ServerPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", ServerPort), Mux); err != nil {
		fmt.Printf("Cannot start server because: %v\n", err)
		panic(err)
	}
}

func main() {
	StartServer(SERVER_PORT)
}
package main

import (
	"fmt"
	"net/http"
)

const (
	SERVER_PORT = 8867
)

func StartServer() {
	Mux := http.NewServeMux()
	
	fmt.Printf("Starting server at http://localhost:%d\n", SERVER_PORT)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", SERVER_PORT), Mux); err != nil {
		fmt.Printf("Cannot start server because: %v\n", err)
		panic(err)
	}
}

func main() {
	
	
	go StartServer()
}
package main

import (
	"net/http"
	"embed"
	"strings"
	"fmt"
	"path/filepath"
	"io"
	"time"
)

//go:embed static/*
var WebStaticContent embed.FS

// TODO: ! this could be set at build time but I'm lazy rn
var ETag string
var ProgramStartTime = time.Now().UTC()

func webstatic_ServeStaticContent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "favicon.ico" {
		path = "favicon.png"
		if APPLICATION_VERSION_TYPE == "debug" {
			path = "favicon_debog.png"
		}
	} else if path == "" || path == "/" {
		path = "index.html"
	} else {
		if strings.HasPrefix(path, "static/") {
			path = strings.TrimPrefix(path, "static/")
		} else {
			path = "index.html"
		}
	}
	
	File, err := WebStaticContent.Open(fmt.Sprintf("static/%s", path))
	if err != nil {
		L_Printf("Cannot find file: %s\n", path)
		http.Error(w, "Page not found.", http.StatusNotFound)
		return
	}
	defer File.Close()
	
	FileSeeker, ok := File.(io.ReadSeeker)
	if !ok {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	http.ServeContent(w, r, filepath.Base(path), ProgramStartTime, FileSeeker)
	//w.Write(FileContent)
}

func init() {
	ETag = time.Now().String()
}

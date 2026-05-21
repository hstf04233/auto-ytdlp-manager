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
var StartTime = time.Now().UTC()

func webstatic_ServeStaticContent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "favicon.ico" {
		path = "favicon.png"
		if APPLICATION_VERSION == "debug" {
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
	
	if r.Header.Get("If-None-Match") == ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	
	FileExt := filepath.Ext(path)
	if FileExt == ".txt" {
		w.Header().Set("Content-Type", "text/plain")
	} else if FileExt == ".html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if FileExt == ".js" {
		w.Header().Set("Content-Type", "text/javascript")
	} else if FileExt == ".css" {
		w.Header().Set("Content-Type", "text/css")
	
	// Images
	} else if FileExt == ".png" {
		w.Header().Set("Content-Type", "image/png")
	} else if FileExt == ".jpeg" || FileExt == ".jpg" {
		w.Header().Set("Content-Type", "image/jpeg")
	} else if FileExt == ".gif" {
		w.Header().Set("Content-Type", "image/gif")
	} else if FileExt == ".webp" {
		w.Header().Set("Content-Type", "image/webp")
	} else if FileExt == ".svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	
	FileSeeker, ok := File.(io.ReadSeeker)
	if !ok {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	http.ServeContent(w, r, filepath.Base(path), StartTime, FileSeeker)
	//w.Write(FileContent)
}

func init() {
	ETag = time.Now().String()
}

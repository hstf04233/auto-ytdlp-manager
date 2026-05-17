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

func webstatic_ServeStaticContent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	//fmt.Printf("Serving \"%s\"\n", path)
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
		fmt.Printf("Cannot find file: %s\n", path)
		http.Error(w, "Page not found.", http.StatusNotFound)
		return
	}
	defer File.Close()
	
	//w.Header().Set("Cache-Control", "public, max-age=86400")	// Cache for 24 hours.
	//w.Header().Set("ETag", ETag)
	if r.Header.Get("If-None-Match") == ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	
	FileContent, err := io.ReadAll(File)
	if err != nil {
		fmt.Printf("Could not read file? \"%s\", err: %v\n", path, err)
		http.Error(w, "File could not be read?", http.StatusInternalServerError)
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
	
	w.Write(FileContent)
}

func init() {
	ETag = time.Now().String()
}

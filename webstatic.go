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

var ProgramStartTime = time.Now().UTC()

func webstatic_ServeStaticContent(w http.ResponseWriter, r *http.Request) {
	if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_GLOBAL) { return }
	
	NeedsLogin := false
	
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "logout" && r.Method == "POST" {
		AuthLogoutRequest(w, r)
		return
	}
	if path == "favicon.ico" {
		path = "favicon.png"
		if APPLICATION_VERSION_TYPE == "debug" {
			path = "favicon_debog.png"
		}
	} else if path == "" || path == "/" {
		path = "index.html"
		NeedsLogin = true
		
	} else if path == "logout" {
		AuthLogoutRequest(w, r)
		return
	} else if strings.HasPrefix(path, "login") {
		path = "login.html"
	} else {
		if strings.HasPrefix(path, "static/") {
			path = strings.TrimPrefix(path, "static/")
		} else {
			NeedsLogin = true
			path = "index.html"
		}
	}
	
	IsAuthorized, err := IsRequestAuthorized(r)
	if err != nil {
		L_Printf("Auth error: %v\n", err)
		http.Error(w, fmt.Sprintf("Auth error: %v", err), http.StatusInternalServerError)
		return
	}
	if !IsAuthorized && NeedsLogin {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
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

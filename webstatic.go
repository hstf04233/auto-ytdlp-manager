package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

//go:embed static/*
var WebStaticContent embed.FS

type WebStaticAssetInfo struct {
	OriginalPath string
	HashedPath   string
	Hash256      string
}
var WebStatic_HashPathAssetInfo = map[string]*WebStaticAssetInfo{}
var WebStatic_OGPathAssetInfo   = map[string]*WebStaticAssetInfo{}

var WebStatic_INDEX_HTML []byte
var WebStatic_LOGIN_HTML []byte

var __FilesToManifest = []string{
	"js/app.js",
	"js/login_app.js",
	"js/chat.js",
	
	"css/style.css",
}

var ProgramStartTime = time.Now().UTC()

func webstatic_ServeIndexHtml(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(WebStatic_INDEX_HTML)
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}
func webstatic_ServeLoginHtml(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(WebStatic_LOGIN_HTML)
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

func webstatic_ServeStaticContent(w http.ResponseWriter, r *http.Request) {
	if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_GLOBAL) { return }
	
	NeedsLogin := false
	
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "logout" && r.Method == "POST" {
		AuthLogoutRequest(w, r)
		return
	}
	if path == "favicon.ico" {
		path = "images/favicon.png"
		if APPLICATION_VERSION_TYPE == "debug" {
			path = "images/favicon_debog.png"
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
	
	IsAuthorized, err := IsRequestReadOnlyAuthorized(r)
	if err != nil {
		L_Printf("Auth error: %v\n", err)
		http.Error(w, fmt.Sprintf("Auth error: %v", err), http.StatusInternalServerError)
		return
	}
	if !IsAuthorized && NeedsLogin {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	if path == "index_gotemplate.html" || path == "login_gotemplate.html" {
		// Bruh
		http.Error(w, "Page not found.", http.StatusNotFound)
		return
	}
	
	// OgPath := path
	
	ManifestAssetInfo, ok := WebStatic_HashPathAssetInfo[path]
	if ok {
		// Replace files like 'js/index-********.js' to just 'js/index.js'
		path = ManifestAssetInfo.OriginalPath
	}
	
	switch path {
		case "index.html": {
			webstatic_ServeIndexHtml(w, r)
			return
		}
		case "login.html": {
			webstatic_ServeLoginHtml(w, r)
			return
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

func _webstatic_AddToManifest(FilePath string) {
	RealFilePath := fmt.Sprintf("static/%s", FilePath)
	FileContent, err := WebStaticContent.ReadFile(RealFilePath)
	if err != nil {
		if errors.Is(os.ErrNotExist, err) {
			L_Printf("Manifest build ERROR!: Cannot find file '%s'. Make sure this program was built correctly with all the assets!\n", RealFilePath)
		} else {
			L_Printf("Manifest build ERROR for file '%s'...\n", RealFilePath)
		}
		return
	}
	
	Sum := sha3.Sum256(FileContent)
	
	HashStr := base64.RawURLEncoding.EncodeToString(Sum[0:32])
	
	ext := filepath.Ext(FilePath)
	HashedName := fmt.Sprintf("%s-%s%s", strings.TrimSuffix(FilePath, ext), HashStr[0:8], ext)
	
	AssetInfo := &WebStaticAssetInfo{
		OriginalPath: FilePath,
		HashedPath:   HashedName,
		
		Hash256:      HashStr,
	}
	
	WebStatic_HashPathAssetInfo[HashedName] = AssetInfo
	WebStatic_OGPathAssetInfo[FilePath] = AssetInfo
}

func webstatic_GetCacheBustedAsset(Path string) string {
	ManifestAssetInfo, ok := WebStatic_OGPathAssetInfo[Path]
	if ok {
		// Replace files like 'js/index-********.js' to just 'js/index.js'
		Path = ManifestAssetInfo.HashedPath
	}
	
	return Path
}

func _webstatic_GenerateExecutedHtmlTemplate(HtmlFilePath string, Data any, Out *[]byte) error {
	HtmlTemplate, err := template.ParseFS(WebStaticContent, HtmlFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("ERROR: Could not read '%s' !!!!!! PLEASE MAKE SURE YOU BUILT THE PROGRAM CORRECTLY!!!!\n", HtmlFilePath)
		L_Printf("%s", errMsg)
		return err
	}
	
	var ExecutedHtmlBuffer bytes.Buffer
	
	err = HtmlTemplate.Execute(&ExecutedHtmlBuffer, Data)
	if err != nil {
		errMsg := fmt.Sprintf("ERROR: Could not parse template file '%s' !!!!!! \n", HtmlFilePath)
		L_Printf("%s", errMsg)
		return err
	}
	
	*Out = ExecutedHtmlBuffer.Bytes()
	
	return nil
}

func init() {
	// Generate manifest
	for _, FilePath := range(__FilesToManifest) {
		_webstatic_AddToManifest(FilePath)
	}
	
	IndexHtmlData := struct{
		AppJsPath       string
		ChatJsPath      string
		LoginAppJsPath  string
		
		StyleCssPath string
	}{
		AppJsPath:      webstatic_GetCacheBustedAsset("js/app.js"),
		ChatJsPath:     webstatic_GetCacheBustedAsset("js/chat.js"),
		LoginAppJsPath: webstatic_GetCacheBustedAsset("js/login_app.js"),
		
		StyleCssPath: webstatic_GetCacheBustedAsset("css/style.css"),
	}
	
	err := _webstatic_GenerateExecutedHtmlTemplate("static/index_gotemplate.html", IndexHtmlData, &WebStatic_INDEX_HTML)
	if err != nil {
		panic(err)
		return
	}
	err = _webstatic_GenerateExecutedHtmlTemplate("static/login_gotemplate.html", IndexHtmlData, &WebStatic_LOGIN_HTML)
	if err != nil {
		panic(err)
		return
	}
}

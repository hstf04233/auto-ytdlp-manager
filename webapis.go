package main

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"strconv"
	"encoding/json"
)

/*
	Name string `json:"name"`
	Url  string `json:"url"`
	
	DownloadDir    string `json:"download_dir"`
	OutputTemplate string `json:"output_template"`
	QualitySelect  int    `json:"quality_select"`
	Type           int32  `json:"type"`
	CheckInterval  int64  `json:"check_interval"`
	FullCheckInterval int64 `json:"full_check_interval"`
	
	Enabled        bool `json:"enabled"`
	IsBeingChecked bool
	
	NextCheckMSEC            int64 `json:"_nextCheckMsec"`
	NextFullChannelCheckMSEC int64 `json:"_nextFullChannelCheckMsec"`
*/

const API_MAX_URL_LENGTH = 1 << 14

type API_RequestChannelBody struct{
	Name string `json:"name"`
	Url  string `json:"url"`
	
	DownloadDir    string `json:"download_dir"`
	OutputTemplate string `json:"output_template"`
	QualitySelect  int    `json:"quality_select"`
	Type           int32  `json:"type"`
	CheckInterval  int64  `json:"check_interval"`
	FullCheckInterval int64 `json:"full_check_interval"`
	
	Enabled *bool `json:"enabled"`
}

func Verify_API_RequestChannelBody(body API_RequestChannelBody) (bool, string) {
	if len(body.Name) > 128 {
		return false, "Name must be shorter than 128 characters."
	}
	if len(body.Url) > API_MAX_URL_LENGTH {
		return false, fmt.Sprintf("Url must be shorter than %d characters.", API_MAX_URL_LENGTH)
	}
	if len(body.DownloadDir) > 1024 {
		return false, "DownloadDir must be shorter than 1024 characters."
	}
	if len(body.OutputTemplate) > 1024 {
		return false, "OutputTemplate must be shorter than 1024 characters."
	}
	
	return true, ""
}

func API_NewChannel(w http.ResponseWriter, r *http.Request) {
	var body API_RequestChannelBody
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if IsVerified, Reason := Verify_API_RequestChannelBody(body); !IsVerified {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		body.Name = "New Channel"
	}
	if body.QualitySelect < 0 {
		body.QualitySelect = 0
	}
	
	NewChannel := &ArchiveChannel{
		Name: body.Name,
		Url: body.Url,
		
		DownloadDir: body.DownloadDir,
		OutputTemplate: body.OutputTemplate,
		QualitySelect: body.QualitySelect,
		Type: body.Type,
		
		CheckInterval: body.CheckInterval,
		FullCheckInterval: body.FullCheckInterval,
	}
	if body.Enabled != nil {
		NewChannel.Enabled = *body.Enabled
	}
	err := AddArchiveChannel(&WatchedDownloading, NewChannel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(NewChannel)
}
func Set_API_RequestChannelBodyDefaults(body *API_RequestChannelBody) {
	body.QualitySelect = -1
	body.Type = -1
	body.CheckInterval = -1
	body.FullCheckInterval = -1
	body.Enabled = nil
}
func API_UpdateChannel(w http.ResponseWriter, r *http.Request) {
	Id := path.Base(r.URL.Path)
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, Id)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	
	var body API_RequestChannelBody
	Set_API_RequestChannelBodyDefaults(&body)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if IsVerified, Reason := Verify_API_RequestChannelBody(body); !IsVerified {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}
	
	if body.Name != "" {
		AChannel.Name = body.Name
	}
	if body.Url != "" {
		AChannel.Url = body.Url
	}
	if body.DownloadDir != "" {
		AChannel.DownloadDir = body.DownloadDir
	}
	if body.OutputTemplate != "" {
		AChannel.OutputTemplate = body.OutputTemplate
	}
	if body.QualitySelect >= 0 {
		AChannel.QualitySelect = body.QualitySelect
	}
	if body.Type != -1 {
		AChannel.Type = body.Type
	}
	if body.CheckInterval != -1 {
		AChannel.CheckInterval = body.CheckInterval
	}
	if body.FullCheckInterval != -1 {
		AChannel.FullCheckInterval = body.FullCheckInterval
	}
	if body.Enabled != nil {
		AChannel.Enabled = *body.Enabled
	}
	err := DB_UpdateArchiveChannel(AChannel)
	if err != nil {
		// Just log this error and move on...
		fmt.Printf("Error when updating archive channel in database: %v\n", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AChannel)
}
func API_DeleteChannel(w http.ResponseWriter, r *http.Request) {
	Id := path.Base(r.URL.Path)
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, Id)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	err := RemoveArchiveChannel(&WatchedDownloading, Id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func API_GetChannels(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "channels/") && RequestId != "" {
		// Request single channel.
		AChannel := GetArchiveChannelFromId(&WatchedDownloading, RequestId)
		if AChannel == nil {
			http.Error(w, "Channel not found.", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AChannel)
		return
	}
	
	WatchedDownloading.ChannelsLock.RLock()
	
	Channels := WatchedDownloading.Channels
	if len(Channels) <= 0 {
		// This fixes the json encode to be {} instead of null
		Channels = []*ArchiveChannel{}
	}
	
	defer WatchedDownloading.ChannelsLock.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"Count": len(WatchedDownloading.Channels),
		"Channels": WatchedDownloading.Channels,
	})
}

func API_GetVideos(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "videos/") && RequestId != "" {
		// Request single video.
		VideoInfo, err := DB_GetVideo(RequestId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if VideoInfo == nil {
			http.Error(w, "Video not found.", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VideoInfo)
		return
	}
	
	Limit := 50
	Offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		Limit, _ = strconv.Atoi(l)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		Offset, _ = strconv.Atoi(o)
	}
	
	
	VideosList, err := DB_ListVideos(fmt.Sprintf("ORDER BY AddedAt DESC LIMIT %d OFFSET %d", Limit, Offset))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"Count": len(VideosList),
		"Videos": VideosList,
	})
}

func API_GetVideoStatus(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId != "" {
		// Request single video.
		VideoInfo, err := DB_GetVideo(RequestId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if VideoInfo == nil {
			http.Error(w, "Video not found.", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Status": VideoInfo.Status,
		})
		return
	}
	
	http.Error(w, "Video ID required.", http.StatusBadRequest)
	return
}

func ServeApi(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/")
	Path := r.URL.Path
	Method := r.Method
	
	if Path == "channels" && Method == "POST" {
		API_NewChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "PUT" {
		API_UpdateChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "DELETE" {
		API_DeleteChannel(w, r)
	} else if (Path == "channels" || strings.HasPrefix(Path, "channels/")) && Method == "GET" {
		API_GetChannels(w, r)
	} else if (Path == "videos" || strings.HasPrefix(Path, "videos/")) && Method == "GET" {
		API_GetVideos(w, r)
	} else if (strings.HasPrefix(Path, "get-video-status/")) && Method == "GET" {
		API_GetVideoStatus(w, r)
	} else {
		http.NotFound(w, r)
	}
}
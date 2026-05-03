package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

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
	// This is intended behavior. I want all newly created channels to be paused by default ! (Even if the request wants it to be enabled...)
	NewChannel.Enabled = false
	
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
		if AChannel.Enabled {
			AChannel.NextCheckMSEC = time.Now().UnixMilli() + (1000 * 4)
		}
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
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
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
		// This fixes an issue where an empty list might be null instead of {}
		Channels = []*ArchiveChannel{}
	}
	
	defer WatchedDownloading.ChannelsLock.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(WatchedDownloading.Channels),
		"channels": Channels,
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
	
	FromChannel := ""
	Status := -1
	Limit := 50
	Offset := 0
	if fc := r.URL.Query().Get("from_channel"); fc != "" {
		FromChannel = fc
		AChannel := GetArchiveChannelFromId(&WatchedDownloading, FromChannel)
		if AChannel == nil {
			http.Error(w, "Channel not found.", http.StatusNotFound)
			return
		}
	}
	if s := r.URL.Query().Get("status"); s != "" {
		Status, _ = strconv.Atoi(s)
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		Limit, _ = strconv.Atoi(l)
		if Limit == -1 {
			Limit = -1
		} else if Limit < 0 {
			Limit = 50
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		Offset, _ = strconv.Atoi(o)
	}
	
	VideosList, err := DB_ListVideos(Limit, Offset, Status, FromChannel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(VideosList),
		"videos": VideosList,
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
			"status": VideoInfo.Status,
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
		// POSTing to api/channels will create a new channel.
		API_NewChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "PUT" {
		// PUTing to api/channels/{channel_id} will update a channel.
		API_UpdateChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "DELETE" {
		// DELETE-ing to api/channels/{channel_id} will delete a channel.
		API_DeleteChannel(w, r)
	} else if (Path == "channels" || strings.HasPrefix(Path, "channels/")) && Method == "GET" {
		// api/channels will give the entire list of channels !
		// api/channels/{channel_id} will give you a specific channel.
		API_GetChannels(w, r)
	} else if (Path == "videos" || strings.HasPrefix(Path, "videos/")) && Method == "GET" {
		/*
		  api/video?limit={int}&offset={int}&status={int}&from_channel={channel_id} Will return a list of videos,
		  status and from_channel are optional.
		  
		  api/video/{video_id} will give you a specific video.
		*/
		API_GetVideos(w, r)
	} else if (strings.HasPrefix(Path, "get-video-status/")) && Method == "GET" {
		// api/get-video-status/{video_id} will return just the status of the video and nothing else.
		// Returns as json (example: {status: 0})
		
		// This is to grab the video status without refreshing the entire list with the api/videos list api.
		API_GetVideoStatus(w, r)
	} else {
		http.NotFound(w, r)
	}
}
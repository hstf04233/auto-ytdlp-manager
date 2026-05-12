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
const API_MAX_REQUEST_ID = 1 << 8

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
	var Body API_RequestChannelBody
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if IsVerified, Reason := Verify_API_RequestChannelBody(Body); !IsVerified {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}
	if Body.Name == "" {
		Body.Name = "New Channel"
	}
	if Body.QualitySelect < 0 {
		Body.QualitySelect = 0
	}
	
	NewChannel := &ArchiveChannel{
		Name: Body.Name,
		Url: Body.Url,
		
		DownloadDir: Body.DownloadDir,
		OutputTemplate: Body.OutputTemplate,
		QualitySelect: Body.QualitySelect,
		Type: Body.Type,
		
		CheckInterval: Body.CheckInterval,
		FullCheckInterval: Body.FullCheckInterval,
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
func Set_API_RequestChannelBodyDefaults(Body *API_RequestChannelBody) {
	Body.QualitySelect = -1
	Body.Type = -1
	Body.CheckInterval = -1
	Body.FullCheckInterval = -1
	Body.Enabled = nil
}
func API_UpdateChannel(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid channel id.", http.StatusBadRequest)
		return
	}
	
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, RequestId)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	
	var Body API_RequestChannelBody
	Set_API_RequestChannelBodyDefaults(&Body)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if IsVerified, Reason := Verify_API_RequestChannelBody(Body); !IsVerified {
		http.Error(w, Reason, http.StatusBadRequest)
		return
	}
	
	if Body.Name != "" {
		AChannel.Name = Body.Name
	}
	if Body.Url != "" {
		AChannel.Url = Body.Url
	}
	if Body.DownloadDir != "" {
		AChannel.DownloadDir = Body.DownloadDir
	}
	if Body.OutputTemplate != "" {
		AChannel.OutputTemplate = Body.OutputTemplate
	}
	if Body.QualitySelect >= 0 {
		AChannel.QualitySelect = Body.QualitySelect
	}
	if Body.Type != -1 {
		AChannel.Type = Body.Type
	}
	if Body.CheckInterval != -1 {
		AChannel.CheckInterval = Body.CheckInterval
		if AChannel.NextCheckMSEC > time.Now().UnixMilli()+AChannel.CheckInterval {
			AChannel.NextCheckMSEC = time.Now().UnixMilli()+AChannel.CheckInterval
		}
	}
	if Body.FullCheckInterval != -1 {
		AChannel.FullCheckInterval = Body.FullCheckInterval
		if AChannel.NextFullChannelCheckMSEC > time.Now().UnixMilli()+AChannel.FullCheckInterval {
			AChannel.NextFullChannelCheckMSEC = time.Now().UnixMilli()+AChannel.FullCheckInterval
		}
	}
	if Body.Enabled != nil {
		AChannel.Enabled = *Body.Enabled
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
func API_CheckChannel(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid channel id.", http.StatusBadRequest)
		return
	}
	
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, RequestId)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	
	AChannel.NextCheckMSEC = time.Now().UnixMilli()-1
}
func API_DeleteChannel(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid channel id.", http.StatusBadRequest)
		return
	}
	
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, RequestId)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	err := RemoveArchiveChannel(&WatchedDownloading, RequestId)
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
		if len(RequestId) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid channel id.", http.StatusBadRequest)
			return
		}
		
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

type TAPI_VideoListStats struct{
	Total int `json:"total"`
	
	TotalQueued      int `json:"total_queued"`
	TotalDownloading int `json:"total_downloading"`
	TotalDownloaded  int `json:"total_downloaded"`
	TotalFailed      int `json:"total_failed"`
	TotalIgnored     int `json:"total_ignored"`
}

var VideosListCache = NewCache(time.Second * 4)

func API_GetVideos(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "videos/") && RequestId != "" {
		if len(RequestId) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid video id.", http.StatusBadRequest)
			return
		}
		
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
	
	OrderBy := 0
	OrderDirection := -1
	FromChannelId := ""
	SearchQuery := ""
	Status := -1
	Limit := 50
	Offset := 0
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
	if fc := r.URL.Query().Get("from_channel"); fc != "" {
		FromChannelId = fc
	}
	if sq := r.URL.Query().Get("search_query"); sq != "" {
		SearchQuery = sq
		if len(SearchQuery) > 1024 {
			http.Error(w, "search_query must be shorter than 1024 characters", http.StatusBadRequest)
			return
		}
	}
	if s := r.URL.Query().Get("status"); s != "" {
		Status, _ = strconv.Atoi(s)
	}
	if o := r.URL.Query().Get("order_direction"); o != "" {
		OrderDirection, _ = strconv.Atoi(o)
	}
	if o := r.URL.Query().Get("order_by"); o != "" {
		if o == "added_at" {
			OrderBy = DB_VIDEO_ORDERBY_AddedAt
		} else if o == "release_date" {
			OrderBy = DB_VIDEO_ORDERBY_ReleaseDate
		} else if o == "updated_at" {
			OrderBy = DB_VIDEO_ORDERBY_UpdatedAt
		}
	}
	
	VideosList, err := DB_ListVideos(Limit, Offset, ListVideosQuery{
		RefreshState: -1,
		Status: Status,
		FromChannelId: FromChannelId,
		SearchQuery:   SearchQuery,
		
		OrderBy: OrderBy,
		OrderDirection: OrderDirection,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	CacheKey := fmt.Sprintf("%s %s %d", FromChannelId, SearchQuery, Status)
	
	VideosListCache.CleanUp()
	
	Stats := &TAPI_VideoListStats{}
	StatsC, CacheExists := VideosListCache.Get(CacheKey)
	if CacheExists {
		Stats = StatsC.(*TAPI_VideoListStats)
	} else {
		VideosListStats := VideosList
		if (len(VideosList) >= Limit || Offset > 0) {
			// TODO: DON'T DO THIS!!! Use a specially crafted database query instead of getting every video...
			// (Or maybe cache the results aswell...)
			VideosListAll, err := DB_ListVideos(-1, 0, ListVideosQuery{
				RefreshState: -1,
				Status: Status,
				FromChannelId: FromChannelId,
				SearchQuery:   SearchQuery,
				
				OrderBy: OrderBy,
				OrderDirection: OrderDirection,
			})
			if err == nil {
				VideosListStats = VideosListAll
			} else if err != nil {
				fmt.Printf("Failed to get videos list for stats... Err: %v\n", VideosListAll)
			}
		}
		
		Stats = &TAPI_VideoListStats{
			Total: len(VideosListStats),
		}
		for _, Video := range(VideosListStats) {
			switch Video.Status {
				case VIDEO_STATUS_QUEUED:      Stats.TotalQueued++
				case VIDEO_STATUS_DOWNLOADING: Stats.TotalDownloading++
				case VIDEO_STATUS_DOWNLOADED:  Stats.TotalDownloaded++
				case VIDEO_STATUS_FAILED:      Stats.TotalFailed++
				case VIDEO_STATUS_IGNORED:     Stats.TotalIgnored++
			}
		}
		
		VideosListCache.Set(CacheKey, Stats)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(VideosList),
		"videos": VideosList,
		"stats":  Stats,
	})
}

func API_GetVideoStatus(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId == "" {
		http.Error(w, "Video id required.", http.StatusBadRequest)
		return
	}
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	
	VideoInfo, err := DB_GetVideo(RequestId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when getting video: %v !", err), http.StatusInternalServerError)
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

func API_UpdateVideo(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId == "" {
		http.Error(w, "Video id required.", http.StatusBadRequest)
		return
	}
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	
	VideoInfo, err := DB_GetVideo(RequestId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when getting video: %v !", err), http.StatusInternalServerError)
		return
	}
	if VideoInfo == nil {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}
	
	var Body struct{
		Status *int `json:"status"`
		RefreshState *bool `json:"refresh_state"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if Body.Status != nil && *Body.Status >= 0 && *Body.Status <= 10 {
		DB_UpdateVideoStatus(VideoInfo, *Body.Status)
	}
	
	if Body.RefreshState != nil {
		if *Body.RefreshState {
			DB_UpdateVideoRefreshState(VideoInfo, 1)
		} else {
			DB_UpdateVideoRefreshState(VideoInfo, 0)
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VideoInfo)
}

func API_DeleteVideo(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	
	Video, err := DB_GetVideo(RequestId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when checking if video exists: %v !", err), http.StatusInternalServerError)
		return
	}
	if Video == nil {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}
	err = DB_DeleteVideo(Video)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when deleting video: %v !", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}

func API_GetTasks(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "tasks/") && RequestId != "" {
		if len(RequestId) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid task id.", http.StatusBadRequest)
			return
		}
		Task, err := CL_GetCommandTask(RequestId)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error when getting task: %v !", err), http.StatusInternalServerError)
			return
		}
		if Task == nil {
			http.Error(w, "Task not found.", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Task)
		return
	}
	
	Limit := 20
	Offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		Limit, _ = strconv.Atoi(l)
		if Limit == -1 {
			Limit = -1
		} else if Limit < 0 {
			Limit = 20
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		Offset, _ = strconv.Atoi(o)
	}
	
	TasksList, err := CL_ListCommandTasks(Limit, Offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when trying to list videos, err: %v", err), http.StatusInternalServerError)
		return
	}
	
	TasksListStats := TasksList
	if (len(TasksList) >= Limit || Offset > 0) {
		// TODO: DON'T DO THIS!!! Use a specially crafted database query instead of getting every task...
		// (Or maybe cache the results aswell...)
		TasksListAll, err := DB_ListCommandTasks(-1, 0)
		if err == nil {
			TasksListStats = TasksListAll
		} else if err != nil {
			fmt.Printf("Failed to get tasks list for stats... Err: %v\n", TasksListAll)
		}
	}
	
	Stats := struct{
		Total int `json:"total"`
		
		TotalRunning  int `json:"total_running"`
		TotalFailed   int `json:"total_failed"`
		TotalFinished int `json:"total_finished"`
		TotalCanceled int `json:"total_canceled"`
	}{
		Total: len(TasksListStats),
	}
	for _, Task := range(TasksListStats) {
		switch Task.Status {
			case TASK_STATUS_RUNNING:  Stats.TotalRunning++
			case TASK_STATUS_FAILED:   Stats.TotalFailed++
			case TASK_STATUS_FINISHED: Stats.TotalFinished++
			case TASK_STATUS_CANCELED: Stats.TotalCanceled++
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(TasksList),
		"tasks": TasksList,
		"stats": Stats,
	})
}

func API_GetTaskOutput(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if RequestId == "" {
		http.Error(w, "Task ID required.", http.StatusBadRequest)
		return
	}
	
	Task, err := CL_GetCommandTask(RequestId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when getting command task: %v !", err), http.StatusInternalServerError)
	}
	if Task == nil {
		http.Error(w, "Task not found!", http.StatusNotFound)
		return
	}
	
	Task.Lock.RLock()
	w.Header().Set("Content-Type", "text/plain")
	if Task.Status == TASK_STATUS_RUNNING {
		w.Write([]byte(TruncateOutput(Task.RealtimeOutput)))
	} else {
		w.Write([]byte(TruncateOutput(Task.Output)))
	}
	Task.Lock.RUnlock()
	return
}

func ServeApi(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/")
	Path := r.URL.Path
	Method := r.Method
	
	if Path == "channels" && Method == "POST" {
		// POSTing to api/channels will create a new channel.
		API_NewChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "PATCH" {
		// PATCHing to api/channels/{channel_id} will update a channel.
		API_UpdateChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "DELETE" {
		// DELETE-ing to api/channels/{channel_id} will delete a channel.
		API_DeleteChannel(w, r)
	} else if (Path == "channels" || strings.HasPrefix(Path, "channels/")) && Method == "GET" {
		// api/channels will give the entire list of channels !
		// api/channels/{channel_id} will give you a specific channel.
		API_GetChannels(w, r)
	} else if strings.HasPrefix(Path, "check-channel-now/") && Method == "PATCH" {
		// PATCHing to api/check-channel-now/{channel_id} make the channel be checked first chance it gets.
		API_CheckChannel(w, r)
	} else if (Path == "videos" || strings.HasPrefix(Path, "videos/")) && Method == "GET" {
		/*
		  api/videos?limit={int}&offset={int}&status={int}&from_channel={channel_id}&order_by={order}&order_direction={1, -1} Will return a list of videos.
		  status, from_channel, order_by and order_direction are optional.
		  
		  api/videos/{video_id} will give you a specific video.
		*/
		API_GetVideos(w, r)
	} else if (strings.HasPrefix(Path, "videos/")) && Method == "PATCH" {
		// You can set status, refresh_state
		API_UpdateVideo(w, r)
	} else if (strings.HasPrefix(Path, "videos/")) && Method == "DELETE" {
		// DELETE api/videos/{video_id} will delete a video
		API_DeleteVideo(w, r)
	} else if (strings.HasPrefix(Path, "get-video-status/")) && Method == "GET" {
		// api/get-video-status/{video_id} will return just the status of the video and nothing else.
		// Returns as json (example: {status: 0})
		
		// This is to grab the video status without refreshing the entire list with the api/videos list api.
		API_GetVideoStatus(w, r)
	} else if (Path == "tasks" || strings.HasPrefix(Path, "tasks/")) && Method == "GET" {
		/*
		  api/tasks?limit={int}&offset={int} Returns a list of videos. (There are no filter at the moment... but there will be swoon)
		  
		  api/tasks/{video_id} will give you a specific task.
		*/
		API_GetTasks(w, r)
	} else if strings.HasPrefix(Path, "get-realtime-task-output/") && Method == "GET" {
		API_GetTaskOutput(w, r)
	} else {
		http.NotFound(w, r)
	}
}
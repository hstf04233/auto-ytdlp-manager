package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
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
	PlaylistEnd       int   `json:"playlist_end"`
	
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
	Body.FullCheckInterval = -1
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
	if Body.FullCheckInterval == -1 {
		Body.FullCheckInterval = 86400
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
		PlaylistEnd: Body.PlaylistEnd,
	}
	if NewChannel.CheckInterval < 0 {
		NewChannel.CheckInterval = 0
	}
	if NewChannel.FullCheckInterval < 0 {
		NewChannel.FullCheckInterval = 0
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
	Body.PlaylistEnd = -2
	Body.Enabled = nil
}
func API_UpdateChannel(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid channel id.", http.StatusBadRequest)
		return
	}
	
	if RequestId == MANUAL_CHANNEL_ID {
		http.Error(w, "Cannot edit manual channel...", http.StatusBadRequest)
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
	
	AChannel.Lock.Lock()
	
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
		if AChannel.CheckInterval < 0 {
			AChannel.CheckInterval = 0
		}
		CheckTime := time.Now().UTC().UnixMilli()+(AChannel.CheckInterval*1000)
		if AChannel.NextCheckMSEC > CheckTime {
			AChannel.NextCheckMSEC = CheckTime
		}
	}
	if Body.FullCheckInterval != -1 {
		AChannel.FullCheckInterval = Body.FullCheckInterval
		if AChannel.FullCheckInterval < 0 {
			AChannel.FullCheckInterval = 0
		}
		CheckTime := time.Now().UTC().UnixMilli()+(AChannel.FullCheckInterval*1000)
		if AChannel.NextFullChannelCheckMSEC > CheckTime {
			AChannel.NextFullChannelCheckMSEC = CheckTime
		}
	}
	if Body.PlaylistEnd != -2 {		// -1 is reserved for 'All Videos'.
		AChannel.PlaylistEnd = Body.PlaylistEnd
	}
	if Body.Enabled != nil {
		LastEnabled := AChannel.Enabled
		AChannel.Enabled = *Body.Enabled
		if !LastEnabled && AChannel.Enabled {
			AChannel.NextCheckMSEC = time.Now().UTC().UnixMilli() + (1000 * 4)
		}
	}
	AChannel.Lock.Unlock()
	
	err := DB_UpdateArchiveChannel(AChannel)
	if err != nil {
		// Just log this error and move on...
		L_Printf("Error when updating archive channel in database: %v\n", err)
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
	
	var Body struct {
		InstantCheck bool `json:"instant_check"`
		
		CheckAll    bool `json:"check_all_videos"`
		ChannelType int `json:"override_channel_type"`
	}
	Body.ChannelType = -1
	
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if Body.ChannelType == -2 {
		// Download
		if AChannel.Type == ACHANNEL_TYPE_LIVE {
			Body.ChannelType = ACHANNEL_TYPE_LIVE
		} else {
			Body.ChannelType = ACHANNEL_TYPE_VIDEOS
		}
	}
	
	if Body.InstantCheck {
		go CheckChannel(AChannel, ChannelCheckSettings{
			QualitySelect: -1,
			ForceEnable: true,
			
			CheckAllVideos: Body.CheckAll,
			OverrideChannelType: Body.ChannelType,
		})
	}
	
	AChannel.NextCheckMSEC = time.Now().UTC().UnixMilli()-1
	AChannel.NextFullChannelCheckMSEC = time.Now().UTC().UnixMilli()-1
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}
func API_DeleteChannel(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid channel id.", http.StatusBadRequest)
		return
	}
	if RequestId == MANUAL_CHANNEL_ID {
		http.Error(w, "Cannot delete manual channel...", http.StatusBadRequest)
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

func API_SpiceUpChannelInfo(w http.ResponseWriter, r *http.Request, AChannel *ArchiveChannel) {
	TasksList, err := CL_ListCommandTasks(-1, 0, ListCommandTasksQuery{
		FromChannelId: AChannel.Id,
		Status: -1,
		Type: -1,
		OrderDirection: -1,
	})
	if err == nil && TasksList != nil {
		AChannel.FORAPI_TasksCount = len(TasksList)
		for _, Task := range(TasksList) {
			if Task.Status == TASK_STATUS_RUNNING {
				AChannel.FORAPI_ActiveTaskId = Task.Id
				break
			}
		}
	}
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
		
		SendChannel := &ArchiveChannel{}
		*SendChannel = *AChannel
		
		API_SpiceUpChannelInfo(w, r, SendChannel)
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SendChannel)
		return
	}
	
	WatchedDownloading.ChannelsLock.RLock()
	
	SendChannels := []*ArchiveChannel{}
	
	Channels := WatchedDownloading.Channels
	for _, AChannel := range(Channels) {
		NewChannel := &ArchiveChannel{}
		*NewChannel = *AChannel
		
		API_SpiceUpChannelInfo(w, r, NewChannel)
		
		SendChannels = append(SendChannels, NewChannel)
	}
	WatchedDownloading.ChannelsLock.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(SendChannels),
		"channels": SendChannels,
	})
}
func API_SpiceUpVideoInfo(w http.ResponseWriter, r *http.Request, Video *VideoInfo) {
	if Video.DownloadedFilename != "" {
		Filepath, err := GetDownloadedVideoFilePath(Video, nil)
		if err == nil {
			Video.VideoFileExists = false
		}
		if Filepath == "" {
			// Video file was not found... Deleted?
			Video.VideoFileExists = false
			
			Video.FileSize = 0   // This is for the web ui, this doesn't save into the database.
		} else {
			Video.VideoFileExists = true
		}
	}
	
	if !Video.VideoFileExists && Video.StreamedDirectory != "" && Video.Status == VIDEO_STATUS_DOWNLOADING {
		// Share m3u8 stream instead.
		Video.VideoStreamUrl = fmt.Sprintf("/video-stream/%s/playlist.m3u8", Video.Id)
	}
	
	TasksList, err := CL_ListCommandTasks(-1, 0, ListCommandTasksQuery{
		FromVideoId: Video.Id,
		Status: -1,
		Type: -1,
	})
	if err == nil && TasksList != nil {
		Video.TasksCount = len(TasksList)
		for _, Task := range(TasksList) {
			if Task.Status == TASK_STATUS_RUNNING {
				Video.ActiveTaskId = Task.Id
				break
			}
		}
	}
}

func API_AddVideos(w http.ResponseWriter, r *http.Request) {
	var Body struct {
		DownloadUrl string `json:"download_url"`
		TargetChannel string `json:"target_channel_id"`
		
		QualitySelect int `json:"quality_select"`
		Type int `json:"type"`
	}
	Body.QualitySelect = -1
	
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(Body.DownloadUrl) > API_MAX_URL_LENGTH {
		http.Error(w, fmt.Sprintf("Download url must be shorter than %d characters.", API_MAX_URL_LENGTH), http.StatusBadRequest)
		return
	}
	
	var AChannel *ArchiveChannel
	if Body.TargetChannel == "" || Body.TargetChannel == MANUAL_CHANNEL_ID {
		AChannel = GetManualArchiveChannel(&WatchedDownloading)
	} else {
		if len(Body.TargetChannel) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid target channel id.", http.StatusBadRequest)
			return
		}
		AChannel = GetArchiveChannelFromId(&WatchedDownloading, Body.TargetChannel)
	}
	if AChannel == nil {
		http.Error(w, "Could not find target channel.", http.StatusNotFound)
		return
	}
	
	if Body.Type == -2 {
		// Download
		if AChannel.Type == ACHANNEL_TYPE_LIVE {
			Body.Type = ACHANNEL_TYPE_LIVE
		} else {
			Body.Type = ACHANNEL_TYPE_VIDEOS
		}
	}
	
	NoWait := false
	if nw := r.URL.Query().Get("no_wait"); nw != "" {
		NoWait = true
	}
	if queue_video_id := r.URL.Query().Get("queue_video_id"); queue_video_id != "" {
		if len(queue_video_id) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid video id (queue_video_id).", http.StatusBadRequest)
			return
		}
		VideoInfo, err := DB_GetVideo(queue_video_id)
		if err == nil && VideoInfo != nil { 
			DB_UpdateVideoStatus(VideoInfo, VIDEO_STATUS_QUEUED)
		}
	}
	
	CS := &CheckStatus{}
	
	go ManuallyAddVideos(AChannel, Body.DownloadUrl, Body.Type, Body.QualitySelect, CS)
	
	Status := 0
	QuitTime := time.Now().UTC().Add(time.Duration(time.Minute * 1))
	
	if !NoWait {
		for {
			time.Sleep(time.Duration(time.Millisecond * 50))
			CS.Mutex.RLock()
			Status = CS.Status
			CS.Mutex.RUnlock()
			
			if time.Now().UTC().UnixMilli() > QuitTime.UnixMilli() {
				Status = CHECK_STATUS_FAILED
			}
			
			if Status >= CHECK_STATUS_ADDED_VIDEOS || Status == CHECK_STATUS_FAILED {
				break
			}
		}
		
		if Status == CHECK_STATUS_FAILED {
			http.Error(w, "An error occured when trying to check videos...", http.StatusInternalServerError)
			return
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}

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
		API_SpiceUpVideoInfo(w, r, VideoInfo)
		
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
	
	if Offset < 0 {
		Offset = 0
	}
	if Limit == -1 || Limit > 100 {
		Limit = 100
	}
	
	Query := ListVideosQuery{
		RefreshState: -1,
		QueuedAction: -1,
		VideoType:    -1,
		Status: Status,
		FromChannelId: FromChannelId,
		SearchQuery:   SearchQuery,
		
		OrderBy: OrderBy,
		OrderDirection: OrderDirection,
	}
	
	VideosList, err := DB_ListVideos(Limit, Offset, Query)
	if err != nil {
		http.Error(w, fmt.Sprintf("DB_ListVideos ERROR! | %s", err.Error()), http.StatusInternalServerError)
		return
	}
	
	Stats, err := DB_GetVideoStatsFromQuery(Query)
	
	for _, Video := range(VideosList) {
		API_SpiceUpVideoInfo(w, r, Video)
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
		if VideoInfo.Status == VIDEO_STATUS_IGNORED {
			DB_UpdateVideoQueuedAction(VideoInfo, VIDEO_QACTION_NONE)
		}
		if VideoInfo.Status == VIDEO_STATUS_IGNORED || VideoInfo.Status == VIDEO_STATUS_QUEUED {
			// Cancel all download tasks for this video
			CL_CancelTasksForVideo(VideoInfo.Id, TASK_TYPE_DOWNLOAD)
		}
	}
	
	if Body.RefreshState != nil {
		if *Body.RefreshState {
			DB_UpdateVideoRefreshState(VideoInfo, 1)
		} else {
			DB_UpdateVideoRefreshState(VideoInfo, 0)
		}
		
		AChannel := GetArchiveChannelFromId(&WatchedDownloading, VideoInfo.FromChannel)
		if AChannel != nil {
			AChannel.NeedsRefreshing = true
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
	
	VideoInfo, err := DB_GetVideo(RequestId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when checking if video exists: %v !", err), http.StatusInternalServerError)
		return
	}
	if VideoInfo == nil {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}
	err = DB_DeleteVideo(VideoInfo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when deleting video: %v !", err), http.StatusInternalServerError)
		return
	}
	
	CL_CancelTasksForVideo(VideoInfo.Id, TASK_TYPE_DOWNLOAD)
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}

func API_CancelTask(w http.ResponseWriter, r *http.Request) {
	RequestId := path.Base(r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "cancel-task/") && RequestId != "" {
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
		
		err = CL_CancelTask(Task)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error when canceling task: %v !", err), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"Success\":true}"))
		return
	}
	
	http.Error(w, "Task id required.", http.StatusBadRequest)
	return
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
	
	Query := ListCommandTasksQuery{
		Status: -2,
		Type: -1,
	}
	
	if fc := r.URL.Query().Get("from_channel"); fc != "" {
		Query.FromChannelId = fc
	}
	if fv := r.URL.Query().Get("from_video"); fv != "" {
		Query.FromVideoId = fv
	}
	if s := r.URL.Query().Get("status"); s != "" {
		Status, _ := strconv.Atoi(s)
		Query.Status = Status
	}
	if t := r.URL.Query().Get("type"); t != "" {
		Type, _ := strconv.Atoi(t)
		Query.Type = Type
	}
	if o := r.URL.Query().Get("order_direction"); o != "" {
		OrderDirection, _ := strconv.Atoi(o)
		Query.OrderDirection = OrderDirection
	}
	if o := r.URL.Query().Get("order_by"); o != "" {
		var OrderBy int
		if o == "start_time" {
			OrderBy = DB_CTASK_ORDERBY_StartTime
		} else if o == "end_time" {
			OrderBy = DB_CTASK_ORDERBY_EndTime
		}
		Query.OrderBy = OrderBy
	}
	
	if Offset < 0 {
		Offset = 0
	}
	if Limit == -1 || Limit > 50 {
		Limit = 50
	}
	
	TasksList, err := CL_ListCommandTasks(Limit, Offset, Query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when trying to list videos, err: %v", err), http.StatusInternalServerError)
		return
	}
	
	TasksListStats := TasksList
	if (len(TasksList) >= Limit || Offset > 0) {
		// TODO: DON'T DO THIS!!! Use a specially crafted database query instead of getting every task...
		// (Or maybe cache the results aswell...)
		TasksListAll, err := DB_ListCommandTasks(-1, 0, Query)
		if err == nil {
			TasksListStats = TasksListAll
		} else if err != nil {
			L_Printf("Failed to get tasks list for stats... Err: %v\n", TasksListAll)
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
	if Task.Status == TASK_STATUS_RUNNING && Task.RealtimeOutput != "" {
		w.Write([]byte(TruncateOutput(Task.RealtimeOutput)))
	} else {
		w.Write([]byte(TruncateOutput(Task.Output)))
	}
	Task.Lock.RUnlock()
	return
}

func API_GetConfig(w http.ResponseWriter, r *http.Request) {
	var RequestConfig ProgramConfig
	RequestConfig = *G_Config
	RequestConfig.ServerPort = 0
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RequestConfig)
}
func API_SetConfig(w http.ResponseWriter, r *http.Request) {
	
	var Body struct {
		YtDlp_Path     string `json:"YtDlp_Path"`
		YtArchive_Path string `json:"YtArchive_Path"`
		FFmpeg_Path    string `json:"FFmpeg_Path"`
		
		AllChannels_Disabled *bool `json:"AllChannels_Disabled"`
		
		Default_DownloadDir string `json:"Default_DownloadDir"`
		Default_YtDlp_OutputTemplate      string `json:"Default_YtDlp_OutputTemplate"`
		Default_YtDlp_OutputTemplate_Live string `json:"Default_YtDlp_OutputTemplate_Live"`
		
		TaskLog_AutoDelete_Enabled *bool `json:"TaskLog_AutoDelete_Enabled"`
		TaskLog_AutoDelete_Seconds      int `json:"TaskLog_AutoDelete_Seconds"`
		TaskLog_List_AutoDelete_Seconds int `json:"TaskLog_List_AutoDelete_Seconds"`
		
		AutoRefresh_Videos_Seconds int `json:"AutoRefresh_Videos_Seconds"`
		
		Download_Video_Thumbnails *bool `json:"Download_Video_Thumbnails"`
	}
	Body.TaskLog_AutoDelete_Seconds = -1
	Body.TaskLog_List_AutoDelete_Seconds = -1
	Body.AutoRefresh_Videos_Seconds = -1
	
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(Body.YtDlp_Path) > 1024 {
		http.Error(w, "YtDlp_Path must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	if len(Body.YtArchive_Path) > 1024 {
		http.Error(w, "YtArchive_Path must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	if len(Body.FFmpeg_Path) > 1024 {
		http.Error(w, "FFmpeg_Path must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	
	if len(Body.Default_DownloadDir) > 1024 {
		http.Error(w, "Default_DownloadDir must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	if len(Body.Default_YtDlp_OutputTemplate) > 1024 {
		http.Error(w, "Default_YtDlp_OutputTemplate must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	if len(Body.Default_YtDlp_OutputTemplate_Live) > 1024 {
		http.Error(w, "Default_YtDlp_OutputTemplate_Live must be shorter than 1024 characters.", http.StatusBadRequest)
		return
	}
	
	if Body.YtDlp_Path != "" {
		if !CommandExists(FindCommand(Body.YtDlp_Path)) {
			http.Error(w, fmt.Sprintf("Could not find yt-dlp path '%s'.", Body.YtDlp_Path), http.StatusBadRequest)
			return
		}
		G_Config.YtDlp_Path = Body.YtDlp_Path
	}
	if Body.YtArchive_Path != "" {
		if !CommandExists(FindCommand(Body.YtArchive_Path)) {
			http.Error(w, fmt.Sprintf("Could not find ytarchive path '%s'.", Body.YtArchive_Path), http.StatusBadRequest)
			return
		}
		G_Config.YtArchive_Path = Body.YtArchive_Path
	}
	if Body.FFmpeg_Path != "" {
		if !CommandExists(FindCommand(Body.FFmpeg_Path)) {
			http.Error(w, fmt.Sprintf("Could not find ffmpeg path '%s'.", Body.FFmpeg_Path), http.StatusBadRequest)
			return
		}
		G_Config.FFmpeg_Path = Body.FFmpeg_Path
	}
	
	// Actually set the config now...
	
	if Body.AllChannels_Disabled != nil {
		G_Config.AllChannels_Disabled = *Body.AllChannels_Disabled
	}
	if Body.TaskLog_AutoDelete_Enabled != nil {
		G_Config.TaskLog_AutoDelete_Enabled = *Body.TaskLog_AutoDelete_Enabled
	}
	if Body.Download_Video_Thumbnails != nil {
		G_Config.Download_Video_Thumbnails = *Body.Download_Video_Thumbnails
	}
	
	if Body.YtDlp_Path != "" {
		G_Config.YtDlp_Path = Body.YtDlp_Path
	}
	if Body.YtArchive_Path != "" {
		G_Config.YtArchive_Path = Body.YtArchive_Path
	}
	if Body.FFmpeg_Path != "" {
		G_Config.FFmpeg_Path = Body.FFmpeg_Path
	}
	
	if Body.Default_DownloadDir != "" {
		G_Config.Default_DownloadDir = Body.Default_DownloadDir
	}
	if Body.Default_YtDlp_OutputTemplate != "" {
		G_Config.Default_YtDlp_OutputTemplate = Body.Default_YtDlp_OutputTemplate
	}
	if Body.Default_YtDlp_OutputTemplate_Live != "" {
		G_Config.Default_YtDlp_OutputTemplate_Live = Body.Default_YtDlp_OutputTemplate_Live
	}
	
	if Body.TaskLog_AutoDelete_Seconds != -1 {
		G_Config.TaskLog_AutoDelete_Seconds = Body.TaskLog_AutoDelete_Seconds
	}
	if Body.TaskLog_List_AutoDelete_Seconds != -1 {
		G_Config.TaskLog_List_AutoDelete_Seconds = Body.TaskLog_List_AutoDelete_Seconds
	}
	if Body.AutoRefresh_Videos_Seconds != -1 {
		G_Config.AutoRefresh_Videos_Seconds = Body.AutoRefresh_Videos_Seconds
	}
	
	err := UpdateConfig(G_Config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update config: %v\n", err), http.StatusInternalServerError)
		return
	}
	
	API_GetConfig(w, r)
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
	} else if strings.HasPrefix(Path, "check-channel-now/") && Method == "POST" {
		// PPOSTing to api/check-channel-now/{channel_id} make the channel be checked first chance it gets.
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
	} else if (strings.HasPrefix(Path, "add-videos")) && Method == "POST" {
		API_AddVideos(w, r)
	} else if (strings.HasPrefix(Path, "get-video-status/")) && Method == "GET" {
		// api/get-video-status/{video_id} will return just the status of the video and nothing else.
		// Returns as json (example: {status: 0})
		
		// This is to grab the video status without refreshing the entire list with the api/videos list api.
		API_GetVideoStatus(w, r)
	} else if (Path == "tasks" || strings.HasPrefix(Path, "tasks/")) && Method == "GET" {
		/*
		  api/tasks?limit={int}&offset={int}&status={int}&type={int}&from_channel={channel_id}&from_video={video_id}&order_by={order}&order_direction={1, -1}
		  
		  api/tasks/{video_id} will give you a specific task.
		*/
		API_GetTasks(w, r)
	} else if (strings.HasPrefix(Path, "cancel-task/")) && Method == "POST" {
		API_CancelTask(w, r)
	} else if strings.HasPrefix(Path, "get-realtime-task-output/") && Method == "GET" {
		API_GetTaskOutput(w, r)
	} else if strings.HasPrefix(Path, "config") && Method == "GET" {
		API_GetConfig(w, r)
	} else if strings.HasPrefix(Path, "config") && Method == "PATCH" {
		API_SetConfig(w, r)
	} else {
		fmt.Printf("Api path: '%s' Method: '%s' not found\n", Path, Method)
		http.NotFound(w, r)
	}
}

func ServeVideoDownload(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Internal error when getting video details.", http.StatusInternalServerError)
		return
	}
	if VideoInfo == nil {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}
	if VideoInfo.DownloadedFilename == "" {
		http.Error(w, "Video contains no file attached.", http.StatusNotFound)
		return
	}
	AChannel := GetArchiveChannelFromId(&WatchedDownloading, VideoInfo.FromChannel)
	if AChannel == nil {
		http.Error(w, "Video info exists but no channel is attached to it?", http.StatusNotFound)
		return
	}
	FilePath, err := GetDownloadedVideoFilePath(VideoInfo, AChannel)
	if err != nil {
		http.Error(w, "Error getting full video file path...", http.StatusInternalServerError)
		return
	}
	if FilePath == "" {
		http.Error(w, fmt.Sprintf("Could not find video file '%s' The video must've been moved or deleted.", VideoInfo.DownloadedFilename), http.StatusNotFound)
		return
	}
	Filename := filepath.Base(FilePath)
	
	if DownloadVal := r.URL.Query().Get("download"); DownloadVal != "" {
		// User wants to download this video
		DownloadVal = strings.ToLower(DownloadVal)
		if DownloadVal == "true" || DownloadVal == "1" || DownloadVal == "yes" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", Filename))
		} else {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
		}
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
	}
	
	http.ServeFile(w, r, FilePath)
}

func ServeVideoStream(w http.ResponseWriter, r *http.Request) {
	Path := r.URL.Path
	Path = strings.TrimPrefix(Path, "/video-stream/")
	PathArgs := strings.Split(Path, "/")
	if len(PathArgs) < 2 {
		http.Error(w, "Invalid request path.", http.StatusBadRequest)
		return
	}
	
	RequestId := PathArgs[0]
	if RequestId == "" {
		http.Error(w, "Video id required.", http.StatusBadRequest)
		return
	}
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	
	RequestFile := PathArgs[1]
	if RequestFile == "" {
		http.Error(w, "File name required.", http.StatusBadRequest)
		return
	}
	if len(RequestFile) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid file name.", http.StatusBadRequest)
		return
	}
	
	VideoInfo, err := DB_GetVideo(RequestId)
	if err != nil {
		http.Error(w, "Internal error when getting video details.", http.StatusInternalServerError)
		return
	}
	if VideoInfo == nil {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}
	StreamedDirectory := VideoInfo.StreamedDirectory
	if StreamedDirectory == "" {
		http.Error(w, "Video contains no stream data attached.", http.StatusNotFound)
		return
	}
	if !DoesFileExist(StreamedDirectory) {
		http.Error(w, "Stream directory does not exist on disk?", http.StatusNotFound)
		return
	}
	
	FilePath := filepath.Join(StreamedDirectory, RequestFile)
	
	if !DoesFileExist(FilePath) {
		http.Error(w, fmt.Sprintf("Could not find file '%s'...", VideoInfo.DownloadedFilename), http.StatusNotFound)
		return
	}
	Filename := filepath.Base(FilePath)
	
	if DownloadVal := r.URL.Query().Get("download"); DownloadVal != "" {
		// User wants to download this video
		DownloadVal = strings.ToLower(DownloadVal)
		if DownloadVal == "true" || DownloadVal == "1" || DownloadVal == "yes" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", Filename))
		} else {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
		}
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
	}
	
	http.ServeFile(w, r, FilePath)
}

func ServeDBImage(w http.ResponseWriter, r *http.Request) {
	ImageId := path.Base(r.URL.Path)
	if len(ImageId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid image.", http.StatusBadRequest)
		return
	}
	
	ImageExtension := filepath.Ext(ImageId)
	ImageIdWithoutExt := strings.TrimSuffix(ImageId, ImageExtension)
	
	ImageInfo, err := DB_GetImageInfo(ImageIdWithoutExt)
	if err != nil {
		L_Printf("Failed to serve image because DB_GetImageInfo errored: %v\n", err)
		http.Error(w, "Error getting image info?", http.StatusInternalServerError)
		return
	}
	
	if ImageInfo == nil || filepath.Ext(ImageInfo.Filename) != ImageExtension {
		http.Error(w, "Image not found.", http.StatusNotFound)
		return
	}
	
	ImageData, err := DB_GetImageData(ImageIdWithoutExt)
	if err != nil {
		L_Printf("Failed to serve image because DB_GetImageData errored: %v\n", err)
		http.Error(w, "Error getting image info?", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", ImageInfo.Filename))
	
	Seeker := bytes.NewReader(ImageData)
	
	http.ServeContent(w, r, ImageInfo.Filename, ImageInfo.UpdatedAt, Seeker)
}

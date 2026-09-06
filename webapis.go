package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path"
	"fmt"
	"os"
	"bufio"
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
	
	DownloadDir    *string `json:"download_dir"`
	OutputTemplate *string `json:"output_template"`
	Type           int32  `json:"type"`
	
	QualitySelect int `json:"quality_select"`
	PreferredVideoFormat *string `json:"preferred_video_format"`
	PreferredAudioFormat *string `json:"preferred_audio_format"`
	
	CheckInterval     int64  `json:"check_interval"`
	FullCheckInterval int64 `json:"full_check_interval"`
	PlaylistEnd       int   `json:"playlist_end"`
	
	Enabled *bool `json:"enabled"`
}

func Verify_API_RequestChannelBody(Body API_RequestChannelBody) (bool, string) {
	if len(Body.Name) > 128 {
		return false, "Name must be shorter than 128 characters."
	}
	if len(Body.Url) > API_MAX_URL_LENGTH {
		return false, fmt.Sprintf("Url must be shorter than %d characters.", API_MAX_URL_LENGTH)
	}
	if strings.HasPrefix(strings.TrimSpace(Body.Url), "-") {
		return false, "Invalid channel URL, must NOT start with '-' character."
	}
	if Body.DownloadDir != nil && len(*Body.DownloadDir) > 1024 {
		return false, "DownloadDir must be shorter than 1024 characters."
	}
	if Body.OutputTemplate != nil && len(*Body.OutputTemplate) > 1024 {
		return false, "OutputTemplate must be shorter than 1024 characters."
	}
	
	if Body.PreferredVideoFormat != nil && len(*Body.PreferredVideoFormat) > 512 {
		return false, "PreferredVideoFormat must be shorter than 512 characters."
	}
	if Body.PreferredAudioFormat != nil && len(*Body.PreferredAudioFormat) > 512 {
		return false, "PreferredAudioFormat must be shorter than 512 characters."
	}
	
	return true, ""
}

func API_NewChannel(w http.ResponseWriter, r *http.Request) {
	var Body API_RequestChannelBody
	Body.FullCheckInterval = -1
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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
		Url:  Body.Url,
		
		Type: Body.Type,
		
		QualitySelect: Body.QualitySelect,
		
		CheckInterval: Body.CheckInterval,
		FullCheckInterval: Body.FullCheckInterval,
		PlaylistEnd: Body.PlaylistEnd,
	}
	if Body.DownloadDir != nil {
		NewChannel.DownloadDir = *Body.DownloadDir
	}
	if Body.OutputTemplate != nil {
		NewChannel.OutputTemplate = *Body.OutputTemplate
	}
	
	if Body.PreferredVideoFormat != nil {
		NewChannel.PreferredVideoFormat = *Body.PreferredVideoFormat
	}
	if Body.PreferredAudioFormat != nil {
		NewChannel.PreferredAudioFormat = *Body.PreferredAudioFormat
	}
	
	if NewChannel.CheckInterval < 0 {
		NewChannel.CheckInterval = 0
	}
	if NewChannel.FullCheckInterval < 0 {
		NewChannel.FullCheckInterval = 0
	}
	// This is intended behavior. I want all newly created channels to be paused by default ! (Even if the request wants it to be enabled...)
	NewChannel.Enabled = false
	
	err := AddArchiveChannel(G_ArchiveChannels, NewChannel)
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
	
	AChannel := GetArchiveChannelFromId(G_ArchiveChannels, RequestId)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	
	var Body API_RequestChannelBody
	Set_API_RequestChannelBodyDefaults(&Body)
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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
	
	if Body.DownloadDir != nil {
		AChannel.DownloadDir = *Body.DownloadDir
	}
	if Body.OutputTemplate != nil {
		AChannel.OutputTemplate = *Body.OutputTemplate
	}
	
	if Body.PreferredVideoFormat != nil {
		AChannel.PreferredVideoFormat = *Body.PreferredVideoFormat
	}
	if Body.PreferredAudioFormat != nil {
		AChannel.PreferredAudioFormat = *Body.PreferredAudioFormat
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
	
	AChannel := GetArchiveChannelFromId(G_ArchiveChannels, RequestId)
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
	
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
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
		CheckSettings := GetCheckSettingsFromAChannel(AChannel)
		CheckSettings.ForceEnable = true
		CheckSettings.CheckAllVideos = Body.CheckAll
		CheckSettings.Type = int32(Body.ChannelType)
		
		go CheckChannel(AChannel, CheckSettings)
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
	
	AChannel := GetArchiveChannelFromId(G_ArchiveChannels, RequestId)
	if AChannel == nil {
		http.Error(w, "Channel not found.", http.StatusNotFound)
		return
	}
	err := RemoveArchiveChannel(G_ArchiveChannels, RequestId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"Success\":true}"))
}

func API_SpiceUpChannelInfo(w http.ResponseWriter, r *http.Request, AChannel *ArchiveChannel) {
	TasksList, err := CL_ListCommandTasks(100, 0, ListCommandTasksQuery{
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
		AChannel := GetArchiveChannelFromId(G_ArchiveChannels, RequestId)
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
	
	G_ArchiveChannels.ChannelsLock.RLock()
	
	SendChannels := []*ArchiveChannel{}
	
	Channels := G_ArchiveChannels.Channels
	for _, AChannel := range(Channels) {
		NewChannel := &ArchiveChannel{}
		*NewChannel = *AChannel
		
		API_SpiceUpChannelInfo(w, r, NewChannel)
		
		SendChannels = append(SendChannels, NewChannel)
	}
	G_ArchiveChannels.ChannelsLock.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(SendChannels),
		"channels": SendChannels,
	})
}
func API_SpiceUpVideoInfo(w http.ResponseWriter, r *http.Request, Video *VideoInfo) {
	if Video.DownloadedFilename != "" {
		Filepath, err := GetDownloadedVideoFilePath(Video, nil)
		if err != nil {
			Video.VideoFileExists = false
		} else if Filepath == "" {
			// Video file was not found... Deleted?
			Video.VideoFileExists = false
		} else {
			Video.VideoFileExists = true
		}
	}
	
	if !Video.VideoFileExists && Video.StreamedDirectory != "" && Video.Status == VIDEO_STATUS_DOWNLOADING {
		// Video file doesn't exist... Share the m3u8 stream instead!
		
		ExpireTime := time.Now().UTC().Add(time.Second*60*60*24 * 7)  // m3u8 stream link will last 1 week...
		VideoStreamUrl := GenerateSignedUserRequest(fmt.Sprintf("/video-stream/%s/playlist.m3u8", Video.Id), []SQuery{
			{"expires", fmt.Sprintf("%d", ExpireTime.Unix())},
		})
		Video.VideoStreamUrl = VideoStreamUrl
	}
	
	if !Video.VideoFileExists && Video.VideoStreamUrl == "" {
		Video.FileSize = 0   // This is for the web ui, this doesn't save into the database.
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
	
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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
		AChannel = GetManualArchiveChannel(G_ArchiveChannels)
	} else {
		if len(Body.TargetChannel) > API_MAX_REQUEST_ID {
			http.Error(w, "Invalid target channel id.", http.StatusBadRequest)
			return
		}
		AChannel = GetArchiveChannelFromId(G_ArchiveChannels, Body.TargetChannel)
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
	
	CheckSettings := GetCheckSettingsFromAChannel(AChannel)
	CheckSettings.CheckUrls = []string{Body.DownloadUrl}
	CheckSettings.Type = int32(Body.Type)
	CheckSettings.QualitySelect = Body.QualitySelect
	
	// TODO: ManuallyAddVideos is a pretty shit function, I need to gut it or remove it and use something else...
	go ManuallyAddVideos(CheckSettings, Body.DownloadUrl, Body.Type, CS)
	
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
	RequestId := strings.TrimPrefix(r.URL.Path, "videos/")
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
			L_Printf("Exact RequestId: '%s'\n", RequestId)
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
		
		if Limit == -1 || Limit > 100 {
			Limit = 100
		}
	}
	if p := r.URL.Query().Get("page"); p != "" {
		Page, _ := strconv.Atoi(p)
		Offset = Limit*Page
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
		} else if o == "file_size" {
			OrderBy = DB_VIDEO_ORDERBY_FileSize
		}
	}
	
	if Offset < 0 {
		Offset = 0
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
		// Don't share description when listing... (Cut down on the amount of data sent)
		Video.Description = ""
		
		API_SpiceUpVideoInfo(w, r, Video)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(VideosList),
		"videos": VideosList,
		"stats":  Stats,
	})
}

type API_VideoUpdateBody struct {
	Status *int `json:"status"`
	RefreshState *bool `json:"refresh_state"`
}

func API_UpdateVideo_imp(VideoId string, UpdateSettings API_VideoUpdateBody) error {
	VideoInfo, err := DB_GetVideo(VideoId)
	if err != nil {
		return fmt.Errorf("Error when getting video: %v !", err)
	}
	if VideoInfo == nil {
		return fmt.Errorf("Video not found.")
	}
	
	if UpdateSettings.Status != nil && *UpdateSettings.Status >= 0 && *UpdateSettings.Status <= 10 {
		DB_UpdateVideoStatus(VideoInfo, *UpdateSettings.Status)
		if VideoInfo.Status == VIDEO_STATUS_IGNORED {
			DB_UpdateVideoQueuedAction(VideoInfo, VIDEO_QACTION_NONE)
		}
		if VideoInfo.Status == VIDEO_STATUS_IGNORED || VideoInfo.Status == VIDEO_STATUS_QUEUED {
			// Cancel all download tasks for this video
			CL_CancelTasksForVideo(VideoInfo.Id, TASK_TYPE_DOWNLOAD)
		}
	}
	
	if UpdateSettings.RefreshState != nil {
		if *UpdateSettings.RefreshState {
			DB_UpdateVideoRefreshState(VideoInfo, 1)
		} else {
			DB_UpdateVideoRefreshState(VideoInfo, 0)
		}
		
		AChannel := GetArchiveChannelFromId(G_ArchiveChannels, VideoInfo.FromChannel)
		if AChannel != nil {
			AChannel.NeedsRefreshing = true
		}
	}
	
	return nil
}

func API_UpdateVideo(w http.ResponseWriter, r *http.Request) {
	//RequestId := path.Base(r.URL.Path)
	RequestId := strings.TrimPrefix(r.URL.Path, "videos/")
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
	
	var Body API_VideoUpdateBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	err = API_UpdateVideo_imp(RequestId, Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VideoInfo)
}

func API_BulkUpdateVideos(w http.ResponseWriter, r *http.Request) {
	var Body []struct{
		VideoId string `json:"video_id"`
		Content API_VideoUpdateBody `json:"content"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<22))
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	Total := len(Body)
	Successes := 0
	
	AlreadyUpdated := make(map[string]bool)
	for _, VidRequest := range(Body) {
		VideoId := VidRequest.VideoId
		if VideoId == "" || len(VideoId) > API_MAX_REQUEST_ID { continue }
		
		if _, ok := AlreadyUpdated[VideoId]; ok { continue }  // Don't update the same video over and over again...
		AlreadyUpdated[VideoId] = true
		
		err := API_UpdateVideo_imp(VideoId, VidRequest.Content)
		if err != nil {
			L_Printf("Bulk update for video: \"%s\" Failed because: %v\n", VideoId, err)
		} else {
			Successes += 1
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{
		Total     int `json:"total"`
		Successes int `json:"successes"`
	}{Total: Total, Successes: Successes})
}

func API_DeleteVideo(w http.ResponseWriter, r *http.Request) {
	//RequestId := path.Base(r.URL.Path)
	RequestId := strings.TrimPrefix(r.URL.Path, "videos/")
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

func API_BulkDeleteVideos(w http.ResponseWriter, r *http.Request) {
	var Body []string  // Array of video ids to delete.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	Total := len(Body)
	Successes := 0
	
	DeletedVideoIds := []string{}
	AlreadyDeleted := make(map[string]bool)
	for _, VideoId := range(Body) {
		if VideoId == "" || len(VideoId) > API_MAX_REQUEST_ID { continue }
		
		if _, ok := AlreadyDeleted[VideoId]; ok { continue }  // Don't delete the same video over and over again...
		AlreadyDeleted[VideoId] = true
		
		VideoInfo, err := DB_GetVideo(VideoId)
		if err != nil {
			L_Printf("[Bulk Delete] Error when checking if video exists: %v !", err)
			continue
		}
		if VideoInfo == nil {
			L_Printf("[Bulk Delete] Video: \"%s\" not found.", VideoId)
			continue
		}
		err = DB_DeleteVideo(VideoInfo)
		if err != nil {
			L_Printf("[Bulk Delete] Error when deleting video: %v !", err)
			continue
		}
		Successes += 1
		DeletedVideoIds = append(DeletedVideoIds, VideoId)
		
		CL_CancelTasksForVideo(VideoInfo.Id, TASK_TYPE_DOWNLOAD)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{
		Total     int `json:"total"`
		Successes int `json:"successes"`
		DeletedVideos []string `json:"deleted_video_ids"`
	}{
		Total: Total,
		Successes: Successes,
		DeletedVideos: DeletedVideoIds,
	})
}

func API_ShareVideoFile(w http.ResponseWriter, r *http.Request) {
	// This is an authenticated request if called from ServeApi !
	
	RequestId := path.Base(r.URL.Path)
	if len(RequestId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	
	// Expire in a week.
	ExpireTime := time.Now().UTC().Add(time.Second*60*60*24 * 7)
	
	ShareLink := GenerateSignedUserRequest(fmt.Sprintf("/video-file/%s", RequestId), []SQuery{
		{"expires", fmt.Sprintf("%d", ExpireTime.Unix())},
	})
	
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(ShareLink))
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
		if Limit == -1 || Limit > 50 {
			Limit = 50
		}
	}
	if p := r.URL.Query().Get("page"); p != "" {
		Page, _ := strconv.Atoi(p)
		Offset = Limit*Page
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
	
	TasksList, err := CL_ListCommandTasks(Limit, Offset, Query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error when trying to list tasks, err: %v", err), http.StatusInternalServerError)
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
			L_Printf("Failed to get tasks list for stats... Err: %v\n", err)
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
	
	err = json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(TasksList),
		"tasks": TasksList,
		"stats": Stats,
	})
	if err != nil {
		// L_Printf("Error when encoding tasks to json! err: %v\n", err)
		http.Error(w, fmt.Sprintf("Error when trying to list tasks, err: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
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
		Download_Live_Chat *bool `json:"Download_Live_Chat"`
	}
	Body.TaskLog_AutoDelete_Seconds = -1
	Body.TaskLog_List_AutoDelete_Seconds = -1
	Body.AutoRefresh_Videos_Seconds = -1
	
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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
	if Body.Download_Live_Chat != nil {
		G_Config.Download_Live_Chat = *Body.Download_Live_Chat
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
	if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_API) { return }
	
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/")
	Path := r.URL.Path
	Method := r.Method
	
	w.Header().Set("Cache-Control", "private, no-cache")
	
	// Unauthorized endpoints
	
	if Path == "login" && Method == "POST" {
		AuthLoginRequest(w, r)
		return
	} else if Path == "whoami" && Method == "GET" {
		AUser, err := GetAuthUserFromRequest(r)
		if err != nil {
			L_Printf("/whoami/ Error when getting auth user from request, error: %v\n", err)
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AUser)
		return
	} else if Path == "create-admin" && Method == "POST" {
		if TestRateLimitForRequest(w, r, RATE_LIMIT_BUCKET_CREATEACCOUNT) {
			http.Error(w, "Too many account creation attempts, try again in a few minutes.", http.StatusTooManyRequests)
			return
		}
		
		AdminExists, err := DoesAdminAccountExist()
		if err != nil {
			L_Printf("Error checking if admin account exists: %v", err)
			http.Error(w, "Error checking if admin account exists...", http.StatusInternalServerError)
			return
		}
		if AdminExists {
			http.Error(w, "Admin account already exists!", http.StatusForbidden)
			return
		}
		
		Ip := GetIpAddressFromRequest(r)
		if !IsIpAddressLocal(Ip) {
			L_Printf("Attempt to create admin account on public ip address[%s]. Please connect via localhost!\nAlternatively, you can run the program with the arguments: './autoytdlpmanager --create-admin-user \"username\" --create-admin-password \"password\"'", Ip)
			
			
			http.Error(w, "Attempt to create admin account on public ip address. Please connect via localhost!\nAlternatively, you can run the program with the arguments: './autoytdlpmanager --create-admin-user \"username\" --create-admin-password \"password\"'", http.StatusForbidden)
			return
		}
		
		AuthCreateUserRequest(w, r, AUTH_ROLE_ADMIN)
		return
	} else if Path == "admin-account-exists" && Method == "GET" {
		Exists, err := DoesAdminAccountExist()
		if err != nil {
			L_Printf("Error checking if admin account exists: %v", err)
			http.Error(w, "Error checking if admin account exists...", http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exists": Exists,
		})
		return
	}
	
	AuthUser, err := GetAuthUserFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Auth error: %v", err), http.StatusInternalServerError)
		return
	}
	if AuthUser == nil {
		http.Error(w, "Unauthorized.", http.StatusUnauthorized)
		return
	}
	
	if AuthUser.Role != AUTH_ROLE_ADMIN && AuthUser.Role != AUTH_ROLE_USER_READONLY {
		http.Error(w, "Unauthorized.", http.StatusUnauthorized)
		return
	}
	/*
	if !IsAuthorized {
		http.Error(w, "Unauthorized.", http.StatusUnauthorized)
		return
	}
	*/
	
	IsAdminAuthorized := (AuthUser.Role == AUTH_ROLE_ADMIN)
	IsReadOnlyAuthorized := (AuthUser.Role == AUTH_ROLE_USER_READONLY || IsAdminAuthorized)
	
	// Authorized endpoints
	
	if Path == "channels" && Method == "POST" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// POSTing to api/channels will create a new channel.
		API_NewChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "PATCH" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// PATCHing to api/channels/{channel_id} will update a channel.
		API_UpdateChannel(w, r)
	} else if strings.HasPrefix(Path, "channels/") && Method == "DELETE" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// DELETE-ing to api/channels/{channel_id} will delete a channel.
		API_DeleteChannel(w, r)
	} else if (Path == "channels" || strings.HasPrefix(Path, "channels/")) && Method == "GET" {
		if !IsReadOnlyAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// api/channels will give the entire list of channels !
		// api/channels/{channel_id} will give you a specific channel.
		API_GetChannels(w, r)
	} else if strings.HasPrefix(Path, "check-channel-now/") && Method == "POST" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// PPOSTing to api/check-channel-now/{channel_id} make the channel be checked first chance it gets.
		API_CheckChannel(w, r)
	} else if (Path == "videos" || strings.HasPrefix(Path, "videos/")) && Method == "GET" {
		if !IsReadOnlyAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		/*
		  api/videos?limit={int}&offset={int}&status={int}&from_channel={channel_id}&order_by={order}&order_direction={1, -1} Will return a list of videos.
		  status, from_channel, order_by and order_direction are optional.
		  
		  api/videos/{video_id} will give you a specific video.
		*/
		API_GetVideos(w, r)
	} else if (strings.HasPrefix(Path, "videos/")) && Method == "PATCH" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// You can set status, refresh_state
		API_UpdateVideo(w, r)
	} else if (strings.HasPrefix(Path, "bulk-update-videos")) && Method == "PATCH" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_BulkUpdateVideos(w, r)
	} else if (strings.HasPrefix(Path, "videos/")) && Method == "DELETE" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		// DELETE api/videos/{video_id} will delete a video
		API_DeleteVideo(w, r)
	} else if (strings.HasPrefix(Path, "bulk-delete-videos")) && Method == "DELETE" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_BulkDeleteVideos(w, r)
	} else if (strings.HasPrefix(Path, "add-videos")) && Method == "POST" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_AddVideos(w, r)
	} else if (strings.HasPrefix(Path, "share-video-file/")) && Method == "GET" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_ShareVideoFile(w, r)
	} else if (Path == "tasks" || strings.HasPrefix(Path, "tasks/")) && Method == "GET" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		/*
		  api/tasks?limit={int}&offset={int}&status={int}&type={int}&from_channel={channel_id}&from_video={video_id}&order_by={order}&order_direction={1, -1}
		  
		  api/tasks/{video_id} will give you a specific task.
		*/
		API_GetTasks(w, r)
	} else if (strings.HasPrefix(Path, "cancel-task/")) && Method == "POST" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_CancelTask(w, r)
	} else if strings.HasPrefix(Path, "get-realtime-task-output/") && Method == "GET" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_GetTaskOutput(w, r)
	} else if strings.HasPrefix(Path, "config") && Method == "GET" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_GetConfig(w, r)
	} else if strings.HasPrefix(Path, "config") && Method == "PATCH" {
		if !IsAdminAuthorized {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		API_SetConfig(w, r)
	} else {
		fmt.Printf("Api path: '%s' Method: '%s' not found\n", Path, Method)
		http.NotFound(w, r)
	}
}

func IsRequestExpired(w http.ResponseWriter, r *http.Request) bool {
	ParseDidError := false
	
	var ExpireTime time.Time = time.Unix(0, 0)
	if ExpiresMsStr := r.URL.Query().Get("expires_ms"); ExpiresMsStr != "" {
		ExpiresMs, err := strconv.Atoi(ExpiresMsStr)
		if err != nil { ParseDidError = true}
		
		ExpireTime = time.UnixMilli(int64(ExpiresMs)).UTC()
	}
	if ExpiresStr := r.URL.Query().Get("expires"); ExpiresStr != "" {
		ExpiresUnix, err := strconv.Atoi(ExpiresStr)
		if err != nil { ParseDidError = true}
		
		ExpireTime = time.Unix(int64(ExpiresUnix), 0).UTC()
	}
	
	if ParseDidError {
		http.Error(w, "Bad Request - Could not parse expire query.", http.StatusBadRequest)
		return true
	}
	
	if ExpireTime.UnixMilli() > 0 && time.Now().UTC().UnixMilli() > ExpireTime.UnixMilli() {
		http.Error(w, "Expired request", http.StatusForbidden)
		return true
	}
	
	return false
}

func ServeVideoDownload(w http.ResponseWriter, r *http.Request) {
	IsAuthorized, err := IsRequestReadOnlyAuthorized(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Auth error: %v", err), http.StatusInternalServerError)
		return
	}
	
	if r.URL.Query().Get(AUTH_REQUEST_SIGN_QUERYNAME) != "" {
		IsSigned := IsUserRequestSignedByServer(r, []string{"expires_ms", "expires", "time_ms"})
		if !IsSigned {
			http.Error(w, "Forbidden - Unsigned or expired request", http.StatusForbidden)
			return
		}
	} else if !IsAuthorized {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	if IsRequestExpired(w, r) {
		return
	}
	
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
	AChannel := GetArchiveChannelFromId(G_ArchiveChannels, VideoInfo.FromChannel)
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
	
	ThrottledWriter := NewDynamicThrottledResponseWriter(w, r)
	
	http.ServeFile(ThrottledWriter, r, FilePath)
	ThrottledWriter.Close()
}

func ServeVideoStream(w http.ResponseWriter, r *http.Request) {
	IsAuthorized, err := IsRequestReadOnlyAuthorized(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Auth error: %v", err), http.StatusInternalServerError)
		return
	}
	
	SignedKey := r.URL.Query().Get(AUTH_REQUEST_SIGN_QUERYNAME)
	if SignedKey != "" {
		IsSigned := IsUserRequestSignedByServer(r, []string{"expires_ms", "expires", "time_ms"})
		if !IsSigned {
			http.Error(w, "Forbidden - Unsigned or expired request", http.StatusForbidden)
			return
		}
	} else if !IsAuthorized {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	if IsRequestExpired(w, r) {
		return
	}
	
	RawPath := r.URL.Path
	Path := strings.TrimPrefix(RawPath, "/video-stream/")
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
	
	Root, err := os.OpenRoot(StreamedDirectory)
	if err != nil {
		http.Error(w, "Error when opening root directory...", http.StatusInternalServerError)
		return
	}
	defer Root.Close()
	
	File, err := Root.Open(RequestFile)  // Check if file exists in directory. (Prevents path traversal!!!)
	if err != nil {
		http.Error(w, "Could not find requested file...", http.StatusNotFound)
		return
	}
	defer File.Close()
	
	FilePath := filepath.Join(StreamedDirectory, filepath.Clean(RequestFile))
	Filename := RequestFile
	
	var WriteContent *[]byte = nil
	
	if strings.HasSuffix(Filename, ".m3u8") && SignedKey != "" {
		// Extend 'expires'/'expires_ms' to the video segments!
		ExtendedContent, err := ExtendedM3U8Sign(FilePath, strings.TrimSuffix(RawPath, RequestFile), r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error when signing M3U8 content! '%v'...", err), http.StatusInternalServerError)
			return
		}
		
		WriteContent = &ExtendedContent
	} else if strings.HasSuffix(Filename, ".ts") {
		if SignedKey != "" {
			w.Header().Set("Cache-Control", "public, max-age=120")  // 2 minutes
		}
		
		w.Header().Set("Content-Type", "video/MP2T")
	}
	
	if DownloadVal := r.URL.Query().Get("download"); DownloadVal != "" {
		// User wants to download this file
		DownloadVal = strings.ToLower(DownloadVal)
		if DownloadVal == "true" || DownloadVal == "1" || DownloadVal == "yes" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", Filename))
		} else {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
		}
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", Filename))
	}
	
	if WriteContent != nil {
		w.Write(*WriteContent)
		return
	}
	
	ThrottledWriter := NewDynamicThrottledResponseWriter(w, r)
	
	http.ServeFile(ThrottledWriter, r, FilePath)
	ThrottledWriter.Close()
}

var CachedM3U8SegmentSigns = NewCache(time.Second * 120)

func ExtendedM3U8Sign(FilePath string, BasePath string, r *http.Request) ([]byte, error) {
	/*
	ShareLink := GenerateSignedUserRequest(fmt.Sprintf("/video-file/%s", RequestId), []SQuery{
		{"expires", fmt.Sprintf("%d", ExpireTime.Unix())},
	})
	*/
	expiresStr := r.URL.Query().Get("expires")
	
	File, err := os.Open(FilePath)
	if err != nil {
		return nil, err
	}
	defer File.Close()
	
	var NewContent bytes.Buffer
	
	scanner := bufio.NewScanner(File)
	
	CachedM3U8SegmentSigns.mutex.Lock()
	defer CachedM3U8SegmentSigns.mutex.Unlock()
	
	for scanner.Scan() {
		Line := scanner.Text()
		if strings.HasPrefix(Line, "#") {
			NewContent.WriteString(Line)
			NewContent.Write([]byte("\n"))
			continue
		}
		
		// Segment
		SegmentFileName := Line
		
		CacheName := fmt.Sprintf("%s/%s/%s", FilePath, SegmentFileName, expiresStr)
		CachedSignedFile, Exists := CachedM3U8SegmentSigns.GetNoMutex(CacheName)
		if Exists {
			NewContent.WriteString(CachedSignedFile.(string))
			NewContent.Write([]byte("\n"))
			continue
		}
		
		SignedFile := GenerateSignedUserRequest(fmt.Sprintf("%s%s", BasePath, SegmentFileName), []SQuery{
			{"expires", expiresStr},
		})
		
		SignedFile = strings.TrimPrefix(SignedFile, BasePath)
		
		NewContent.WriteString(SignedFile)
		NewContent.Write([]byte("\n"))
		
		CachedM3U8SegmentSigns.SetNoMutex(CacheName, SignedFile)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	return NewContent.Bytes(), nil
}

func ServeDBImage(w http.ResponseWriter, r *http.Request) {
	if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_GLOBAL) { return }
	
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
	w.Header().Set("Cache-Control", "public, max-age=604800")  // 1 week
	
	Seeker := bytes.NewReader(ImageData)
	
	http.ServeContent(w, r, ImageInfo.Filename, ImageInfo.UpdatedAt, Seeker)
}


func InitWebApis() {
	for true {
		time.Sleep(time.Second * 360)
		CachedM3U8SegmentSigns.CleanUp()
	}
}

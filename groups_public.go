package main

// Public (shared) channel groups.
// A group marked as public can be viewed by ANYONE with its share link, even when logged out.
// Everything here is read only and only exposes non sensitive information:
// no file paths, download directories, tasks, channel settings or internal ids of other groups.
//
//   Page:   /share/<token>
//   API:    GET /api/public/groups/<token>
//           GET /api/public/groups/<token>/videos
//           GET /api/public/groups/<token>/videos/<video id>
//   Files:  GET /share-file/<token>/<video id>[?download=1]
//
// Every request re-checks that the group is still public and that the video is inside it,
// so making a group private (or resetting its link) cuts off access immediately.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

const PUBLIC_VIDEOS_MAX_PAGE_SIZE = 48

type PublicGroupInfo struct {
	Name      string             `json:"name"`
	SubGroups []PublicSubGroup   `json:"sub_groups"`
	Channels  []PublicChannel    `json:"channels"`
}

type PublicSubGroup struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type PublicChannel struct {
	Name       string `json:"name"`
	SubGroupId string `json:"sub_group_id"`  // '' if the channel is directly inside the shared group.
}

// Whitelisted video information, safe to show to anyone.
type PublicVideo struct {
	Id           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Url          string `json:"url"`
	Availability string `json:"availability"`

	UploaderName string `json:"uploader"`
	UploaderUrl  string `json:"uploader_url"`
	ChannelName  string `json:"channel_name"`

	StoredThumbnail string `json:"stored_thumbnail"`
	OriginThumbnail string `json:"origin_thumbnail_url"`

	ReleaseDate int64   `json:"release_date"`
	Duration    float64 `json:"duration"`
	VideoType   int32   `json:"video_type"`
	Status      int     `json:"status"`

	HasFile  bool   `json:"has_file"`
	FileSize uint64 `json:"filesize"`
}

func publicVideoFromVideoInfo(Video *VideoInfo, IncludeDescription bool) PublicVideo {
	PV := PublicVideo{
		Id:           Video.Id,
		Title:        Video.Title,
		Url:          Video.Url,
		Availability: Video.Availability,

		UploaderName: Video.UploaderName,
		UploaderUrl:  Video.UploaderUrl,

		StoredThumbnail: Video.Thumbnail,
		OriginThumbnail: Video.OriginThumbnail,

		ReleaseDate: Video.ReleaseDate,
		Duration:    Video.Duration,
		VideoType:   Video.VideoType,
		Status:      Video.Status,
	}
	if IncludeDescription {
		PV.Description = Video.Description
	}
	if AChannel := GetArchiveChannelFromId(G_ArchiveChannels, Video.FromChannel); AChannel != nil {
		AChannel.Lock.RLock()
		PV.ChannelName = AChannel.Name
		AChannel.Lock.RUnlock()
	}
	if Video.DownloadedFilename != "" {
		FilePath, err := GetDownloadedVideoFilePath(Video, nil)
		if err == nil && FilePath != "" {
			PV.HasFile = true
			PV.FileSize = Video.FileSize
		}
	}
	return PV
}

func publicNotFound(w http.ResponseWriter) {
	// Same answer for "doesn't exist" and "not public", so links can't be probed.
	http.Error(w, "This shared group does not exist or is no longer public.", http.StatusNotFound)
}

// Returns the public group for the token, or writes an error and returns nil.
func publicGetGroupFromToken(w http.ResponseWriter, Token string) *ChannelGroup {
	Group, err := DB_GetPublicChannelGroupByToken(Token)
	if err != nil {
		L_Printf("Public group lookup error: %v\n", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return nil
	}
	if Group == nil {
		publicNotFound(w)
		return nil
	}
	return Group
}

// Is the video inside the public group (or one of its sub groups)?
func publicIsVideoInGroup(Group *ChannelGroup, Video *VideoInfo) (bool, error) {
	ChannelIds, Found, err := GetChannelIdsInGroup(Group.Id)
	if err != nil || !Found {
		return false, err
	}
	return slices.Contains(ChannelIds, Video.FromChannel), nil
}

func API_Public_GetGroup(w http.ResponseWriter, r *http.Request, Group *ChannelGroup) {
	Info := PublicGroupInfo{
		Name:      Group.Name,
		SubGroups: []PublicSubGroup{},
		Channels:  []PublicChannel{},
	}

	Groups, err := DB_ListChannelGroups()
	if err != nil {
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	GroupIds := map[string]string{Group.Id: ""}
	for _, G := range(Groups) {
		if G.ParentId == Group.Id {
			Info.SubGroups = append(Info.SubGroups, PublicSubGroup{Id: G.Id, Name: G.Name})
			GroupIds[G.Id] = G.Id
		}
	}

	G_ArchiveChannels.ChannelsLock.RLock()
	for _, AChannel := range(G_ArchiveChannels.Channels) {
		AChannel.Lock.RLock()
		if SubGroupId, ok := GroupIds[AChannel.GroupId]; ok && AChannel.GroupId != "" && !AChannel.Hidden {
			Info.Channels = append(Info.Channels, PublicChannel{Name: AChannel.Name, SubGroupId: SubGroupId})
		}
		AChannel.Lock.RUnlock()
	}
	G_ArchiveChannels.ChannelsLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Info)
}

func API_Public_ListVideos(w http.ResponseWriter, r *http.Request, Group *ChannelGroup) {
	Q := r.URL.Query()

	// Optionally narrow down to one of the shared group's sub groups.
	FilterGroupId := Group.Id
	if sg := Q.Get("sub_group"); sg != "" {
		SubGroup, err := DB_GetChannelGroup(sg)
		if len(sg) > API_MAX_REQUEST_ID || err != nil || SubGroup == nil || SubGroup.ParentId != Group.Id {
			http.Error(w, "Sub group not found.", http.StatusNotFound)
			return
		}
		FilterGroupId = SubGroup.Id
	}
	ChannelIds, Found, err := GetChannelIdsInGroup(FilterGroupId)
	if err != nil {
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	if !Found {
		publicNotFound(w)
		return
	}

	Limit := 24
	if l := Q.Get("limit"); l != "" {
		Limit, _ = strconv.Atoi(l)
		if Limit <= 0 || Limit > PUBLIC_VIDEOS_MAX_PAGE_SIZE {
			Limit = 24
		}
	}
	Page, _ := strconv.Atoi(Q.Get("page"))
	if Page < 0 {
		Page = 0
	}
	SearchQuery := Q.Get("search_query")
	if len(SearchQuery) > 256 {
		http.Error(w, "search_query must be shorter than 256 characters", http.StatusBadRequest)
		return
	}

	Query := ListVideosQuery{
		RefreshState: -1,
		QueuedAction: -1,
		VideoType:    -1,
		Status:       -1,

		UseFromChannelIds: true,
		FromChannelIds:    ChannelIds,
		SearchQuery:       SearchQuery,

		OrderBy:        DB_VIDEO_ORDERBY_ReleaseDate,
		OrderDirection: -1,

		ExcludeDescription: true,
	}
	// Only "downloaded" can be filtered for, the other statuses are internal details.
	if Q.Get("downloaded_only") == "1" {
		Query.Status = VIDEO_STATUS_DOWNLOADED
	}
	switch Q.Get("order_by") {
	case "added_at":
		Query.OrderBy = DB_VIDEO_ORDERBY_AddedAt
	case "file_size":
		Query.OrderBy = DB_VIDEO_ORDERBY_FileSize
	}
	if Q.Get("order_direction") == "1" {
		Query.OrderDirection = 1
	}

	Videos, err := DB_ListVideos(Limit, Limit*Page, Query)
	if err != nil {
		L_Printf("Public list videos error: %v\n", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	Stats, _ := DB_GetVideoStatsFromQuery(Query)
	Total := 0
	if Stats != nil {
		Total = Stats.Total
	}

	PublicVideos := make([]PublicVideo, 0, len(Videos))
	for _, Video := range(Videos) {
		PublicVideos = append(PublicVideos, publicVideoFromVideoInfo(Video, false))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":     Total,
		"page":      Page,
		"page_size": Limit,
		"videos":    PublicVideos,
	})
}

func API_Public_GetVideo(w http.ResponseWriter, r *http.Request, Group *ChannelGroup, VideoId string) {
	if VideoId == "" || len(VideoId) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid video id.", http.StatusBadRequest)
		return
	}
	Video, err := DB_GetVideo(VideoId)
	if err != nil {
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	InGroup := false
	if Video != nil {
		InGroup, err = publicIsVideoInGroup(Group, Video)
		if err != nil {
			http.Error(w, "Internal server error.", http.StatusInternalServerError)
			return
		}
	}
	if !InGroup {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(publicVideoFromVideoInfo(Video, true))
}

// Handles GET /api/public/... (no login required). Returns true if the path was handled.
func ServePublicApi(w http.ResponseWriter, r *http.Request, Path string) bool {
	if !strings.HasPrefix(Path, "public/groups/") {
		return false
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return true
	}

	// public/groups/<token>[/videos[/<video id>]]
	Parts := strings.SplitN(strings.TrimPrefix(Path, "public/groups/"), "/", 3)
	Group := publicGetGroupFromToken(w, Parts[0])
	if Group == nil {
		return true
	}

	switch {
	case len(Parts) == 1:
		API_Public_GetGroup(w, r, Group)
	case len(Parts) == 2 && Parts[1] == "videos":
		API_Public_ListVideos(w, r, Group)
	case len(Parts) == 3 && Parts[1] == "videos":
		API_Public_GetVideo(w, r, Group, Parts[2])
	default:
		http.NotFound(w, r)
	}
	return true
}

// GET /share-file/<token>/<video id>[?download=1]
func ServePublicVideoFile(w http.ResponseWriter, r *http.Request) {
	if RateLimitRequest(w, r, RATE_LIMIT_BUCKET_API) { return }
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	Parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/share-file/"), "/")
	if len(Parts) != 2 || Parts[1] == "" || len(Parts[1]) > API_MAX_REQUEST_ID {
		http.Error(w, "Invalid request path.", http.StatusBadRequest)
		return
	}

	Group := publicGetGroupFromToken(w, Parts[0])
	if Group == nil {
		return
	}

	Video, err := DB_GetVideo(Parts[1])
	if err != nil {
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}
	InGroup := false
	if Video != nil {
		InGroup, err = publicIsVideoInGroup(Group, Video)
		if err != nil {
			http.Error(w, fmt.Sprintf("Internal server error."), http.StatusInternalServerError)
			return
		}
	}
	if !InGroup {
		http.Error(w, "Video not found.", http.StatusNotFound)
		return
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "private, no-cache")
	ServeVideoFileForVideo(w, r, Video)
}

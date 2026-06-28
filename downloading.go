package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/google/uuid"
	//"os/exec"
	//"fmt"
)

const (
	ACHANNEL_TYPE_VIDEOS  = 0
	ACHANNEL_TYPE_LIVE    = 1
	ACHANNEL_TYPE__UNUSED = 2
	ACHANNEL_TYPE_LIST_AND_IGNORE = 3
)

const (
	VIDEO_STATUS_QUEUED      = 0
	VIDEO_STATUS_DOWNLOADING = 1
	VIDEO_STATUS_DOWNLOADED  = 2
	VIDEO_STATUS_FAILED      = 3
	VIDEO_STATUS_IGNORED     = 4
)
const (
	VIDEO_QACTION_NONE = 0
	VIDEO_QACTION_WILL_DOWNLOAD = 1
	VIDEO_QACTION_WILL_IGNORE = 2
)

const (
	MANUAL_CHANNEL_ID = "-Manual-Channel-"
)

type ArchiveChannel struct {
	Lock sync.RWMutex `json:"-"`
	
	Id   string `json:"id"`
	Name string `json:"name"`
	Url  string `json:"url"`
	
	DownloadDir    string `json:"download_dir"`
	OutputTemplate string `json:"output_template"`
	QualitySelect  int    `json:"quality_select"`
	Type           int32  `json:"type"`
	CheckInterval  int64  `json:"check_interval"`
	FullCheckInterval int64 `json:"full_check_interval"`
	
	NeedsRefreshing bool `json:"-"`
	
	Enabled        bool `json:"enabled"`
	Hidden         bool `json:"hidden"`
	IsBeingChecked bool `json:"-"`
	
	NextCheckMSEC            int64 `json:"_nextCheckMsec"`
	NextFullChannelCheckMSEC int64 `json:"_nextFullChannelCheckMsec"`
	
	PlaylistEnd int `json:"playlist_end"`
	
	FORAPI_TasksCount   int `json:"tasks_count"`
	FORAPI_ActiveTaskId string `json:"active_task"`
}

type WatchingBundle struct {
	ChannelsLock sync.RWMutex
	Channels []*ArchiveChannel
}

var WatchedDownloading WatchingBundle

func AddArchiveChannel(WD *WatchingBundle, AChannel *ArchiveChannel) error {
	if AChannel.Id == "" {
		AChannel.Id = uuid.New().String()
	}
	
	err := DB_UpdateArchiveChannel(AChannel)
	if err != nil {
		L_Printf("!!! COULD NOT ADD CHANNEL: \"%s\" TO DATABASE ERR: %v !!!\n", AChannel.Url, err)
		return err
	}
	
	WD.ChannelsLock.Lock()
	WD.Channels = append(WD.Channels, AChannel)
	WD.ChannelsLock.Unlock()
	
	return nil
}
func RemoveArchiveChannel(WD *WatchingBundle, Id string) error {
	err := DB_RemoveChannel(Id)
	if err != nil {
		L_Printf("Could not remove channel from database err: %v\n", err)
		return err
	}
	WD.ChannelsLock.Lock()
	defer WD.ChannelsLock.Unlock()
	NewChannels := make([]*ArchiveChannel, 0, len(WD.Channels))
	
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id {
			AChannel.Enabled = false
			continue
		}
		
		NewChannels = append(NewChannels, AChannel)
	}
	
	WD.Channels = NewChannels
	
	return nil
}

func GetManualArchiveChannel(WD *WatchingBundle) *ArchiveChannel {
	AChannel := GetArchiveChannelFromId(WD, MANUAL_CHANNEL_ID)
	if AChannel == nil {
		// Create the manual channel
		AChannel = &ArchiveChannel{
			Id: MANUAL_CHANNEL_ID,
			Name: "Manually Downloaded Videos",
			Enabled: false,
			
			Hidden: true,
			PlaylistEnd: -1,
		}
		err := AddArchiveChannel(WD, AChannel)
		if err != nil {
			L_Printf("Error when creating manual channel: %s\n", err)
			return nil
		}
	} else {
		AChannel.Hidden = true
	}
	
	return AChannel
}

func GetArchiveChannelFromId(WD *WatchingBundle, Id string) *ArchiveChannel {
	WD.ChannelsLock.RLock()
	defer WD.ChannelsLock.RUnlock()
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id {
			return AChannel
		}
	}
	
	return nil
}

func DoesFileExist(FilePath string) bool {
	_, err := os.Stat(FilePath)
	if err == nil {
		// File exists!
		return true
	}
	
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	L_Printf("DoesFileExist error %v\n", err)
	return false
}

func GetDownloadedVideoFilePath(Video *VideoInfo, AChannel *ArchiveChannel) (string, error) {
	if AChannel == nil {
		AChannel = GetArchiveChannelFromId(&WatchedDownloading, Video.FromChannel)
		if AChannel == nil {
			return "", fmt.Errorf("Channel for video could not be found.")
		}
	}
	
	var err error
	DownloadDir := GetDownloadDir(AChannel)
	DownloadDir, err = filepath.Abs(DownloadDir)
	if err != nil {
		return "", err
	}
	
	Filename := Video.DownloadedFilename
	if Filename == "" {
		return "", nil
	}
	
	FilePath := filepath.Join(DownloadDir, Filename)
	if DoesFileExist(FilePath) {
		return FilePath, nil
	}
	
	// Sneaky video might have an entirely different file extension
	FileExtension := filepath.Ext(Filename)
	FilenameWithoutExt := strings.TrimSuffix(Filename, FileExtension)
	
	AlternativeFileExtensions := []string{".mp4", ".mkv", ".webm", ".mov"}
	for _, Ext := range(AlternativeFileExtensions) {
		FilePath = filepath.Join(DownloadDir, fmt.Sprintf("%s%s", FilenameWithoutExt, Ext))
		if DoesFileExist(FilePath) {
			return FilePath, nil
		}
	}
	return "", nil
}

func CheckIsVideoDownloaded(Video *VideoInfo) bool {
	DB_VideoInfo, err := DB_GetVideo(Video.Id)
	if err != nil {
		L_Printf("CheckIsVideoDownloaded err: %v\n", err)
		return false
	}
	if DB_VideoInfo != nil {
		if DB_VideoInfo.Status == VIDEO_STATUS_DOWNLOADED ||
		   DB_VideoInfo.Status == VIDEO_STATUS_DOWNLOADING ||
		   DB_VideoInfo.Status == VIDEO_STATUS_IGNORED {
			return true
		}
		return false
	}
	
	// Maybe...
	return false
}

func GetHistoryDifference(OldVideoInfo *VideoInfo, NewVideoInfo *VideoInfo) *VideoInfoHistory {
	if OldVideoInfo.Id == "" {
		// Wat??? Video might not have existed before?
		return nil
	}
	
	ChangesWereMade := false
	HistoryInfo := &VideoInfoHistory{
		HId: -1,
		RevisionNumber: -1,
		
		Url: OldVideoInfo.Url,
		Id:  OldVideoInfo.Id,
		
		Duration:  -1,
		VideoType: -1,
		
		AddedAt:   OldVideoInfo.UpdatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	
	if OldVideoInfo.Title != NewVideoInfo.Title {
		ChangesWereMade = true
		HistoryInfo.Title = &OldVideoInfo.Title
	}
	if OldVideoInfo.Description != NewVideoInfo.Description {
		ChangesWereMade = true
		HistoryInfo.Description = &OldVideoInfo.Description
	}
	if OldVideoInfo.Availability != NewVideoInfo.Availability {
		ChangesWereMade = true
		HistoryInfo.Availability = &OldVideoInfo.Availability
	}
	
	if OldVideoInfo.Thumbnail != NewVideoInfo.Thumbnail || OldVideoInfo.OriginThumbnail != NewVideoInfo.OriginThumbnail {
		ChangesWereMade = true
		HistoryInfo.Thumbnail = &OldVideoInfo.Thumbnail
		HistoryInfo.OriginThumbnail = &OldVideoInfo.OriginThumbnail
	}
	if OldVideoInfo.Duration != NewVideoInfo.Duration {
		ChangesWereMade = true
		HistoryInfo.Duration = OldVideoInfo.Duration
	}
	
	if OldVideoInfo.VideoType != NewVideoInfo.VideoType {
		ChangesWereMade = true
		HistoryInfo.VideoType = OldVideoInfo.VideoType
	}
	
	if ChangesWereMade {
		return HistoryInfo
	}
	
	// No changes were made
	return nil
}

func AddVideoHistoryPoint(OldVideoInfo *VideoInfo, NewVideoInfo *VideoInfo) bool {
	HistoryDifference := GetHistoryDifference(OldVideoInfo, NewVideoInfo)
	if HistoryDifference == nil {
		L_Printf("No history changes found for video: %s !!!\n", NewVideoInfo.Id)
		// No changes...
		return false
	}
	
	//L_Printf("Adding history point %+v !!!\n", HistoryDifference)
	
	err := DB_AddVideoHistoryPoint(HistoryDifference)
	if err != nil {
		L_Printf("Failed to add video history point to database!!! Error: %v\n", err)
		return false
	}
	
	// History point successfully added to database!
	return true
}

func RefreshVideoInfo(AChannel *ArchiveChannel, Video *VideoInfo, Task *CommandTask) {
	UpdateVideoFileSize(Video, AChannel)
	
	// Hopefully the current video info is from the database!!!
	OldVideoInfo := *Video
	
	err := RequestVideoInfo(AChannel, Video.Url, -1, Video, Task)
	
	AddVideoHistoryPoint(&OldVideoInfo, Video)
	
	if OldVideoInfo.Availability != Video.Availability {
		DB_UpdateVideoAvailability(Video, Video.Availability)
	}
	if err != nil {
		CL_Logf(Task, "Failed to grab video info... err: %v\n", err)
		DB_UpdateVideoInfo(Video)
		DB_UpdateVideoRefreshState(Video, 0)
		return
	}
	DB_UpdateVideoInfo(Video)
	DB_UpdateVideoRefreshState(Video, 0)
	
	if Video.OriginThumbnail != "" {
		_, err := DownloadThumbnailForVideo(Video, Video.OriginThumbnail)
		if err != nil {
			CL_Logf(Task, "Failed to download thumbnail | %v\n", err)
		}
	}
	
	if Video.QueuedAction == VIDEO_QACTION_WILL_IGNORE {
		// This video was queued to be ignored.
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_IGNORED)
		DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_NONE)
	}
}

func GetFileSize(FilePath string) (int64, error) {
	FileStat, err := os.Stat(FilePath)
	if err != nil {
		return 0, err
	}
	
	return FileStat.Size(), nil
}

func UpdateVideoFileSize(Video *VideoInfo, AChannel *ArchiveChannel) {
	FilePath, fpErr := GetDownloadedVideoFilePath(Video, AChannel)
	if FilePath == "" && Video.Status == VIDEO_STATUS_DOWNLOADING && Video.VideoType == VIDEO_TYPE_ISLIVE {
		// Find ytarchive temp directory
		ytaState, ok := Get_ytarchive_State(GetDownloadDir(AChannel), Video.Id)
		
		var TotalSize uint64
		
		if ok && ytaState != nil {
			Files, err := os.ReadDir(ytaState.TempDir)
			if err == nil {
				for _, File := range(Files) {
					if File.IsDir() { continue }
					
					Info, err := File.Info()
					if err == nil {
						TotalSize += uint64(Info.Size())
					} else {
						L_Printf("[UpdateVideoFileSize] Failed to read file info '%s', error: %v\n", filepath.Join(ytaState.TempDir, File.Name()), err)
					}
				}
			} else {
				L_Printf("[UpdateVideoFileSize] Failed to read directory '%s', error: %v\n", ytaState.TempDir, err)
			}
		}
		
		L_Printf("TotalSize: %d\n", TotalSize)
		
		DB_UpdateVideoFileSize(Video, TotalSize)
		
		return
	}
	
	if fpErr == nil && FilePath != "" {
		FileSize, err := GetFileSize(FilePath)
		if err == nil {
			DB_UpdateVideoFileSize(Video, uint64(FileSize))
		} else {
			L_Printf("[UpdateVideoFileSize] Failed to get file size for '%s', error: %v\n", FilePath, err)
		}
	} else if FilePath == "" {
		// File doesn't exist.
		DB_UpdateVideoFileSize(Video, 0)
	}
}

func ConvertImageDataToJpg(ImageContent []byte) ([]byte, error) {
	JpegContent, err := RunFFmpegWithStdinStdout(ImageContent,
		[]string{
			"-i", "pipe:0",   // Stdin
			"-q:v", "10",
			"-f", "image2pipe",
			"-c:v", "mjpeg",
			"-vf", "scale='min(1920,iw)':'min(1080,ih)':force_original_aspect_ratio=decrease",
			"-vframes", "1",
			
			"pipe:1",
		},
	)
	if err != nil {
		return nil, err
	}
	
	return JpegContent, nil
}

func RunFFmpegWithStdinStdout(Input []byte, args []string) ([]byte, error) {
	cmd := exec.Command(Get_FFmpegPath(G_Config), args...)
	
	cmd.Stdin = bytes.NewReader(Input)
	
	var stdout, stderr bytes.Buffer
	
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w\nStderr: %s", err, stderr.String())
	}
	
	return stdout.Bytes(), nil
}

func DownloadThumbnailForVideo(Video *VideoInfo, ThumbnailUrl string) (string, error) {
	if !G_Config.Download_Video_Thumbnails {
		// Don't download thumbnail!
		return "", nil
	}
	
	Client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	Response, err := Client.Get(ThumbnailUrl)
	if err != nil {
		return "", fmt.Errorf("Failed because http.Get error: %v", err)
	}
	defer Response.Body.Close()
	
	if Response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Failed because response status: %s", Response.Status)
	}
	
	ContentType := Response.Header.Get("Content-Type")
	
	ImageContainer := ""
	if ContentType == "image/jpeg" {
		ImageContainer = "jpg"
	} else if ContentType == "image/png" {
		ImageContainer = "png"
	} else if ContentType == "image/webp" {
		ImageContainer = "webp"
	} else if ContentType == "image/avif" {
		ImageContainer = "avif"
	} else {
		return "", fmt.Errorf("Unknown Content-Type: '%s'", ContentType)
	}
	
	LimitedBodyStream := io.LimitReader(Response.Body, DB_IMAGE_MAX_FILESIZE)
	
	ImageContent, err := io.ReadAll(LimitedBodyStream)
	if err != nil {
		return "", fmt.Errorf("Could not read response body because: %v", err)
	}
	
	tmp := make([]byte, 1)
	n, _ := Response.Body.Read(tmp)
	if n > 0 {
		// Image file is too big!!!
		return "", fmt.Errorf("The downloaded thumbnail is larger than 10MB...")
	}
	
	RawHash := sha3.Sum256(ImageContent)
	ImageHash := base64.RawURLEncoding.EncodeToString(RawHash[0:32])
	
	if ImageContainer != "jpg" && ImageContainer != "png" {
		// Auto convert webp and avif to jpeg (We don't want none of that yucky shit 🤣)
		JpegContent, err := ConvertImageDataToJpg(ImageContent)
		if err == nil {
			ImageContent = JpegContent
			ImageContainer = "jpg"
		} else if err != nil {
			L_Printf("Failed to convert '%s' to jpg because %v\n", ImageContainer, err)
		}
	}
	
	ImageId := ImageHash
	NewDBImage := &DB_Image{
		Id: ImageId,
		Filename: fmt.Sprintf("%s-thumbnail.%s", Video.Id, ImageContainer),
		
		Type: DB_IMAGE_TYPE_THUMBNAIL,
	}
	err = DB_UpdateImage(NewDBImage)
	if err != nil {
		return "", fmt.Errorf("DB_UpdateImage Database error: %v", err)
	}
	
	err = DB_SetImageData(NewDBImage, ImageContent)
	if err != nil {
		return "", fmt.Errorf("Could not save image into database because: %v", err)
	}
	
	StoredThumbnailId := fmt.Sprintf("%s.%s", NewDBImage.Id, ImageContainer)
	
	DB_UpdateVideoStoredThumbnail(Video, StoredThumbnailId)
	
	return StoredThumbnailId, nil
}

func DownloadVideo(AChannel *ArchiveChannel, Video *VideoInfo, QualitySelect int, Task *CommandTask) error {
	DB_UpdateVideoStatus(Video, VIDEO_STATUS_DOWNLOADING)
	err := yt_dlp_DownloadVideo(AChannel, Video, QualitySelect)
	if err != nil {
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_FAILED)
		return err
	}
	DB_UpdateVideoStatus(Video, VIDEO_STATUS_DOWNLOADED)
	DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_NONE)
	
	UpdateVideoFileSize(Video, AChannel)
	
	return nil
}
func DownloadYTLive(AChannel *ArchiveChannel, Video *VideoInfo, QualitySelect int, Task *CommandTask) {
	DB_UpdateVideoStatus(Video, VIDEO_STATUS_DOWNLOADING)
	err := ytarchive_DownloadLive(AChannel, Video, QualitySelect)
	if err != nil {
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_FAILED)
		return
	}
	DB_UpdateVideoStatus(Video, VIDEO_STATUS_DOWNLOADED)
	DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_NONE)
	
	RefreshVideoInfo(AChannel, Video, Task)
}

type ChannelCheckSettings struct{
	CheckUrl    string
	ForceEnable bool
	OverrideChannelType int
	
	QualitySelect int
	
	CheckAllVideos bool
}

func CheckVideoAndDownload(AChannel *ArchiveChannel, Video *VideoInfo, Task *CommandTask, CheckSettings ChannelCheckSettings) bool {
	if CheckIsVideoDownloaded(Video) {
		return false
	}
	
	CL_Logf(Task, "Checking video: \"%s\" %s\n", Video.Title, Video.Url)
	
	ChannelType := AChannel.Type
	if CheckSettings.OverrideChannelType >= 0 {
		ChannelType = int32(CheckSettings.OverrideChannelType)
	}
	
	if ChannelType != ACHANNEL_TYPE__UNUSED && ChannelType != ACHANNEL_TYPE_LIST_AND_IGNORE {
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_DOWNLOADING)
	}
	
	QualitySelect := AChannel.QualitySelect
	if CheckSettings.QualitySelect >= 0 {
		QualitySelect = CheckSettings.QualitySelect
	}
	
	OldDBVideoInfo, err := DB_GetVideo(Video.Id)
	if err != nil {
		L_Printf("Failed to get database video info for video: %s error: %v", Video.Id, err)
	}
	
	err = RequestVideoInfo(AChannel, Video.Url, QualitySelect, Video, Task)
	if (Task != nil && Task.Status != TASK_STATUS_RUNNING) {
		// This task was canceled!! Don't do anything else.
		return false
	}
	if err != nil {
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_FAILED)
		if OldDBVideoInfo == nil || OldDBVideoInfo.Availability != Video.Availability {
			// TODO: Create a history point because the availability changed!!!
			// I can't do that rn because the video info that we have might not be from the database and have no title, description, duration, etc...
			DB_UpdateVideoAvailability(Video, Video.Availability)
		}
		CL_Logf(Task, "Failed to grab video info for \"%s\"... Error: %v\n", Video.Title, err)
		return false
	}
	
	if OldDBVideoInfo != nil {
		AddVideoHistoryPoint(OldDBVideoInfo, Video)
	}
	
	if Video.OriginThumbnail != "" && Video.Thumbnail == "" {
		_, err := DownloadThumbnailForVideo(Video, Video.OriginThumbnail)
		if err != nil {
			CL_Logf(Task, "Failed to download thumbnail | %v\n", err)
		}
	}
	
	DB_UpdateVideoInfo(Video)
	
	if ChannelType == ACHANNEL_TYPE__UNUSED || ChannelType == ACHANNEL_TYPE_LIST_AND_IGNORE {
		if ChannelType == ACHANNEL_TYPE_LIST_AND_IGNORE {
			DB_UpdateVideoStatus(Video, VIDEO_STATUS_IGNORED)
		}
		
		return false
	}
	
	DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_WILL_DOWNLOAD)
	
	switch Video.VideoType {
	case VIDEO_TYPE_ISLIVE:
		// TODO:
		if Video.Url[0:23] == "https://www.youtube.com" {
			// use ytarchive
			CL_Logf(Task, "Downloading live stream: \"%s\" %s\n", Video.Title, Video.Url)
			go DownloadYTLive(AChannel, Video, QualitySelect, Task)
			break
		}
		
		fallthrough
	case VIDEO_TYPE_WASLIVE: fallthrough
	case VIDEO_TYPE_VIDEO:   fallthrough
	default:
		if Task != nil {
			CL_Logf(Task, "Downloading video: \"%s\" %s\n", Video.Title, Video.Url)
			DB_UpdateCommandTaskInfo(Task)
		}
		err := DownloadVideo(AChannel, Video, QualitySelect, Task)
		if (Task != nil && Task.Status != TASK_STATUS_RUNNING) {
			return false
		}
		if err != nil {
			CL_Logf(Task, "Failed to download video \"%s\" because: %v\n", Video.Title, err)
		}
	}
	
	return true
}

func IsChannelEnabled(AChannel *ArchiveChannel) bool {
	if G_Config.AllChannels_Disabled {
		return false
	}
	return AChannel.Enabled
}
func IsChannelEnabledWithCheck(AChannel *ArchiveChannel, CheckSettings ChannelCheckSettings) bool {
	if CheckSettings.ForceEnable {
		return true
	}
	return IsChannelEnabled(AChannel)
}

const (
	CHECK_STATUS_GETTING_VIDEOS  = 0
	CHECK_STATUS_ADDED_VIDEOS    = 1
	CHECK_STATUS_CHECKING_VIDEOS = 2
	CHECK_STATUS_FINISHED        = 3
	
	
	CHECK_STATUS_FAILED = 10
)

type CheckStatus struct{
	Mutex sync.RWMutex
	
	Status int
}

func SetCheckStatus(CS *CheckStatus, NewStatus int) {
	if CS == nil {
		return
	}
	
	CS.Mutex.Lock()
	CS.Status = NewStatus
	CS.Mutex.Unlock()
}

func ManuallyAddVideos(AChannel *ArchiveChannel, Url string, Type int, QualitySelect int, CS *CheckStatus) {
	Task := CL_NewGenericTask()
	defer func() {
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	Task.Type = TASK_TYPE_LISTING
	Task.FromChannelId = AChannel.Id
	Task.Title = fmt.Sprintf("Checking: \"%s\"", Url)
	DB_UpdateCommandTaskInfo(Task)
	
	SetCheckStatus(CS, CHECK_STATUS_GETTING_VIDEOS)
	VideoList, err := yt_dlp_ListVideos(Url, -1, Task)
	if err != nil {
		SetCheckStatus(CS, CHECK_STATUS_FAILED)
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
		return
	}
	DB_UpdateCommandTaskInfo(Task)
	
	CL_Logf(Task, "Found %d videos\n", len(VideoList))
	
	// Add the videos to the queued list.
	for i := len(VideoList)-1; i >= 0; i-- {
		if Task.Status != TASK_STATUS_RUNNING {
			SetCheckStatus(CS, CHECK_STATUS_FAILED)
			return
		}
		
		Video := &VideoList[i]
		Video.FromChannel = AChannel.Id
		Exists, err := DB_GetVideo(Video.Id)
		if Exists == nil && err == nil {
			CL_Logf(Task, "Adding new video \"%s\" %s ! \n", Video.Id, Video.Url)
			DB_UpdateVideoInfo(Video)
		} else if err == nil && Type != ACHANNEL_TYPE_LIST_AND_IGNORE {
			Video = Exists
			DB_UpdateVideoStatus(Video, VIDEO_STATUS_QUEUED)
		}
		
		if Type == ACHANNEL_TYPE_LIST_AND_IGNORE {
			DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_WILL_IGNORE)
		} else if Type == ACHANNEL_TYPE_VIDEOS || Type == ACHANNEL_TYPE_LIVE {
			DB_UpdateVideoQueuedAction(Video, VIDEO_QACTION_WILL_DOWNLOAD)
		}
		
		time.Sleep(time.Millisecond * 1)
		
		DB_UpdateCommandTaskInfo(Task)
	}
	
	SetCheckStatus(CS, CHECK_STATUS_ADDED_VIDEOS)
	
	if len(VideoList) <= 0 {
		// This must just be a video url...
	}
	
	if QualitySelect == -1 {
		QualitySelect = AChannel.QualitySelect
	}
	
	CheckSettings := ChannelCheckSettings{
		OverrideChannelType: Type,
		QualitySelect: QualitySelect,
	}
	
	SetCheckStatus(CS, CHECK_STATUS_CHECKING_VIDEOS)
	for _, Video := range(VideoList) {
		if Task.Status != TASK_STATUS_RUNNING {
			SetCheckStatus(CS, CHECK_STATUS_FAILED)
			return
		}
		
		Video.FromChannel = AChannel.Id
		CheckVideoAndDownload(AChannel, &Video, Task, CheckSettings)
		DB_UpdateCommandTaskInfo(Task)
	}
	
	CL_FinishTask(Task, TASK_STATUS_FINISHED)
}

func IsVideoIdInVideosList(List []*VideoInfo, VideoId string) bool {
	for _, Vid := range(List) {
		if Vid.Id == VideoId {
			return true
		}
	}
	
	return false
}

var MaxVideosToRefresh = 20
func GetRefreshableVideos(AChannel *ArchiveChannel) ([]*VideoInfo) {
	RefreshableVideos := []*VideoInfo{}
	
	// Get videos with RefreshState = 1
	List1, err := DB_ListVideos(MaxVideosToRefresh, 0, ListVideosQuery{
		FromChannelId: AChannel.Id,
		RefreshState: 1,
		Status: -1,
		VideoType: -1,
		QueuedAction: -1,
	})
	if err == nil {
		RefreshableVideos = append(RefreshableVideos, List1...)
	} else {
		L_Printf("Failed to get refreshable videos from DB_ListVideos, err: %v\n", err)
	}
	
	
	// Get videos that are currently 'live' and check if they are still being downloaded.
	List2, err := DB_ListVideos(-1, 0, ListVideosQuery{
		FromChannelId: AChannel.Id,
		VideoType: VIDEO_TYPE_ISLIVE,
		RefreshState: -1,
		Status: -1,
		QueuedAction: -1,
	})
	if err == nil {
		for _, Vid := range(List2) {
			// Is this 'live' video still downloading? or queued even?? if not then add it to the list.
			if Vid.Status != VIDEO_STATUS_DOWNLOADING && Vid.Status != VIDEO_STATUS_QUEUED && !IsVideoIdInVideosList(RefreshableVideos, Vid.Id) {
				RefreshableVideos = append(RefreshableVideos, Vid)
			}
			if len(RefreshableVideos) > MaxVideosToRefresh {
				continue
			}
		}
	} else {
		L_Printf("[GetRefreshableVideos]: Failed to get 'live' videos from DB_ListVideos, err: %v\n", err)
	}
	
	return RefreshableVideos
}

func CheckChannel(AChannel *ArchiveChannel, CheckSettings ChannelCheckSettings) {
	AChannel.Lock.Lock()
	if AChannel.Url == "" {
		AChannel.Lock.Unlock()
		return
	}
	if AChannel.Id != MANUAL_CHANNEL_ID {
		if AChannel.IsBeingChecked {
			AChannel.Lock.Unlock()
			return
		}
		
		AChannel.IsBeingChecked = true
		defer func() {AChannel.IsBeingChecked = false}()
	}
	
	Url := AChannel.Url
	if CheckSettings.CheckUrl != "" {
		Url = CheckSettings.CheckUrl
	}
	ChannelId := AChannel.Id
	
	TimeNow := time.Now().UTC().UnixMilli()
	
	AChannel.NextCheckMSEC = TimeNow + (AChannel.CheckInterval*1000)
	
	Task := CL_NewGenericTask()
	defer func() {
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	Task.Type = TASK_TYPE_LISTING
	Task.FromChannelId = ChannelId
	if ChannelId != MANUAL_CHANNEL_ID {
		Task.Title = fmt.Sprintf("Checking channel: \"%s\"", AChannel.Name)
	} else {
		Task.Title = fmt.Sprintf("Checking: \"%s\"", Url)
	}
	DB_UpdateCommandTaskInfo(Task)
	
	PlaylistEnd := AChannel.PlaylistEnd
	if PlaylistEnd <= -1 {
		// Can check all videos!
		PlaylistEnd = 50
	}
	if CheckSettings.CheckAllVideos {
		PlaylistEnd = -1
		//L_Printf("Checking every video for \"%s\" ! \n", AChannel.Name)
		CL_Logf(Task, "Checking every video for \"%s\" ! \n", AChannel.Name)
	}
	
	ChannelType := AChannel.Type
	if CheckSettings.OverrideChannelType >= 0 {
		ChannelType = int32(CheckSettings.OverrideChannelType)
	}
	
	AChannel.Lock.Unlock()
	
	VideoList, err := yt_dlp_ListVideos(Url, PlaylistEnd, Task)
	if err != nil {
		CL_FinishTask(Task, TASK_STATUS_FAILED)
		return
	}
	DB_UpdateCommandTaskInfo(Task)
	
	CL_Logf(Task, "Found %d videos\n", len(VideoList))
	
	// Add the videos to the queued list.
	for i := len(VideoList)-1; i >= 0; i-- {
		if Task.Status != TASK_STATUS_RUNNING { return }
		
		Video := VideoList[i]
		Video.FromChannel = ChannelId
		ExistingVideo, err := DB_GetVideo(Video.Id)
		if ExistingVideo == nil && err == nil {
			CL_Logf(Task, "Adding new video \"%s\" %s ! \n", Video.Id, Video.Url)
			DB_UpdateVideoInfo(&Video)
			
			if ChannelType == ACHANNEL_TYPE_LIST_AND_IGNORE {
				DB_UpdateVideoQueuedAction(&Video, VIDEO_QACTION_WILL_IGNORE)
			} else if ChannelType == ACHANNEL_TYPE_VIDEOS || ChannelType == ACHANNEL_TYPE_LIVE {
				DB_UpdateVideoQueuedAction(&Video, VIDEO_QACTION_WILL_DOWNLOAD)
			}
		}
		
		time.Sleep(time.Millisecond * 1)
		
		DB_UpdateCommandTaskInfo(Task)
	}
	
	VideosCheckedCount := 0
	
	for _, Video := range(VideoList) {
		if Task.Status != TASK_STATUS_RUNNING { return }
		if !IsChannelEnabledWithCheck(AChannel, CheckSettings) { break }
		
		Video.FromChannel = ChannelId
		if CheckVideoAndDownload(AChannel, &Video, Task, CheckSettings) {
			VideosCheckedCount += 1
		}
		DB_UpdateCommandTaskInfo(Task)
	}
	
	// Get queued videos in the database.
	QueuedVideosList, err := DB_ListVideos(-1, 0, ListVideosQuery{
		RefreshState: -1,
		QueuedAction: -1,
		VideoType: -1,
		Status: 0,
		FromChannelId: ChannelId,
	})
	if err != nil {
		L_Printf("DB_ListVideos err: %v\n", err)
		CL_Logf(Task, "DB_ListVideos error grabbing queued videos: %v \n", err)
	}
	if len(QueuedVideosList) > 0 {
		for _, Video := range(QueuedVideosList) {
			if Task.Status != TASK_STATUS_RUNNING { return }
			if !IsChannelEnabledWithCheck(AChannel, CheckSettings) { break }
			
			Video.FromChannel = ChannelId
			if CheckVideoAndDownload(AChannel, Video, Task, CheckSettings) {
				VideosCheckedCount += 1
			}
			DB_UpdateCommandTaskInfo(Task)
		}
	}
	
	RefreshableVideos := GetRefreshableVideos(AChannel)
	if len(RefreshableVideos) > 0 {
		for _, Video := range(RefreshableVideos) {
			if Task.Status != TASK_STATUS_RUNNING { return }
			if !IsChannelEnabledWithCheck(AChannel, CheckSettings) { break }
			
			CL_Logf(Task, "Refreshing video info: \"%s\" %s\n", Video.Title, Video.Url)
			RefreshVideoInfo(AChannel, Video, Task)
			DB_UpdateCommandTaskInfo(Task)
		}
	}
	
	AChannel.Lock.Lock()
	AChannel.NextCheckMSEC = time.Now().UTC().UnixMilli() + (AChannel.CheckInterval*1000)
	AChannel.Lock.Unlock()
	
	CL_FinishTask(Task, TASK_STATUS_FINISHED)
}

func CheckChannelRefreshes(AChannel *ArchiveChannel) {
	if AChannel.IsBeingChecked { return }
	AChannel.NeedsRefreshing = false
	
	AChannel.IsBeingChecked = true
	defer func() {AChannel.IsBeingChecked = false}()
	
	Task := CL_NewGenericTask()
	defer func() {
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	Task.Type = TASK_TYPE_LISTING
	Task.FromChannelId = AChannel.Id
	Task.Title = fmt.Sprintf("Refreshing videos for: \"%s\"", AChannel.Name)
	DB_UpdateCommandTaskInfo(Task)
	
	RefreshableVideos := GetRefreshableVideos(AChannel)
	if len(RefreshableVideos) > 0 {
		for _, Video := range(RefreshableVideos) {
			if Task.Status != TASK_STATUS_RUNNING { return }
			
			CL_Logf(Task, "Refreshing video info: \"%s\" %s\n", Video.Title, Video.Url)
			DB_UpdateCommandTaskInfo(Task)
			RefreshVideoInfo(AChannel, Video, Task)
		}
		
		if len(RefreshableVideos) >= MaxVideosToRefresh {
			AChannel.NeedsRefreshing = true
		}
	}
	
	DB_UpdateCommandTaskInfo(Task)
	
	CL_FinishTask(Task, TASK_STATUS_FINISHED)
}

func CheckChannels(WD *WatchingBundle) {
	TimeNow := time.Now().UTC().UnixMilli()
	
	for _, AChannel := range(WD.Channels) {
		if AChannel.NeedsRefreshing {
			go CheckChannelRefreshes(AChannel)
		}
		if !IsChannelEnabled(AChannel) {
			continue
		}
		if time.Now().UTC().UnixMilli() < AChannel.NextCheckMSEC || AChannel.CheckInterval <= 0 {
			continue
		}
		if AChannel.Id == MANUAL_CHANNEL_ID {
			// Don't auto download stuff from the manual channel.
			continue
		}
		
		CheckAll := false
		
		if AChannel.PlaylistEnd <= -1 && TimeNow > AChannel.NextFullChannelCheckMSEC {
			if AChannel.FullCheckInterval <= 0 {
				AChannel.FullCheckInterval = 86400
			}
			AChannel.NextFullChannelCheckMSEC = TimeNow + (AChannel.FullCheckInterval * 1000)
			CheckAll = true
		}
		
		go CheckChannel(AChannel, ChannelCheckSettings{
			QualitySelect: -1,
			OverrideChannelType: -1,
			ForceEnable: false,
			
			CheckAllVideos: CheckAll,
		})
	}
}

var IsOldRefreshing = false
func AutoRefreshOldUpdatedVideos() {
	if IsOldRefreshing {
		return
	}
	IsOldRefreshing = true
	defer func() {
		IsOldRefreshing = false
	}()
	
	AutoRefresh_Videos_Seconds := G_Config.AutoRefresh_Videos_Seconds
	
	Task := CL_NewGenericTask()
	defer func() {
		if Task.Status == TASK_STATUS_RUNNING {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	Task.Type = TASK_TYPE_LISTING
	Task.Title = fmt.Sprintf("Auto refreshing videos older than %d seconds", AutoRefresh_Videos_Seconds)
	DB_UpdateCommandTaskInfo(Task)
	
	RefreshTimeUnix := time.Now().UTC().Add(time.Second * -time.Duration(AutoRefresh_Videos_Seconds)).Unix()
	
	VideosRefreshed := 0
	THIS_MaxVideosToRefresh := 200
	PageSize := 100
	Page := 0
	PageOffset := -0
	for {
		// Get videos oldest to newest!
		RefreshableVideos, err := DB_ListVideos(PageSize, (Page*PageSize)+PageOffset, ListVideosQuery{
			Status:       -1,
			VideoType:    -1,
			RefreshState: 0,
			QueuedAction: -1,
			
			OrderBy: DB_VIDEO_ORDERBY_UpdatedAt,
			OrderDirection: 1,
		})
		if err != nil {
			L_Printf("Failed to RefreshOldUpdatedVideos, DB_ListVideos error: %v\n", err)
			return
		}
		for _, Video := range(RefreshableVideos) {
			if RefreshTimeUnix < Video.UpdatedAt.Unix() { continue }  // Check if video is old enough to be refreshed.
			
			if Video.Availability == "removed" {
				// This video is removed. Don't auto refresh it 
			}
			
			if Task.Status != TASK_STATUS_RUNNING { return }
			if G_Config.AutoRefresh_Videos_Seconds <= 0 { return }
			
			AChannel := GetArchiveChannelFromId(&WatchedDownloading, Video.FromChannel)
			if AChannel == nil {
				continue
			}
			VideosRefreshed += 1
			PageOffset -= 1  // Updating a video reorders the video list in the database. If we don't move backwards we can miss some videos!
			
			CL_Logf(Task, "Refreshing video info: \"%s\" %s\n", Video.Title, Video.Url)
			DB_UpdateCommandTaskInfo(Task)
			RefreshVideoInfo(AChannel, Video, Task)
			if VideosRefreshed >= THIS_MaxVideosToRefresh {
				break
			}
		}
		
		if VideosRefreshed >= THIS_MaxVideosToRefresh {
			break
		}
		if len(RefreshableVideos) < PageSize {
			// We have reached the end.
			break
		}
		
		Page += 1
	}
	
	if VideosRefreshed == 0 {
		CL_Logf(Task, "Found no videos to refresh!")
		// TODO: Delete this task as it is useless information...
	} else {
		CL_Logf(Task, "\nRefreshed %d videos.\n", VideosRefreshed)
	}
	if Task.Status == TASK_STATUS_RUNNING {
		CL_FinishTask(Task, TASK_STATUS_FINISHED)
	}
}

func InitDownloading() {
	err := DB_LoadChannels(&WatchedDownloading)
	if err != nil {
		panic(err)
	}
	
	GetManualArchiveChannel(&WatchedDownloading)
	
	NextTasksDatabaseCleanUp := time.Now().UTC().Unix()+10
	NextVideosRefreshUpdate  := time.Now().UTC().Unix()+10
	
	for true {
		time.Sleep(1 * time.Second)
		if time.Now().UTC().Unix() > NextTasksDatabaseCleanUp {
			NextCleanUp := (60*10)   // Clean up old tasks every 10 minutes.
			if NextCleanUp > G_Config.TaskLog_AutoDelete_Seconds {
				NextCleanUp = G_Config.TaskLog_AutoDelete_Seconds
			}
			if NextCleanUp > G_Config.TaskLog_List_AutoDelete_Seconds {
				NextCleanUp = G_Config.TaskLog_List_AutoDelete_Seconds
			}
			NextTasksDatabaseCleanUp = time.Now().UTC().Unix() + int64(NextCleanUp)
			CleanUpTasksInDatabase()
		}
		if G_Config.AutoRefresh_Videos_Seconds > 0 && time.Now().UTC().Unix() > NextVideosRefreshUpdate {
			NextVideosRefreshUpdate = time.Now().UTC().Unix() + (60*30)
			go AutoRefreshOldUpdatedVideos()
		}
		
		CheckChannels(&WatchedDownloading)
	}
}

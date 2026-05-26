package main

import (
	"bytes"
	"crypto/sha256"
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

func RefreshVideoInfo(AChannel *ArchiveChannel, Video *VideoInfo, Task *CommandTask) {
	UpdateVideoFileSize(Video, AChannel)
	
	err := RequestVideoInfo(AChannel, Video.Url, -1, Video, Task)
	DB_UpdateVideoAvalibility(Video, Video.Availability)   // RequestVideoInfo() might have updated the Availability tag.
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
	if fpErr == nil && FilePath != "" {
		FileSize, err := GetFileSize(FilePath)
		if err == nil {
			DB_UpdateVideoFileSize(Video, FileSize)
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
	
	Response, err := http.Get(ThumbnailUrl)
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
	
	ImageContent, err := io.ReadAll(Response.Body)
	if err != nil {
		return "", fmt.Errorf("Could not read response body because: %v", err)
	}
	if len(ImageContent) > DB_IMAGE_MAX_FILESIZE {
		// Image file is too big!!!
		return "", fmt.Errorf("The downloaded thumbnail is larger than 10MB...")
	}
	
	ImageHash := fmt.Sprintf("%x", sha256.Sum256(ImageContent))
	
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
	
	err := RequestVideoInfo(AChannel, Video.Url, QualitySelect, Video, Task)
	if (Task != nil && Task.Status != TASK_STATUS_RUNNING) {
		// This task was canceled!! Don't do anything else.
		return false
	}
	if err != nil {
		DB_UpdateVideoStatus(Video, VIDEO_STATUS_FAILED)
		DB_UpdateVideoAvalibility(Video, Video.Availability)
		CL_Logf(Task, "Failed to grab video info for \"%s\"... Error: %v\n", Video.Title, err)
		return false
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
	
	QueuedVideosList, err := DB_ListVideos(-1, 0, ListVideosQuery{
		RefreshState: -1,
		QueuedAction: -1,
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
	
	MaxVideosToRefresh := 20
	
	RefreshableVideos, err := DB_ListVideos(MaxVideosToRefresh, 0, ListVideosQuery{
		RefreshState: 1,
		FromChannelId: ChannelId,
		Status: -1,
		QueuedAction: -1,
	})
	if err != nil {
		CL_Logf(Task, "Failed to grab refreshable videos from DB_ListVideos, error: %v\n", err)
	}
	if err == nil && len(RefreshableVideos) > 0 {
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
	
	MaxVideosToRefresh := 20
	
	RefreshableVideos, err := DB_ListVideos(MaxVideosToRefresh, 0, ListVideosQuery{
		RefreshState: 1,
		FromChannelId: AChannel.Id,
		Status: -1,
		QueuedAction: -1,
	})
	if err != nil {
		CL_Logf(Task, "Failed to grab refreshable videos from DB_ListVideos, error: %v\n", err)
	}
	if err == nil && len(RefreshableVideos) > 0 {
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

func InitDownloading() {
	err := DB_LoadChannels(&WatchedDownloading)
	if err != nil {
		panic(err)
	}
	
	GetManualArchiveChannel(&WatchedDownloading)
	
	NextTasksDatabaseCleanUp := time.Now().UTC().Unix()+10
	
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
		
		CheckChannels(&WatchedDownloading)
	}
}

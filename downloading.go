package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	//"os/exec"
	//"fmt"
)

const (
	ACHANNEL_TYPE_VIDEOS = 0
	ACHANNEL_TYPE_LIVE   = 1
	ACHANNEL_TYPE_LIST_NO_DOWNLOAD = 2
	ACHANNEL_TYPE_LIST_AND_IGNORE  = 3
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
	
	Enabled        bool `json:"enabled"`
	IsBeingChecked bool `json:"-"`
	
	NextCheckMSEC            int64 `json:"_nextCheckMsec"`
	NextFullChannelCheckMSEC int64 `json:"_nextFullChannelCheckMsec"`
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
		fmt.Printf("!!! COULD NOT ADD CHANNEL: \"%s\" TO DATABASE ERR: %v !!!\n", AChannel.Url, err)
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
		fmt.Printf("Could not remove channel from database err: %v\n", err)
		return err
	}
	WD.ChannelsLock.Lock()
	NewChannels := make([]*ArchiveChannel, 0, len(WD.Channels))
	
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id { continue }
		
		AChannel.Enabled = false
		
		NewChannels = append(NewChannels, AChannel)
		DB_RemoveChannel(Id)
	}
	
	WD.Channels = NewChannels
	WD.ChannelsLock.Unlock()
	
	return nil
}

func GetArchiveChannelFromId(WD *WatchingBundle, Id string) *ArchiveChannel {
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id {
			return AChannel
		}
	}
	
	return  nil
}

func CheckIsVideoDownloaded(v VideoInfo) bool {
	DB_VideoInfo, err := DB_GetVideo(v.Id)
	if err != nil {
		fmt.Printf("CheckIsVideoDownloaded err: %v\n", err)
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

func RefreshVideoInfo(v *VideoInfo) {
	err := RequestVideoInfo(v.Url, v)
	if err != nil {
		fmt.Printf("Failed to grab video info... err: %v\n", err)
		DB_UpdateVideoAvalibility(v, "UNKNOWN")
		DB_UpdateVideoRefreshState(v, 0)
		return
	}
	DB_UpdateVideoInfo(v)
	DB_UpdateVideoRefreshState(v, 0)
}

func DownloadVideo(AChannel *ArchiveChannel, v *VideoInfo) {
	if AChannel.Type == ACHANNEL_TYPE_LIST_NO_DOWNLOAD || AChannel.Type == ACHANNEL_TYPE_LIST_AND_IGNORE {
		fmt.Printf("DownloadVideo tried to download from a channel that doesn't allow downloads? Name: \"%s\"\n", AChannel.Name)
		return
	}
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADING)
	err := yt_dlp_DownloadVideo(*AChannel, v)
	if err != nil {
		DB_UpdateVideoStatus(v, VIDEO_STATUS_FAILED)
		return
	}
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADED)
}
func DownloadYTLive(AChannel *ArchiveChannel, v *VideoInfo) {
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADING)
	err := ytarchive_DownloadLive(*AChannel, v)
	if err != nil {
		DB_UpdateVideoStatus(v, VIDEO_STATUS_FAILED)
		return
	}
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADED)
	RefreshVideoInfo(v)
}

func CheckVideoAndDownload(AChannel *ArchiveChannel, v *VideoInfo) {
	if CheckIsVideoDownloaded(*v) {
		return
	}
	
	if AChannel.Type != ACHANNEL_TYPE_LIST_NO_DOWNLOAD && AChannel.Type != ACHANNEL_TYPE_LIST_AND_IGNORE {
		DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADING)
	}
	
	err := RequestVideoInfo(v.Url, v)
	if err != nil {
		fmt.Printf("Failed to grab video info... err: %v\n", err)
		DB_UpdateVideoStatus(v, VIDEO_STATUS_FAILED)
		return
	}
	
	DB_UpdateVideoInfo(v)
	
	if AChannel.Type == ACHANNEL_TYPE_LIST_NO_DOWNLOAD || AChannel.Type == ACHANNEL_TYPE_LIST_AND_IGNORE {
		if AChannel.Type == ACHANNEL_TYPE_LIST_AND_IGNORE {
			DB_UpdateVideoStatus(v, VIDEO_STATUS_IGNORED)
		}
		
		return
	}
	
	switch v.VideoType {
	case VIDEO_TYPE_ISLIVE:
		// TODO:
		if v.Url[0:23] == "https://www.youtube.com" {
			// use ytarchive
			go DownloadYTLive(AChannel, v)
			break
		}
		fallthrough
	case VIDEO_TYPE_WASLIVE: fallthrough
	case VIDEO_TYPE_VIDEO:   fallthrough
	default:
		DownloadVideo(AChannel, v)
	}
}

func CheckChannel(AChannel *ArchiveChannel) {
	if AChannel.Url == "" { return }
	if AChannel.IsBeingChecked { return }
	
	AChannel.IsBeingChecked = true
	defer func() {AChannel.IsBeingChecked = false}()
	
	TimeNow := time.Now().UnixMilli()
	
	AChannel.NextCheckMSEC = TimeNow + (AChannel.CheckInterval*1000)
	
	AChannel.Lock.RLock()
	Url := AChannel.Url
	AChannel.Lock.RUnlock()
	
	PlaylistEnd := 6
	if AChannel.FullCheckInterval > 0 && TimeNow > AChannel.NextFullChannelCheckMSEC {
		PlaylistEnd = -1
		AChannel.NextFullChannelCheckMSEC = TimeNow + (AChannel.FullCheckInterval * 1000)
		fmt.Printf("Checking every video for \"%s\" ! \n", AChannel.Name)
	}
	
	VideoList, err := yt_dlp_ListVideos(Url, PlaylistEnd)
	if err != nil {
		fmt.Printf("Error when grabbing videos: %v\n", err)
		return
	}
	
	// Add the videos to the queued list.
	for i := len(VideoList)-1; i >= 0; i-- {
		v := VideoList[i]
		v.FromChannel = AChannel.Id
		Exists, err := DB_GetVideo(v.Id)
		if Exists == nil && err == nil {
			DB_UpdateVideoInfo(&v)
		}
	}
	
	for _, v := range(VideoList) {
		if !AChannel.Enabled { break }
		v.FromChannel = AChannel.Id
		CheckVideoAndDownload(AChannel, &v)
	}
	
	QueuedVideosList, err := DB_ListVideos(-1, 0, ListVideosQuery{
		RefreshState: -1,
		Status: 0,
		FromChannelId: AChannel.Id,
	})
	if err != nil {
		fmt.Printf("DB_ListVideos err: %v\n", err)
	}
	if len(QueuedVideosList) > 0 {
		for _, v := range(QueuedVideosList) {
			if !AChannel.Enabled { break }
			v.FromChannel = AChannel.Id
			CheckVideoAndDownload(AChannel, v)
		}
	}
	
	RefreshableVideos, err := DB_ListVideos(10, 0, ListVideosQuery{
		RefreshState: 1,
		FromChannelId: AChannel.Id,
		Status: -1,
	})
	if err != nil {
		fmt.Printf("RefreshableVideos DB_ListVideos err: %v\n", err)
	}
	if err == nil && len(RefreshableVideos) > 0 {
		for _, v := range(RefreshableVideos) {
			if !AChannel.Enabled { break }
			RefreshVideoInfo(v)
		}
	}
	
	AChannel.NextCheckMSEC = time.Now().UnixMilli() + (AChannel.CheckInterval*1000)
}

func CheckChannels(WD *WatchingBundle) {
	WD.ChannelsLock.RLock()
	
	for _, AChannel := range(WD.Channels) {
		if !AChannel.Enabled {
			continue
		}
		if time.Now().UnixMilli() < AChannel.NextCheckMSEC {
			continue
		}
		go CheckChannel(AChannel)
	}
	
	WD.ChannelsLock.RUnlock()
}

func StartDownloading() {
	err := DB_LoadChannels(&WatchedDownloading)
	if err != nil {
		panic(err)
	}
	
	for true {
		time.Sleep(1 * time.Second)
		
		CheckChannels(&WatchedDownloading)
	}
}

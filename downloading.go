package main

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	//"os/exec"
	//"fmt"
)

const (
	ACHANNEL_TYPE_LIVE = iota
	ACHANNEL_TYPE_VIDEOS
)

type ArchiveChannel struct {
	Lock sync.RWMutex
	
	Id   string `json:"id"`
	Name string `json:"name"`
	Url  string `json:"url"`
	DownloadDir    string `json:"download_dir"`
	OutputTemplate string `json:"output_template"`
	QualitySelect  int    `json:"quality_select"`
	Type           int32  `json:"type"`
	Enabled        bool   `json:"enabled"`
	IsBeingChecked bool
	
	NextTimeCheckMSEC int64 `json:"nextTimeCheckMsec"`
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
	
	WD.ChannelsLock.Lock()
	WD.Channels = append(WD.Channels, AChannel)
	WD.ChannelsLock.Unlock()
	
	err := DB_UpdateArchiveChannel(AChannel)
	if err != nil {
		fmt.Printf("!!! COULD NOT ADD CHANNEL: \"%s\" TO DATABASE ERR: %v !!!\n", AChannel.Url, err)
		return err
	}
	
	return nil
}
func RemoveArchiveChannel(WD *WatchingBundle, Id string) {
	WD.ChannelsLock.Lock()
	NewChannels := make([]*ArchiveChannel, 0, len(WD.Channels))
	
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id { continue }
		
		NewChannels = append(NewChannels, AChannel)
		DB_RemoveChannel(Id)
	}
	
	WD.Channels = NewChannels
	WD.ChannelsLock.Unlock()
}

func CheckIsVideoDownloaded(v VideoInfo) bool {
	DB_VideoInfo, err := DB_GetVideo(v.Id)
	if err == sql.ErrNoRows {
		return false
	}
	if DB_VideoInfo != nil {
		if DB_VideoInfo.Status == VIDEO_STATUS_DOWNLOADED || DB_VideoInfo.Status == VIDEO_STATUS_DOWNLOADING {
			return true
		}
		return false
	}
	
	// Maybe...
	return false
}

func DownloadVideo(AChannel *ArchiveChannel, v *VideoInfo) {
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADING)
	err := yt_dlp_DownloadVideo(*AChannel, v)
	if err != nil {
		DB_UpdateVideoStatus(v, VIDEO_STATUS_FAILED)
	}
	DB_UpdateVideoStatus(v, VIDEO_STATUS_DOWNLOADED)
}

func CheckVideoAndDownload(AChannel *ArchiveChannel, v *VideoInfo) {
	if CheckIsVideoDownloaded(*v) {
		fmt.Printf("Video already exists: %s\n", v.Url)
		return
	}
	
	err := RequestVideoInfo(v.Url, v)
	if err != nil {
		fmt.Printf("Failed to grab video info... err: %v\n", err)
		return
	}
	
	DB_UpdateVideoInfo(v)
	DB_UpdateVideoStatus(v, VIDEO_STATUS_QUEUED)
	
	switch v.VideoType {
	case VIDEO_TYPE_ISLIVE:
		// TODO:
		if v.Url[0:19] == "https://youtube.com" {
			// use ytarchive
			
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
	if AChannel.IsBeingChecked { return }
	AChannel.IsBeingChecked = true
	defer func() {AChannel.IsBeingChecked = false}()
	
	AChannel.Lock.RLock()
	Url := AChannel.Url
	AChannel.Lock.RUnlock()
	
	switch AChannel.Type {
	case ACHANNEL_TYPE_LIVE:
		VideoList, err := ListVideos(Url, 10)
		if err != nil {
			fmt.Printf("Error when grabbing videos: %v\n", err)
			return
		}
		
		// Add the videos to the queued list.
		for _, v := range(VideoList) {
			DB_UpdateVideoInfo(&v)
			//DB_UpdateVideoStatus(&v, VIDEO_STATUS_QUEUED)
		}
		
		for _, v := range(VideoList) {
			v.FromChannel = AChannel.Id
			CheckVideoAndDownload(AChannel, &v)
		}
	case ACHANNEL_TYPE_VIDEOS: fallthrough
	default:
		
		break
	}
	
	
}

func CheckChannels(WD *WatchingBundle) {
	WD.ChannelsLock.RLock()
	
	for _, AChannel := range(WD.Channels) {
		go CheckChannel(AChannel)
	}
	
	WD.ChannelsLock.RUnlock()
}

func StartDownloading() {
	for true {
		time.Sleep(1 * time.Second)
	}
}

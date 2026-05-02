package main

import (
	"fmt"
	"sync"
	"time"
	"yt-stream-manager/yt_dlp"

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
	
	Id   string        `json:"id"`
	Name string        `json:"name"`
	Url  string        `json:"url"`
	DownloadDir string `json:"download_dir"`
	Type int32         `json:"type"`
	Enabled bool       `json:"enabled"`
	IsBeingChecked bool
	
	NextTimeCheckMSEC int64 `json:"nextTimeCheckMsec"`
}

type WatchingBundle struct {
	ChannelsLock sync.RWMutex
	Channels []*ArchiveChannel
}

var WatchedDownloading WatchingBundle

func AddArchiveChannel(WD *WatchingBundle, AChannel *ArchiveChannel) {
	if AChannel.Id == "" {
		AChannel.Id = uuid.New().String()
	}
	
	WD.ChannelsLock.Lock()
	WD.Channels = append(WD.Channels, AChannel)
	WD.ChannelsLock.Unlock()
	
	// TODO: database stuff...
}
func RemoveArchiveChannel(WD *WatchingBundle, Id string) {
	WD.ChannelsLock.Lock()
	NewChannels := make([]*ArchiveChannel, 0, len(WD.Channels))
	
	for _, AChannel := range(WD.Channels) {
		if AChannel.Id == Id { continue }
		NewChannels = append(NewChannels, AChannel)
	}
	
	WD.Channels = NewChannels
	WD.ChannelsLock.Unlock()
}

func CheckVideo(AChannel *ArchiveChannel, v yt_dlp.VideoInfo) {
	
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
		VideoList, err := yt_dlp.ListVideos(Url, 10)
		if err != nil {
			fmt.Printf("Error when grabbing videos: %v\n", err)
			return
		}
		
		for _, v := range(VideoList) {
			CheckVideo(AChannel, v)
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

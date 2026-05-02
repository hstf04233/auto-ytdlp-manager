package yt_dlp

import (
	"fmt"
	"bufio"
	"os/exec"
	"strings"
	"encoding/json"
)

const CMD_YT_DLP = "yt-dlp"

const (
	VIDEO_TYPE_UNKNOWN = iota
	VIDEO_TYPE_VIDEO
	VIDEO_TYPE_ISLIVE
	VIDEO_TYPE_WASLIVE
)

type VideoInfo struct {
	Title string
	Url   string
	Id    string
	ReleaseDate int64
	Duration float64
	
	VideoType int32
}

type YT_DLP_OUTVIDEO struct {
	Title        string  `json:"title"`
	Duration     float64 `json:"duration"`
	Url          string  `json:"webpage_url"`
	Id           string  `json:"id"`
	Availability string  `json:"availability"`
	Timestamp    int64   `json:"timestamp"`
	ReleaseTimestamp int64 `json:"release_timestamp"`
	
	IsLive  bool `json:"is_live"`
	WasLive bool `json:"was_live"`
}

func RequestVideoInfo(VideoUrl string, VI *VideoInfo) (*VideoInfo, error) {
	// TODO
	return nil, nil
}

func ListVideos(ChannelUrl string, PlaylistEnd int64) ([]VideoInfo, error) {
	fmt.Printf("ChannelUrl: %s\n", ChannelUrl)
	Args := []string{
		ChannelUrl,
		"--ignore-config",
		"--flat-playlist",
		"--dump-json",
		"--skip-download",
		"--extractor-args", "youtubetab:approximate_date",
	}
	if PlaylistEnd > 0 {
		Args = append(Args, "--playlist-end", fmt.Sprintf("%d", PlaylistEnd))
	}
	
	fmt.Printf("%v\n", Args)
	
	Cmd := exec.Command(CMD_YT_DLP, Args...)
	
	Out, err := Cmd.Output()
	if err != nil {
		fmt.Printf("Failed to list videos from channel: %s, Error: %v\n", ChannelUrl, err)
		return nil, err
	}
	
	var OutVideos []VideoInfo
	
	scanner := bufio.NewScanner(strings.NewReader(string(Out)))
	for scanner.Scan() {
		OutVideo := YT_DLP_OUTVIDEO{}
		err = json.Unmarshal([]byte(scanner.Text()), &OutVideo)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return nil, err
		}
		
		VideoInfo := VideoInfo{
			Title:    OutVideo.Title,
			Url:      OutVideo.Url,
			Id:       OutVideo.Id,
			Duration: OutVideo.Duration,
		}
		if OutVideo.IsLive {
			VideoInfo.VideoType = VIDEO_TYPE_ISLIVE
		} else if OutVideo.WasLive {
			VideoInfo.VideoType = VIDEO_TYPE_WASLIVE
		} else {
			VideoInfo.VideoType = VIDEO_TYPE_VIDEO
		}
		
		if OutVideo.ReleaseTimestamp != 0 {
			VideoInfo.ReleaseDate = OutVideo.ReleaseTimestamp
		} else if OutVideo.Timestamp != 0 {
			VideoInfo.ReleaseDate = OutVideo.Timestamp
		}
		OutVideos = append(OutVideos, VideoInfo)
	}
	
	return OutVideos, nil
}


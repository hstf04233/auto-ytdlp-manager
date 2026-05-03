package main

import (
	"fmt"
	"bufio"
	"os"
	"os/exec"
	"strings"
	"encoding/json"
)

const (
	VIDEO_TYPE_UNKNOWN = iota
	VIDEO_TYPE_VIDEO
	VIDEO_TYPE_ISLIVE
	VIDEO_TYPE_WASLIVE
)

type VideoInfo struct {
	FromChannel string
	Title string
	Url   string
	Id    string
	ReleaseDate int64
	Duration float64
	Status int
	
	VideoType int32
}

type YT_DLP_OUTVIDEO struct {
	Title        string    `json:"title"`
	Duration     float64   `json:"duration"`
	Url          string    `json:"webpage_url"`
	Id           string    `json:"id"`
	Availability string    `json:"availability"`
	Timestamp    int64     `json:"timestamp"`
	ReleaseTimestamp int64 `json:"release_timestamp"`
	
	IsLive  bool `json:"is_live"`
	WasLive bool `json:"was_live"`
}

func PopulateVideoInfoFromOutVideo(VideoInfo *VideoInfo, OutVideo YT_DLP_OUTVIDEO) {
	VideoInfo.Title    = OutVideo.Title
	VideoInfo.Url      = OutVideo.Url
	VideoInfo.Id       = OutVideo.Id
	VideoInfo.Duration = OutVideo.Duration
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
}

func RequestVideoInfo(VideoUrl string, v *VideoInfo) (error) {
	Cmd := exec.Command(
		CMD_YT_DLP,
		"--ignore-config",
		"--dump-json",
		"--skip-download",
	)
	Out, err := Cmd.Output()
	if err != nil {
		fmt.Printf("Failed to get video info from url: %s, Error: %v\n", VideoUrl, err)
		return err
	}
	var OutVideo YT_DLP_OUTVIDEO
	err = json.Unmarshal(Out, &OutVideo)
	if err != nil {
		fmt.Printf("json.Unmarshal err: %v\n", err)
		return err
	}
	
	PopulateVideoInfoFromOutVideo(v, OutVideo)
	
	return nil
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
		
		var VideoInfo VideoInfo
		
		PopulateVideoInfoFromOutVideo(&VideoInfo, OutVideo)
		OutVideos = append(OutVideos, VideoInfo)
	}
	
	return OutVideos, nil
}


func yt_dlp_DownloadVideo(AChannel ArchiveChannel, v *VideoInfo) (error) {
	DownloadDir := AChannel.DownloadDir
	if DownloadDir == "" {
		DownloadDir = DEFAULT_DOWNLOAD_DIR
	}
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		fmt.Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	Cmd := exec.Command(
		CMD_YT_DLP,
		v.Url,
		"--ignore-config",
		"-s", fmt.Sprintf("res:%d", AChannel.QualitySelect),
	)
	Cmd.Dir = AChannel.DownloadDir
	Out, err := Cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to download video from url: %s, Error: %v\n", v.Url, err)
		return err
	}
	fmt.Printf("Output: %s\n", Out)
	
	return nil
}


package main

import (
	"os"
	"os/exec"
	"time"
	"fmt"
	"bufio"
	"strings"
	"encoding/json"
)

const (
	VIDEO_TYPE_UNKNOWN = 0
	VIDEO_TYPE_VIDEO   = 1
	VIDEO_TYPE_ISLIVE  = 2
	VIDEO_TYPE_WASLIVE = 3
)

type VideoInfo struct {
	FromChannel string  `json:"from_channel"`	// Channel id
	Title       string  `json:"title"`
	Url         string  `json:"url"`
	Id          string  `json:"id"`
	ReleaseDate int64   `json:"release_date"`
	Duration    float64 `json:"duration"`
	Status      int     `json:"status"`
	
	VideoType   int32   `json:"video_type"`
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
	fmt.Printf("%+v\n", OutVideo)
	if OutVideo.IsLive {
		VideoInfo.VideoType = VIDEO_TYPE_ISLIVE
	} else if OutVideo.WasLive {
		VideoInfo.VideoType = VIDEO_TYPE_WASLIVE
	} else {
		VideoInfo.VideoType = VIDEO_TYPE_VIDEO
	}
	fmt.Printf("VideoType: %v\n", VideoInfo.VideoType)
	
	if OutVideo.ReleaseTimestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.ReleaseTimestamp
	} else if OutVideo.Timestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.Timestamp
	}
}

func RequestVideoInfo(VideoUrl string, v *VideoInfo) (error) {
	Cmd := exec.Command(
		CMD_YT_DLP,
		VideoUrl,
		"--ignore-config",
		"--dump-json",
		"--skip-download",
	)
	Out, err := Cmd.Output()
	if err != nil {
		fmt.Printf("Failed to get video info from url: %s, Error: %v\n", VideoUrl, err)
		return err
	}
	if v.Id == "7WiKbK5Qlis" {
		fmt.Printf("%s\n", Out)
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

func ListVideos(ChannelUrl string, PlaylistEnd int) ([]VideoInfo, error) {
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
	
	OutputTemplate := AChannel.OutputTemplate
	if OutputTemplate == "" {
		OutputTemplate = DEFAULT_YT_DLP_OUTPUT_TEMPLATE
	}
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		fmt.Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	if v.VideoType == VIDEO_TYPE_ISLIVE || v.VideoType == VIDEO_TYPE_WASLIVE {
		DateAndTime := time.Unix(v.ReleaseDate, 0).Format("2006-01-02")
		OutputTemplate = fmt.Sprintf("%s %s", DateAndTime, OutputTemplate)
	}
	
	Args := []string{
		v.Url,
		"--ignore-config",
		"-o", OutputTemplate,
	}
	if AChannel.QualitySelect > 0 {
		Args = append(Args, "-S", fmt.Sprintf("res:%d", AChannel.QualitySelect))
	}
	
	Cmd := exec.Command(CMD_YT_DLP, Args...)
	Cmd.Dir = DownloadDir
	Out, err := Cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("%s\n", Out)
		fmt.Printf("Failed to download video from url: %s, Error: %v\n", v.Url, err)
		return err
	}
	//fmt.Printf("Output: %s\n", Out)
	
	return nil
}
func ytarchive_DownloadLive(AChannel ArchiveChannel, v *VideoInfo) (error) {
	DownloadDir := AChannel.DownloadDir
	if DownloadDir == "" {
		DownloadDir = DEFAULT_DOWNLOAD_DIR
	}
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		fmt.Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	//144p, 240p, 360p, 480p, 720p, 720p60, 1080p, 1080p60, 1440p, 1440p60, 2160p, 2160p60, best
	QualitySelect := AChannel.QualitySelect
	QualityString := "144p/best"
	if QualitySelect >= 2160 {
		QualityString = "2160p60/2160p/best"
	} else if QualitySelect >= 1440 {
		QualityString = "1440p60/1440p/best"
	} else if QualitySelect >= 1080 {
		QualityString = "1080p60/1080p/best"
	} else if QualitySelect >= 720 {
		QualityString = "720p60/720p/best"
	} else if QualitySelect >= 480 {
		QualityString = "480p/best"
	} else if QualitySelect >= 360 {
		QualityString = "360p/best"
	} else if QualitySelect >= 240 {
		QualityString = "240p/best"
	} else if QualitySelect <= 0 {
		QualityString = "best"
	}
	
	DateAndTime := time.Unix(v.ReleaseDate, 0).Format("2006-01-02")
	
	Cmd := exec.Command(
		CMD_YT_ARCHIVE,
		"--no-wait",
		"--add-metadata",
		"-o", fmt.Sprintf("%s %%(title)s %%(id)s", DateAndTime),
		
		v.Url,
		QualityString,
	)
	Cmd.Dir = DownloadDir
	Out, err := Cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("%s\n", Out)
		fmt.Printf("Failed to download video from url: %s, Error: %v\n", v.Url, err)
		return err
	}
	fmt.Printf("Output: %s\n", Out)
	
	return nil
}


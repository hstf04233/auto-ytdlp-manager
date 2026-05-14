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
	FromChannel  string  `json:"from_channel"`	// Channel id
	Title        string  `json:"title"`
	Url          string  `json:"url"`
	Id           string  `json:"id"`
	Availability string  `json:"availability"`  // public, unlisted, private etc...
	Resolution   string  `json:"resolution"`
	
	Filename     string  `json:"filename"`		// Where the video is stored on device
	
	ReleaseDate  int64   `json:"release_date"`
	Duration     float64 `json:"duration"`
	Status       int     `json:"status"`
	
	VideoType   int32   `json:"video_type"`
	
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	RefreshState int `json:"refresh_state"`
}

type YT_DLP_OUTVIDEO struct {
	Title        string  `json:"title"`
	FullTitle    string  `json:"fulltitle"`
	Url          string  `json:"webpage_url"`
	Id           string  `json:"id"`
	Availability string  `json:"availability"`
	Resolution   string  `json:"resolution"`
	Filename     string  `json:"filename"`
	
	Duration     float64  `json:"duration"`
	
	Timestamp        int64 `json:"timestamp"`
	ReleaseTimestamp int64 `json:"release_timestamp"`
	
	IsLive  bool `json:"is_live"`
	WasLive bool `json:"was_live"`
}

func PopulateVideoInfoFromOutVideo(VideoInfo *VideoInfo, OutVideo YT_DLP_OUTVIDEO) {
	if OutVideo.FullTitle != "" {
		VideoInfo.Title = OutVideo.FullTitle
	} else {
		VideoInfo.Title = OutVideo.Title
	}
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
	
	if OutVideo.Availability != "" {
		VideoInfo.Availability = OutVideo.Availability
	} else {
		VideoInfo.Availability = "public?"
	}
	
	if OutVideo.Resolution != "" {
		VideoInfo.Resolution = OutVideo.Resolution
	}
	if OutVideo.Filename != "" && VideoInfo.Filename == "" {
		// TODO Filename is unfinished...
		VideoInfo.Filename = OutVideo.Filename
	}
	
	if OutVideo.ReleaseTimestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.ReleaseTimestamp
	} else if OutVideo.Timestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.Timestamp
	}
}

func GetDownloadDir(AChannel *ArchiveChannel) string {
	DownloadDir := AChannel.DownloadDir
	if DownloadDir == "" {
		DownloadDir = DEFAULT_DOWNLOAD_DIR
	}
	return DownloadDir
}
func GetOutputTemplate(AChannel *ArchiveChannel) string {
	OutputTemplate := AChannel.OutputTemplate
	if OutputTemplate == "" {
		OutputTemplate = DEFAULT_YT_DLP_OUTPUT_TEMPLATE
	}
	return OutputTemplate
}

func RequestVideoInfo(AChannel *ArchiveChannel, VideoUrl string, Video *VideoInfo) (error) {
	DownloadDir    := GetDownloadDir(AChannel)
	OutputTemplate := GetOutputTemplate(AChannel)
	
	Args := []string{
		VideoUrl,
		"--ignore-config",
		"--dump-json",
		"--skip-download",
		"--live-from-start",
		//"--restrict-filenames",
		"-o", OutputTemplate,
	}
	if AChannel.QualitySelect > 0 {
		Args = append(Args, "-S", fmt.Sprintf("res:%d", AChannel.QualitySelect))
	}
	
	Cmd := exec.Command(CMD_YT_DLP, Args...)
	Cmd.Dir = DownloadDir
	
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
	
	OldVideoId := OutVideo.Id
	PopulateVideoInfoFromOutVideo(Video, OutVideo)
	
	// Some platforms (like twitch) might give out a completely different video ids...
	if OldVideoId != "" {
		OutVideo.Id = OldVideoId
	}
	
	if Video.VideoType == VIDEO_TYPE_ISLIVE || Video.VideoType == VIDEO_TYPE_WASLIVE {
		DateAndTime := time.Unix(Video.ReleaseDate, 0).Format("2006-01-02")
		Video.Filename = fmt.Sprintf("%s %s", DateAndTime, Video.Filename)
	}
	
	return nil
}

func yt_dlp_ListVideos(ChannelUrl string, PlaylistEnd int, Task *CommandTask) ([]VideoInfo, error) {
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
	if Task != nil {
		CL_Logf(Task, fmt.Sprintf(">%s\n\n", GetRealArgs(Cmd.Args)))
	}
	
	Out, err := Cmd.Output()
	if err != nil {
		ErrorMsg := fmt.Sprintf("Failed to list videos from channel: %s, Error: %v\n", ChannelUrl, err)
		fmt.Print(ErrorMsg)
		if Task != nil {
			CL_Logf(Task, "%s", ErrorMsg)
			DB_UpdateCommandTaskInfo(Task)
		}
		return nil, err
	}
	
	var OutVideos []VideoInfo
	
	scanner := bufio.NewScanner(strings.NewReader(string(Out)))
	for scanner.Scan() {
		OutVideo := YT_DLP_OUTVIDEO{}
		err = json.Unmarshal([]byte(scanner.Text()), &OutVideo)
		if err != nil {
			ErrorMsg := fmt.Sprintf("Error when decoding json: %v\n", err)
			fmt.Print(ErrorMsg)
			if Task != nil {
				CL_Logf(Task, "%s", ErrorMsg)
				CL_FinishTask(Task, TASK_STATUS_FAILED)
				//DB_UpdateCommandTaskInfo(Task)
			}
			return nil, err
		}
		
		var VideoInfo VideoInfo
		
		PopulateVideoInfoFromOutVideo(&VideoInfo, OutVideo)
		OutVideos = append(OutVideos, VideoInfo)
		/*
		if Task != nil {
			CL_Logf(Task, "Found video: \"%s\" %s\n", VideoInfo.Title, VideoInfo.Url)
		}
		*/
	}
	
	if Task != nil {
		DB_UpdateCommandTaskInfo(Task)
	}
	
	return OutVideos, nil
}


func yt_dlp_DownloadVideo(AChannel *ArchiveChannel, Video *VideoInfo) (error) {
	DownloadDir    := GetDownloadDir(AChannel)
	OutputTemplate := GetOutputTemplate(AChannel)
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		fmt.Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	if Video.VideoType == VIDEO_TYPE_ISLIVE || Video.VideoType == VIDEO_TYPE_WASLIVE {
		DateAndTime := time.Unix(Video.ReleaseDate, 0).Format("2006-01-02")
		OutputTemplate = fmt.Sprintf("%s %s", DateAndTime, OutputTemplate)
	}
	
	Args := []string{
		Video.Url,
		"--ignore-config",
		"--external-downloader-args", "ffmpeg: -loglevel warning -stats",
		"-o", OutputTemplate,
	}
	if AChannel.QualitySelect > 0 {
		Args = append(Args, "-S", fmt.Sprintf("res:%d", AChannel.QualitySelect))
	}
	
	Cmd := exec.Command(CMD_YT_DLP, Args...)
	Cmd.Dir = DownloadDir
	CL_RunDownloadTask(Cmd, Video, AChannel.Id)
	
	err = Cmd.Start()
	if err != nil {
		fmt.Printf("Failed to start download video from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	err = Cmd.Wait()
	if err != nil {
		fmt.Printf("Failed to download video from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	//fmt.Printf("Output: %s\n", Out)
	
	return nil
}
func ytarchive_DownloadLive(AChannel *ArchiveChannel, Video *VideoInfo) (error) {
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
	
	DateAndTime := time.Unix(Video.ReleaseDate, 0).Format("2006-01-02")
	
	Cmd := exec.Command(
		CMD_YT_ARCHIVE,
		"--no-wait",
		"--add-metadata",
		"--save-state",
		"--threads", "2",
		"-o", fmt.Sprintf("%s %%(title)s %%(id)s", DateAndTime),
		
		Video.Url,
		QualityString,
	)
	Cmd.Dir = DownloadDir
	CL_RunDownloadTask(Cmd, Video, AChannel.Id)
	
	err = Cmd.Start()
	if err != nil {
		fmt.Printf("Failed to start live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	err = Cmd.Wait()
	if err != nil {
		fmt.Printf("Failed to live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	//fmt.Printf("Output: %s\n", Out)
	
	return nil
}


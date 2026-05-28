package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	VIDEO_TYPE_UNKNOWN = 0
	VIDEO_TYPE_VIDEO   = 1
	VIDEO_TYPE_ISLIVE  = 2
	VIDEO_TYPE_WASLIVE = 3
)

type VideoInfo struct {
	FromChannel  string `json:"from_channel"`	// Channel id
	Title        string `json:"title"`
	Description  string `json:"description"`
	Url          string `json:"url"`
	Id           string `json:"id"`
	Availability string `json:"availability"`  // public, unlisted, private etc...
	Resolution   string `json:"resolution"`
	OriginThumbnail string `json:"origin_thumbnail_url"`
	Thumbnail       string `json:"stored_thumbnail"`    // Downloaded thumbnail image
	
	UploaderUrl  string `json:"uploader_url"`
	UploaderName string `json:"uploader"`
	
	Filename           string `json:"-"`
	DownloadedFilename string `json:"filename"` // Where the video is stored on device (This is only the file name, not the file path...)
	VideoFileExists    bool  `json:"videofile_exists"`
	FileSize           int64 `json:"filesize"` // Size (in bytes) of video file on device.
	
	ReleaseDate  int64   `json:"release_date"`
	Duration     float64 `json:"duration"`
	Status       int     `json:"status"`
	QueuedAction int     `json:"queued_action"`	// Currently unused...
	
	TasksCount   int `json:"tasks_count"`
	ActiveTaskId string `json:"active_task"`
	
	VideoType int32 `json:"video_type"`
	
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	RefreshState int `json:"refresh_state"`
}

type YT_DLP_OUTVIDEO struct {
	Title        string `json:"title"`
	FullTitle    string `json:"fulltitle"`
	Description  string `json:"description"`
	Url          string `json:"webpage_url"`
	Id           string `json:"id"`
	Availability string `json:"availability"`
	Resolution   string `json:"resolution"`
	Thumbnail    string `json:"thumbnail"`
	Filename     string `json:"filename"`
	
	Extractor string `json:"extractor"`
	
	ChannelName  string `json:"channel"`
	ChannelUrl   string `json:"channel_url"`
	UploaderUrl  string `json:"uploader_url"`
	UploaderName string `json:"uploader"`
	UploaderId   string `json:"uploader_id"`
	
	Duration float64 `json:"duration"`
	
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
	if OutVideo.Extractor == "twitch:stream" {
		// yt-dlp thinks the title for twitch live streams should be '{name} (live)'... The description is the actual title.
		if OutVideo.Description != "" {
			VideoInfo.Title = OutVideo.Description
		}
	}
	
	if OutVideo.Description != "" {
		VideoInfo.Description = OutVideo.Description
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
	if OutVideo.Thumbnail != "" {
		VideoInfo.OriginThumbnail = OutVideo.Thumbnail
	}
	
	if OutVideo.Extractor == "twitch:clips" && OutVideo.ChannelName != "" {
		VideoInfo.UploaderName = OutVideo.ChannelName
	} else if OutVideo.UploaderName != "" {
		VideoInfo.UploaderName = OutVideo.UploaderName
	} else if OutVideo.UploaderId != "" {
		VideoInfo.UploaderName = OutVideo.UploaderId
	}
	
	if OutVideo.ChannelUrl != "" {
		VideoInfo.UploaderUrl = OutVideo.ChannelUrl
	} else if OutVideo.UploaderUrl != "" {
		VideoInfo.UploaderUrl = OutVideo.UploaderUrl
	} else {
		// Try to get uploader url...
		if OutVideo.Extractor == "twitch:vod" || OutVideo.Extractor == "twitch:stream" {
			// yt-dlp doesn't set uploader_url for twitch
			if OutVideo.UploaderId != "" {
				VideoInfo.UploaderUrl = fmt.Sprintf("https://www.twitch.tv/%s", OutVideo.UploaderId)
			}
		} else if OutVideo.Extractor == "twitch:clips" {
			if OutVideo.ChannelName != "" {
				VideoInfo.UploaderUrl = fmt.Sprintf("https://www.twitch.tv/%s", OutVideo.ChannelName)
			}
		}
	}
	
	VideoInfo.Filename = OutVideo.Filename
	
	if OutVideo.ReleaseTimestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.ReleaseTimestamp
	} else if OutVideo.Timestamp != 0 {
		VideoInfo.ReleaseDate = OutVideo.Timestamp
	}
}

func GetDownloadDir(AChannel *ArchiveChannel) string {
	DownloadDir := AChannel.DownloadDir
	if DownloadDir == "" {
		DownloadDir = G_Config.Default_DownloadDir
	}
	
	if filepath.IsLocal(DownloadDir) {
		DownloadDir = filepath.Join(CURRENT_WORKING_DIRECTORY, DownloadDir)
	}
	
	return DownloadDir
}
func GetOutputTemplate(AChannel *ArchiveChannel) string {
	OutputTemplate := AChannel.OutputTemplate
	if OutputTemplate == "" {
		OutputTemplate = G_Config.Default_YtDlp_OutputTemplate
		if AChannel.Type == ACHANNEL_TYPE_LIVE {
			OutputTemplate = G_Config.Default_YtDlp_OutputTemplate_Live
		}
	}
	return OutputTemplate
}

func ShouldLiveFromStart(AChannel *ArchiveChannel, VideoUrl string) bool {
	if strings.Contains(VideoUrl, "twitch.tv/") {
		return false
	}
	
	return true
}

type VideoFileInfo struct {
	Width     int `json:"width"`
	Height    int `json:"height"`
	Duration  float64 `json:"duration"`
	RFramerate   string `json:"r_frame_rate"`
	AvgFramerate string `json:"avg_frame_rate"`
	Framerate float64
}

type FFprobe_Output struct {
	Streams []VideoFileInfo `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func GetVideoFileInfo(filePath string) (*VideoFileInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate:format=duration",
		"-of", "json",
		filePath,
	)
	
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %w", err)
	}
	
	var data FFprobe_Output
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}
	if len(data.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found")
	}
	
	Info := data.Streams[0]
	
	//Info.Duration = Info.Format.Duration
	fmt.Sscanf(data.Format.Duration, "%f", &Info.Duration)
	
	Rate := Info.AvgFramerate
	if Rate == "0/0" || Rate == "" {
		Rate = Info.RFramerate
	}
	
	RateParts := strings.Split(Rate, "/")
	if len(RateParts) > 1 {
		var n, d float64
		fmt.Sscanf(RateParts[0], "%f", &n)
		fmt.Sscanf(RateParts[1], "%f", &d)
		
		Info.Framerate = n/d
	} else {
		Info.Framerate = 1
	}
	
	return &Info, nil
}

func RequestVideoInfo(AChannel *ArchiveChannel, VideoUrl string, QualitySelect int, Video *VideoInfo, Task *CommandTask) (error) {
	DownloadDir    := GetDownloadDir(AChannel)
	OutputTemplate := GetOutputTemplate(AChannel)
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		CL_Logf(Task, "Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	Args := []string{
		VideoUrl,
		"--ignore-config",
		"--config-locations", GLOBAL_YT_DLP_CONFIG_PATH,
		"--dump-json",
		"--skip-download",
		//"--restrict-filenames",
		"-o", OutputTemplate,
	}
	
	if QualitySelect == -1 {
		QualitySelect = AChannel.QualitySelect
	}
	
	if QualitySelect > 0 {
		Args = append(Args, "-S", fmt.Sprintf("res:%d", QualitySelect))
	}
	if ShouldLiveFromStart(AChannel, VideoUrl) {
		Args = append(Args, "--live-from-start")
	}
	
	Cmd := exec.Command(Get_YtDlpPath(G_Config), Args...)
	Cmd.Dir = DownloadDir
	
	stderr, err := Cmd.StderrPipe()
	if err != nil {
		CL_Logf(Task, "Error when creating StderrPipe: %v\n", err)
		return err
	}
	ErrOut := CL_BasicWatchStdPipe(stderr)
	
	Out, err := Cmd.Output()
	if (Task != nil && Task.Status != TASK_STATUS_RUNNING) {
		return nil
	}
	if err != nil {
		ErrOut.Lock.RLock()
		ErrOutput := ErrOut.RawOutput
		ErrOut.Lock.RUnlock()
		
		if Video.VideoType == VIDEO_TYPE_ISLIVE {
			Video.VideoType = VIDEO_TYPE_WASLIVE
		}
		
		FilePath, fpErr := GetDownloadedVideoFilePath(Video, AChannel)
		if fpErr == nil && FilePath != "" {
			VFileInfo, err := GetVideoFileInfo(FilePath)
			if err != nil {
				CL_Logf(Task, "Could not get video file info because: %v\n", err)
			}
			if Video.Duration <= 1 {
				Video.Duration = VFileInfo.Duration
			}
		}
		
		if strings.Contains(ErrOutput, "Private video.") {
			Video.Availability = "private"
			return fmt.Errorf("%s", ErrOutput)
		} else if strings.Contains(ErrOutput, "Sign in to confirm your age.") ||
				  strings.Contains(ErrOutput, "This video may be inappropriate for some users.") {
			Video.Availability = "age-restricted"
			return fmt.Errorf("%s", ErrOutput)
		} else if strings.Contains(ErrOutput, "This video has been removed by the uploader") ||
				  strings.Contains(ErrOutput, "Video unavailable.") ||
				  strings.Contains(ErrOutput, "This video has been removed for violating") {
			Video.Availability = "removed"
			return fmt.Errorf("%s", ErrOutput)
		} else if strings.Contains(ErrOutput, "This live event will begin in a few moments.") ||
				  strings.Contains(ErrOutput, "This live event will begin in") {
			return fmt.Errorf("%s", ErrOutput)
		}
		
		CL_Logf(Task, "%s\n", ErrOutput)
		CL_Logf(Task, "Failed to get video info from url: %s, Error: %v\n", VideoUrl, err)
		return fmt.Errorf("%s", ErrOutput)
	}
	var OutVideo YT_DLP_OUTVIDEO
	err = json.Unmarshal(Out, &OutVideo)
	if err != nil {
		CL_Logf(Task, "json.Unmarshal err: %v\n", err)
		return err
	}
	
	OldVideoId := Video.Id
	PopulateVideoInfoFromOutVideo(Video, OutVideo)
	if OutVideo.Availability == "" {
		// Grabbing video info was successful! Assume availability is 'public'.
		Video.Availability = "public"
	}
	
	// Some platforms (like twitch) might give out a completely different video ids...
	if OldVideoId != "" {
		Video.Id = OldVideoId
	}
	
	return nil
}

func yt_dlp_ListVideos(ChannelUrl string, PlaylistEnd int, Task *CommandTask) ([]VideoInfo, error) {
	Args := []string{
		ChannelUrl,
		"--ignore-config",
		"--config-locations", GLOBAL_YT_DLP_CONFIG_PATH,
		"--flat-playlist",
		"--dump-json",
		"--skip-download",
		//"--restrict-filenames",
		"--extractor-args", "youtubetab:approximate_date",
	}
	if PlaylistEnd > 0 {
		Args = append(Args, "--playlist-end", fmt.Sprintf("%d", PlaylistEnd))
	}
	
	Cmd := exec.Command(Get_YtDlpPath(G_Config), Args...)
	if Task != nil {
		CL_Logf(Task, fmt.Sprintf(">%s\n\n", GetRealArgs(Cmd.Args)))
	}
	
	stderr, err := Cmd.StderrPipe()
	if err != nil {
		CL_Logf(Task, "Error when creating StderrPipe: %v\n", err)
		return nil, err
	}
	ErrOut := CL_BasicWatchStdPipe(stderr)
	
	var OutVideos []VideoInfo
	
	Out, err := Cmd.Output()
	if err != nil {
		if Task != nil && Task.Status != TASK_STATUS_RUNNING {
			return nil, err
		}
		ErrOut.Lock.RLock()
		ErrOutput := ErrOut.RawOutput
		ErrOut.Lock.RUnlock()
		
		if Task != nil {
			CL_Logf(Task, "%v\n", ErrOutput)
			DB_UpdateCommandTaskInfo(Task)
		}
		
		// Very crude way of checking if yt-dlp errored because of no videos in the list...
		if strings.Contains(ChannelUrl, "youtube.com/") {
			if strings.Contains(ErrOutput, "This channel does not have a streams tab") {
				return OutVideos, nil
			}
		} else if strings.Contains(ChannelUrl, "twitch.tv/") {
			if strings.Contains(ErrOutput, "The channel is not currently live") {
				return OutVideos, nil
			}
		}
		
		ErrorMsg := fmt.Sprintf("Failed to list videos from channel: %s, Error: %v\n", ChannelUrl, err)
		if Task != nil {
			CL_Logf(Task, "%s", ErrorMsg)
			DB_UpdateCommandTaskInfo(Task)
		}
		return nil, err
	}
	
	scanner := bufio.NewScanner(strings.NewReader(string(Out)))
	ScannedAnything := false
	for scanner.Scan() {
		ScannedAnything = true
		OutVideo := YT_DLP_OUTVIDEO{}
		err = json.Unmarshal([]byte(scanner.Text()), &OutVideo)
		if err != nil {
			ErrorMsg := fmt.Sprintf("Error when decoding json: %v\n", err)
			L_Printf("%s", ErrorMsg)
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
	if ScannedAnything == false {
		// The output json must not be a playlist?
		var VideoInfo VideoInfo
		
		OutVideo := YT_DLP_OUTVIDEO{}
		err = json.Unmarshal([]byte(Out), &OutVideo)
		if err == nil {
			PopulateVideoInfoFromOutVideo(&VideoInfo, OutVideo)
			OutVideos = append(OutVideos, VideoInfo)
		} else {
			L_Printf("Json error: %v\n", err)
		}
	}
	
	if Task != nil {
		DB_UpdateCommandTaskInfo(Task)
	}
	
	return OutVideos, nil
}


// This must be called with a Video that has been passed through RequestVideoInfo()
func yt_dlp_DownloadVideo(AChannel *ArchiveChannel, Video *VideoInfo, QualitySelect int) (error) {
	DownloadDir    := GetDownloadDir(AChannel)
	//OutputTemplate := GetOutputTemplate(AChannel)
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		L_Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	Filename := Video.Filename
	
	FileExtension := filepath.Ext(Filename)
	FilenameWithoutExt := strings.TrimSuffix(Filename, FileExtension)
	DB_UpdateVideoFilename(Video, Filename)
	
	Args := []string{
		Video.Url,
		"--ignore-config",
		"--config-locations", GLOBAL_YT_DLP_CONFIG_PATH,
		"--embed-metadata",
		"--external-downloader-args", "ffmpeg: -loglevel warning -stats",
		"-o", fmt.Sprintf("%s.%%(ext)s", FilenameWithoutExt),
	}
	FFmpegPath := Get_FFmpegPath(G_Config)
	if filepath.IsAbs(FFmpegPath) {
		// '--ffmpeg-location' doesn't handle system environment paths.
		// Only add the ffmpeg location if it's absolute!
		
		Args = append(Args, "--ffmpeg-location", FFmpegPath)
	}
	DenoPath := Get_DenoPath(G_Config)
	if filepath.IsAbs(DenoPath) {
		Args = append(Args, "--js-runtimes", fmt.Sprintf("deno:%s", DenoPath))
	}
	
	if QualitySelect > 0 {
		Args = append(Args, "-S", fmt.Sprintf("res:%d", QualitySelect))
	}
	
	Cmd := exec.Command(Get_YtDlpPath(G_Config), Args...)
	Cmd.Dir = DownloadDir
	CL_RunDownloadTask(Cmd, Video, AChannel.Id)
	
	err = Cmd.Start()
	if err != nil {
		L_Printf("Failed to start download video from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	err = Cmd.Wait()
	if err != nil {
		L_Printf("Failed to download video from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	//L_Printf("Output: %s\n", Out)
	
	return nil
}



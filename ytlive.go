package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// This must be called with a Video that has been passed through RequestVideoInfo()
func ytarchive_DownloadLive(AChannel *ArchiveChannel, Video *VideoInfo, QualitySelect int) (error) {
	DownloadDir := GetDownloadDir(AChannel)
	
	err := os.MkdirAll(DownloadDir, 0755)
	if err != nil {
		L_Printf("Could not make directory \"%s\" err: %v\n", DownloadDir, err)
	}
	
	//144p, 240p, 360p, 480p, 720p, 720p60, 1080p, 1080p60, 1440p, 1440p60, 2160p, 2160p60, best
	//QualitySelect := AChannel.QualitySelect
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
	
	//DateAndTime := time.Unix(Video.ReleaseDate, 0).Format("2006-01-02")
	
	Filename := Video.Filename
	if Video.DownloadedFilename != "" {
		Filename = Video.DownloadedFilename
	}
	
	FileExtension := filepath.Ext(Filename)
	FilenameWithoutExt := strings.TrimSuffix(Filename, FileExtension)
	DB_UpdateVideoFilename(Video, Filename)
	
	Args := []string{
		"--ffmpeg-path", Get_FFmpegPath(G_Config),
		"--ytdlp-path",  Get_YtDlpPath(G_Config),
		//"--ytdlp-opts", fmt.Sprintf("--config-locations \"%s\"", GLOBAL_YT_DLP_CONFIG_PATH),
		"--no-wait",
		"--add-metadata",
		"--save-state",
		"--threads", "2",
		"-o", FilenameWithoutExt,
	}
	
	if QualitySelect != 0 && QualitySelect <= 1080 {
		Args = append(Args, "--h264")
	}
	
	Args = append(Args, Video.Url, QualityString)
	
	Cmd := exec.Command(
		Get_YtArchivePath(G_Config),
		Args ...,
	)
	Cmd.Dir = DownloadDir
	CL_RunDownloadTask(Cmd, Video, AChannel.Id)
	
	err = Cmd.Start()
	if err != nil {
		L_Printf("Failed to start live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	err = Cmd.Wait()
	if err != nil {
		L_Printf("Failed to live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	//L_Printf("Output: %s\n", Out)
	
	return nil
}
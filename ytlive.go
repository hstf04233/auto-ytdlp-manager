package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"autoytdlpmanager/os_tings"
	"github.com/natefinch/npipe" // Only used on Windows
)

type ytarchive_State struct {
	StateFilePath string
	
	StartFrag int
	Fragments int
	Size      uint64
	TempDir   string
}

func Get_ytarchive_State(DownloadDir string, VideoId string) (*ytarchive_State, bool) {
	StateFilePath := ""
	
	AudioFilePath := filepath.Join(DownloadDir, fmt.Sprintf("%s.f140.state", VideoId))
	if DoesFileExist(AudioFilePath) {
		StateFilePath = AudioFilePath
	}
	
	if StateFilePath == "" {
		// We could not find the audio state... Go nuclear and search through every file in the directory...
		Files, err := os.ReadDir(DownloadDir)
		if err == nil {
			for _, File := range(Files) {
				if File.IsDir() { continue }
				
				FileName := File.Name()
				if strings.HasPrefix(FileName, VideoId) && strings.HasSuffix(FileName, ".state") {
					// This is possibly what we are looking for?
					StateFilePath = filepath.Join(DownloadDir, FileName)
					break
				}
			}
		}
	}
	
	if StateFilePath == "" {
		return nil, false
	}
	
	L_Printf("Found state! StateFilePath: %s\n", StateFilePath)
	
	FileContents, err := os.ReadFile(StateFilePath)
	if err != nil {
		L_Printf("Failed to read '%s', error: %v\n", StateFilePath, err)
		return nil, false
	}
	
	State := &ytarchive_State{}
	err = json.Unmarshal(FileContents, State)
	if err != nil {
		L_Printf("Failed to decode state file '%s', error: %v\n", StateFilePath, err)
		return nil, false
	}
	State.StateFilePath = StateFilePath
	
	return State, true
}

type ytarchive_VideoAndAudio struct {
	VideoPath string
	AudioPath string
}

func Get_ytarchive_VideoAndAudioDownloadFiles(State ytarchive_State) (ytarchive_VideoAndAudio, bool) {
	var VideoAndAudio ytarchive_VideoAndAudio
	
	Files, err := os.ReadDir(State.TempDir)
	if err == nil {
		VideoPath := ""
		VideoModTime := time.Unix(0, 0)
		AudioPath := ""
		AudioModTime := time.Unix(0, 0)
		for _, File := range(Files) {
			if File.IsDir() { continue }
			
			FileName := File.Name()
			if strings.Contains(FileName, ".frag") {
				// TODO: This is a VERY crude way of checking if this is a fragment file or not...
				continue
			}
			
			FilePath := filepath.Join(State.TempDir, FileName)
			FileInfo, ierr := File.Info()
			if ierr != nil {
				L_Printf("Failed to get file info from '%s', error: %v\n", FilePath, err)
				continue
			}
			ModTime := FileInfo.ModTime()
			if strings.HasSuffix(FileName, ".f140.ts") {
				// This is the audio file... (hopefully)
				if ModTime.After(AudioModTime) {
					AudioPath = FilePath
					AudioModTime = ModTime
				}
			} else if strings.HasSuffix(FileName, ".ts") {
				// This might be the video file? (hopefully 🤞)
				if ModTime.After(VideoModTime) {
					VideoPath = FilePath
					VideoModTime = ModTime
				}
			}
		}
		
		if AudioPath == "" || VideoPath == "" {
			// TODO: Should this just return successfully instead if both video and audio arent found?
			return VideoAndAudio, false
		}
		
		VideoAndAudio.VideoPath = VideoPath
		VideoAndAudio.AudioPath = AudioPath
		
		return  VideoAndAudio, true
	} else {
		L_Printf("Could not ReadDir '%s', error: %v\n", State.TempDir, err)
		return VideoAndAudio, false
	}
	
	return VideoAndAudio, false
}

func GetPipeName(PipeName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\%s`, PipeName)
	}
	
	// It's a unix system
	return filepath.Join("/tmp", PipeName)
}

func CreatePipe(PipeName string) (io.WriteCloser, error) {
	if runtime.GOOS == "windows" {
		listener, err := npipe.Listen(PipeName)
		if err != nil {
			return nil, err
		}
		// Accept one connection (FFmpeg will connect as client)
		return listener.Accept()
	}
	
	// Remove old pipe if exists
	os.Remove(PipeName)
	
	// Create FIFO on Unix
	cmd := exec.Command("mkfifo", PipeName)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create fifo %s: %v", PipeName, err)
	}
	return os.OpenFile(PipeName, os.O_WRONLY, 0666)
}

func OpenPipe(PipeName string) (io.WriteCloser, error) {
	if runtime.GOOS == "windows" {
		return npipe.DialTimeout(PipeName, time.Second * 5)
	}
	return os.OpenFile(PipeName, os.O_WRONLY, 0666)
}

func ReadFileAndWriteToPipe(PipeName string, InputFile *os.File, DownloadTask *CommandTask, Task *CommandTask) {
	L_Printf("Creating pipe: %v\n", PipeName)
	VideoPipe, err := CreatePipe(PipeName)
	if err != nil {
		L_Printf("Failed to create pipe: '%s': %v", PipeName, err)
		return
	}
	defer VideoPipe.Close()
	L_Printf("Video pipe created!: %v\n", PipeName)
	
	VideoBuf := make([]byte, 16384)
	for CL_IsRunning(DownloadTask) && CL_IsRunning(Task) {
		Count, err := InputFile.Read(VideoBuf)
		if err == io.EOF {
			time.Sleep(100 * time.Millisecond)
			continue
		} else if err != nil {
			L_Printf("File read errored for pipe: '%s' error: %v\n", PipeName, err)
			break
		}
		VideoPipe.Write(VideoBuf[0:Count])
	}
}

func TurnYTLiveIntoM3U8LiveStream(DownloadTask *CommandTask, DownloadDir string, AChannel *ArchiveChannel, Video *VideoInfo) (error) {
	VideoId := Video.Id
	var State *ytarchive_State
	
	for {
		FindState, ok := Get_ytarchive_State(DownloadDir, VideoId)
		if ok {
			State = FindState
			break
		}
		
		// Wait before finding the state again.
		time.Sleep(time.Second * 2)
		
		if !CL_IsRunning(DownloadTask) {
			return nil
		}
	}
	if State == nil {
		// wat
		return nil
	}
	
	VideoAndAudio, ok := Get_ytarchive_VideoAndAudioDownloadFiles(*State)
	if !ok {
		return nil
	}
	
	VideoFile, err := os_tings.OpenFileWithoutLocking(VideoAndAudio.VideoPath)
	if err != nil {
		L_Printf("Failed to open video file '%s', error: %v\n", VideoAndAudio.VideoPath, err)
		return err
	}
	defer VideoFile.Close()
	AudioFile, err := os_tings.OpenFileWithoutLocking(VideoAndAudio.AudioPath)
	if err != nil {
		L_Printf("Failed to open audio file '%s', error: %v\n", VideoAndAudio.AudioPath, err)
		return err
	}
	defer AudioFile.Close()
	
	Pipe1Name := GetPipeName(fmt.Sprintf("video_pipe_%s", VideoId))
	Pipe2Name := GetPipeName(fmt.Sprintf("audio_pipe_%s", VideoId))
	
	TempDirectory, err := os.MkdirTemp(DownloadDir, fmt.Sprintf("TEMP_WILL_DELETE_streamed_live-%s-*", VideoId))
	if err != nil {
		return fmt.Errorf("Failed to create temporary directory, error: %v", err)
	}
	defer func(){
		err := os.RemoveAll(TempDirectory)
		if err != nil {
			DB_UpdateVideoStreamedDirectory(Video, "")
		}
	}()
	DB_UpdateVideoStreamedDirectory(Video, TempDirectory)
	
	FFmpegCmd := exec.Command(Get_FFmpegPath(G_Config),
		"-i", Pipe1Name,
		"-i", Pipe2Name,
		"-loglevel", "error", "-stats",
		"-y",
		
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "1",
		"-hls_list_size", "60",
		"-hls_delete_threshold", "10",
		"-hls_flags", "delete_segments+append_list+omit_endlist",
		
		"-hls_segment_filename", "segment_%03d.ts",
		
		"playlist.m3u8",
	)
	FFmpegCmd.Dir = TempDirectory
	
	Task, err := CL_RunDownloadTask(FFmpegCmd, Video, AChannel.Id)
	if err != nil {
		return fmt.Errorf("Failed to create download task: %v", err)
	}
	defer func() {
		if Task != nil && CL_IsRunning(Task) {
			CL_FinishTask(Task, TASK_STATUS_FAILED)
		}
	}()
	
	if err := FFmpegCmd.Start(); err != nil {
		return fmt.Errorf("Failed to start ffmpeg because: %v", err)
	}
	
	go ReadFileAndWriteToPipe(Pipe1Name, VideoFile, DownloadTask, Task)
	go ReadFileAndWriteToPipe(Pipe2Name, AudioFile, DownloadTask, Task)
	
	go func() {
		if err := FFmpegCmd.Wait(); err != nil {
			L_Printf("Vid stream failed, error: %v\n", err)
		}
	}()
	
	for CL_IsRunning(DownloadTask) && CL_IsRunning(Task) {
		// Check if the video and audio are still being downloaded.
		if !DoesFileExist(VideoAndAudio.VideoPath) {
			break
		}
		if !DoesFileExist(VideoAndAudio.AudioPath) {
			break
		}
		
		time.Sleep(50 * time.Millisecond)
	}
	
	return nil
}

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
	DownloadTask, task_err := CL_RunDownloadTask(Cmd, Video, AChannel.Id)
	
	err = Cmd.Start()
	if err != nil {
		L_Printf("Failed to start live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	
	if task_err == nil {
		go func() {
			err := TurnYTLiveIntoM3U8LiveStream(DownloadTask, DownloadDir, AChannel, Video)
			if err != nil {
				L_Printf("Failed to TurnYTLiveIntoM3U8LiveStream, error: %v\n", err)
			}
		}()
	}
	
	err = Cmd.Wait()
	if err != nil {
		L_Printf("Failed to live download from url: %s, Error: %v\n", Video.Url, err)
		return err
	}
	//L_Printf("Output: %s\n", Out)
	
	return nil
}
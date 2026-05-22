package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	CONFIG_PATH = "config.json"
	CONFIG_PATH_DEBUG = "config_DEBUG.json"
	
	YT_DLP_CONFIG_FILENAME = "ytdlp_config.txt"
	
	DEFAULT_SERVER_PORT = 8867
	DEFAULT_SERVER_PORT_DEBUG = 6788
	
	DEFAULT_DOWNLOAD_DIR = "./downloads"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE      = "%(title)s %(id)s.%(ext)s"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE = "%(release_date>%Y-%m-%d,upload_date>%Y-%m-%d)s %(title)s %(id)s.%(ext)s"
	
	MAX_TASK_LOG_LIFETIME        = (60*60*24 * 30) // 1 Month
	MAX_CHANNEL_LISTING_LIFETIME = (60*60*24)      // 1 Day
)

var (
	GLOBAL_YT_DLP_CONFIG_PATH = ""
)

type ProgramConfig struct {
	Mutex *sync.RWMutex `json:"-"`
	ConfigFile *os.File `json:"-"`
	
	ServerPort uint16
	
	YtDlp_Path     string
	YtArchive_Path string
	FFmpeg_Path    string
	Deno_Path      string `json:"-"`
	YtDlp_Path_Real     string `json:"-"`
	YtArchive_Path_Real string `json:"-"`
	FFmpeg_Path_Real    string `json:"-"`
	Deno_Path_Real      string `json:"-"`
	
	AllChannels_Disabled bool
	
	Default_DownloadDir string
	Default_YtDlp_OutputTemplate      string
	Default_YtDlp_OutputTemplate_Live string
	
	TaskLog_AutoDelete_Enabled      bool
	TaskLog_AutoDelete_Seconds      int
	TaskLog_List_AutoDelete_Seconds int
}

var G_Config = &ProgramConfig{
	Mutex: &sync.RWMutex{},
	ServerPort: DEFAULT_SERVER_PORT,
	
	YtDlp_Path:     "yt-dlp",
	YtArchive_Path: "ytarchive",
	FFmpeg_Path:    "ffmpeg",
	Deno_Path:      "deno",
	
	AllChannels_Disabled: false,
	
	Default_DownloadDir: "./downloads",
	Default_YtDlp_OutputTemplate:      DEFAULT_YT_DLP_OUTPUT_TEMPLATE,
	Default_YtDlp_OutputTemplate_Live: DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE,
	
	TaskLog_AutoDelete_Enabled:      true,
	TaskLog_AutoDelete_Seconds:      MAX_TASK_LOG_LIFETIME,
	TaskLog_List_AutoDelete_Seconds: MAX_CHANNEL_LISTING_LIFETIME,
}

func Get_YtDlpPath(Config *ProgramConfig) string {
	Config.Mutex.RLock()
	defer Config.Mutex.RUnlock()
	return Config.YtDlp_Path_Real
}
func Get_YtArchivePath(Config *ProgramConfig) string {
	Config.Mutex.RLock()
	defer Config.Mutex.RUnlock()
	return Config.YtArchive_Path_Real
}
func Get_FFmpegPath(Config *ProgramConfig) string {
	Config.Mutex.RLock()
	defer Config.Mutex.RUnlock()
	return Config.FFmpeg_Path_Real
}
func Get_DenoPath(Config *ProgramConfig) string {
	Config.Mutex.RLock()
	defer Config.Mutex.RUnlock()
	return Config.Deno_Path_Real
}

func UpdateConfig(Config *ProgramConfig) error {
	Config.Mutex.Lock()
	defer Config.Mutex.Unlock()
	
	err := SaveConfig(Config, Config.ConfigFile)
	if err != nil {
		return err
	}
	
	Config.YtDlp_Path_Real     = FindCommand(Config.YtDlp_Path)
	Config.YtArchive_Path_Real = FindCommand(Config.YtArchive_Path)
	Config.FFmpeg_Path_Real    = FindCommand(Config.FFmpeg_Path)
	
	return nil
}

func SaveConfig(Config *ProgramConfig, File *os.File) error {
	err := File.Truncate(0)
	if err != nil {
		return fmt.Errorf("Could not write config json, error %v\n", err)
	}
	_, err = File.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("Could not write config json, error %v\n", err)
	}
	
	JsonEncoder := json.NewEncoder(File)
	JsonEncoder.SetEscapeHTML(false)  // DIE! (I disable this setting because it messes with the YtDlp_OutputTemplate)
	JsonEncoder.SetIndent("", "\t")
	
	err = JsonEncoder.Encode(G_Config)
	if err != nil {
		return fmt.Errorf("Could not write config json, error %v\n", err)
	}
	return nil
}

func OpenConfig(ConfigPath string) error {
	if APPLICATION_VERSION == "debug" {
		G_Config.ServerPort = DEFAULT_SERVER_PORT_DEBUG
	}
	
	YtDlpConfigPath := filepath.Join(CURRENT_WORKING_DIRECTORY, YT_DLP_CONFIG_FILENAME)
	GLOBAL_YT_DLP_CONFIG_PATH = YtDlpConfigPath
	
	YtDlpConfigFile, err := os.OpenFile(YtDlpConfigPath, os.O_RDWR, 0644)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		YtDlpConfigFile, err = os.OpenFile(YtDlpConfigPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
		if err == nil {
			YtDlpConfigFile.Write([]byte(`# Add yt-dlp commands here!
# See https://github.com/yt-dlp/yt-dlp#configuration on how to use yt-dlp configs.

# Add cookies to yt-dlp with:
#--cookies "path/to/cookies.txt"

`))
			defer YtDlpConfigFile.Close()
		}
	} else if err == nil {
		defer YtDlpConfigFile.Close()
	}
	
	ConfigFile, err := os.OpenFile(ConfigPath, os.O_RDWR, 0644)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		// The config doesn't exist! Write the default config.
		NewConfigFile, err := os.OpenFile(ConfigPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("Could not create default config file '%s' %v\n", ConfigPath, err)
		}
		
		err = SaveConfig(G_Config, NewConfigFile)
		if err != nil {
			return fmt.Errorf("Could not write config file, error %v\n", err)
		}
		
		G_Config.ConfigFile = NewConfigFile
		
		return nil
	}
	if err != nil {
		return fmt.Errorf("Error when opening config file '%s' %v\n", ConfigPath, err)
	}
	
	ConfigContent, err := io.ReadAll(ConfigFile)
	if err != nil {
		return fmt.Errorf("Error when reading config file '%s' %v\n", ConfigPath, err)
	}
	err = json.Unmarshal(ConfigContent, &G_Config)
	if err != nil {
		return fmt.Errorf("Error when decoding config file json '%s' %v\n", ConfigPath, err)
	}
	
	G_Config.ConfigFile = ConfigFile
	
	return nil
}


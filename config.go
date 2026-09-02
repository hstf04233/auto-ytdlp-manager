package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	CONFIG_PATH = "config.json"
	CONFIG_PATH_DEBUG = "config_DEBUG.json"
	
	DEFAULT_RATE_LIMIT_KBPS_PER_IP = 10_000
	
	YT_DLP_CONFIG_FILENAME = "ytdlp_config.txt"
	
	DEFAULT_SERVER_PORT = 8867
	DEFAULT_SERVER_PORT_DEBUG = 6788
	
	TEMPORARY_DIRECTORY = "./temp"
	
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE      = "%(title)s %(id)s.%(ext)s"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE = "%(release_date>%Y-%m-%d,upload_date>%Y-%m-%d)s %(title)s %(id)s.%(ext)s"
	
	MAX_TASK_LOG_LIFETIME        = (60*60*24 * 30) // 1 Month
	MAX_CHANNEL_LISTING_LIFETIME = (60*60*24)      // 1 Day
)

const (
	IP_STRATEGY_DIRECT     = "direct"
	IP_STRATEGY_CLOUDFLARE = "cloudflare"
	IP_STRATEGY_REALIP     = "real_ip"
	IP_STRATEGY_FORWARDED  = "forwarded"
)

var (
	GLOBAL_YT_DLP_CONFIG_PATH = ""
)

type ProgramConfig struct {
	Mutex *sync.RWMutex `json:"-"`
	ConfigFile *os.File `json:"-"`
	
	ServerPort uint16
	IpStrategy string
	RateLimitKBPSPerPublicIp int
	
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
	
	AutoRefresh_Videos_Seconds int
	
	//
	
	Download_Live_Chat bool
	Download_Video_Thumbnails bool
	
	APPLICATION_VERSION string `json:"application_version"`
}

var G_Config = &ProgramConfig{
	Mutex: &sync.RWMutex{},
	ServerPort: DEFAULT_SERVER_PORT,
	IpStrategy: IP_STRATEGY_DIRECT,
	RateLimitKBPSPerPublicIp: DEFAULT_RATE_LIMIT_KBPS_PER_IP,
	
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
	
	AutoRefresh_Videos_Seconds: (60*60*24 * 30), // 1 Month
	
	Download_Video_Thumbnails: true,
	Download_Live_Chat: true,
	
	APPLICATION_VERSION: APPLICATION_VERSION,
}

func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
func FindCommand(cmd string) string {
	if runtime.GOOS == "windows" {
		if filepath.Ext(cmd) == "" {
			cmd += ".exe"
		}
	}
	
	if filepath.IsLocal(cmd) {
		LocalCmd := fmt.Sprintf("%s/%s", CURRENT_WORKING_DIRECTORY, cmd)
		if CommandExists(LocalCmd) {
			// Program exists in working directory
			return LocalCmd
		}
	}
	
	if CommandExists(cmd) {
		// Program exists in the system path environment
		return cmd
	}
	
	return cmd
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
func Get_RateLimitKBPSPerPublicIp(Config *ProgramConfig) int {
	Config.Mutex.RLock()
	defer Config.Mutex.RUnlock()
	return Config.RateLimitKBPSPerPublicIp
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
	
	err = JsonEncoder.Encode(Config)
	if err != nil {
		return fmt.Errorf("Could not write config json, error %v\n", err)
	}
	return nil
}

func OpenConfig(ConfigPath string) error {
	if APPLICATION_VERSION_TYPE == "debug" {
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
		// Config file doesn't exist! Write the default config.
		NewConfigFile, err := os.OpenFile(ConfigPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("Could not create default config file '%s' %v\n", ConfigPath, err)
		}
		
		err = SaveConfig(G_Config, NewConfigFile)
		if err != nil {
			NewConfigFile.Close()
			return fmt.Errorf("Could not write config file, error %v\n", err)
		}
		
		G_Config.ConfigFile = NewConfigFile
		
		return nil
	}
	if err != nil {
		return fmt.Errorf("Error when opening config file '%s' %v\n", ConfigPath, err)
	}
	
	// Read the config file
	
	ConfigContent, err := io.ReadAll(ConfigFile)
	if err != nil {
		ConfigFile.Close()
		return fmt.Errorf("Error when reading config file '%s' %v\n", ConfigPath, err)
	}
	err = json.Unmarshal(ConfigContent, &G_Config)
	if err != nil {
		ConfigFile.Close()
		return fmt.Errorf("Error when decoding config file json '%s' %v\n", ConfigPath, err)
	}
	
	G_Config.APPLICATION_VERSION = APPLICATION_VERSION // bruh
	
	G_Config.ConfigFile = ConfigFile
	
	return nil
}


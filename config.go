package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	CONFIG_PATH = "config.json"
	CONFIG_PATH_DEBUG = "config_DEBUG.json"
	
	DEFAULT_SERVER_PORT = 8867
	DEFAULT_SERVER_PORT_DEBUG = 6788
	
	C_CMD_YT_DLP = "yt-dlp"
	C_CMD_YT_ARCHIVE = "ytarchive"
	C_CMD_FFMPEG = "ffmpeg"
	
	DEFAULT_DOWNLOAD_DIR = "./downloads"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE      = "%(title)s %(id)s.%(ext)s"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE = "%(upload_date>%Y-%m-%d)s %(title)s %(id)s.%(ext)s"
	
	MAX_TASK_LOG_LIFETIME        = (60*60*24 * 7*2) // 2 Weeks
	MAX_CHANNEL_LISTING_LIFETIME = (60*60*24)       // 1 Day
)

type ProgramConfig struct {
	ServerPort uint16
	
	YtDlp_Path     string
	YtArchive_Path string
	FFmpeg_Path    string
	
	Default_DownloadDir string
	Default_YtDlp_OutputTemplate      string
	Default_YtDlp_OutputTemplate_Live string
	
	TaskLog_AutoDelete_Enabled      bool
	TaskLog_AutoDelete_Seconds      int
	TaskLog_List_AutoDelete_Seconds int
}

var G_Config = &ProgramConfig{
	ServerPort: DEFAULT_SERVER_PORT,
	
	YtDlp_Path:     "yt-dlp",
	YtArchive_Path: "ytarchive",
	FFmpeg_Path:    "ffmpeg",
	
	Default_DownloadDir: "./downloads",
	Default_YtDlp_OutputTemplate:      DEFAULT_YT_DLP_OUTPUT_TEMPLATE,
	Default_YtDlp_OutputTemplate_Live: DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE,
	
	TaskLog_AutoDelete_Enabled:      true,
	TaskLog_AutoDelete_Seconds:      MAX_TASK_LOG_LIFETIME,
	TaskLog_List_AutoDelete_Seconds: MAX_CHANNEL_LISTING_LIFETIME,
}


func OpenConfig(ConfigPath string) error {
	if APPLICATION_VERSION == "debug" {
		G_Config.ServerPort = DEFAULT_SERVER_PORT_DEBUG
	}
	
	ConfigContent, err := os.ReadFile(ConfigPath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		// The config doesn't exist! Write the default config.
		NewConfigFile, err := os.OpenFile(ConfigPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Could not create default config file '%s' %v\n", ConfigPath, err)
		}
		defer NewConfigFile.Close()
		
		JsonEncoder := json.NewEncoder(NewConfigFile)
    	JsonEncoder.SetEscapeHTML(false)  // DIE (I disable this setting because it messes with the YtDlp_OutputTemplate!!)
		JsonEncoder.SetIndent("", "\t")
		
		err = JsonEncoder.Encode(G_Config)
		if err != nil {
			return fmt.Errorf("Could not write config json, error %v\n", err)
		}
		
		return nil
	}
	if err != nil {
		return fmt.Errorf("Error when reading config file '%s' %v\n", ConfigPath, err)
	}
	
	err = json.Unmarshal(ConfigContent, &G_Config)
	if err != nil {
		return fmt.Errorf("Error when decoding config file json '%s' %v\n", ConfigPath, err)
	}
	
	return nil
}


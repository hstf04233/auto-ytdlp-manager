package main


const (
	SERVER_PORT = 8867
	SERVER_PORT_DEBUG = 6788
	
	C_CMD_YT_DLP = "yt-dlp"
	C_CMD_YT_ARCHIVE = "ytarchive"
	C_CMD_FFMPEG = "ffmpeg"
	
	DEFAULT_DOWNLOAD_DIR = "./downloads"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE      = "%(title)s %(id)s.%(ext)s"
	DEFAULT_YT_DLP_OUTPUT_TEMPLATE_LIVE = "%(release_date>%Y-%m-%d)s %(title)s %(id)s.%(ext)s"
	
	MAX_CHANNEL_LISTING_LIFETIME = (60*60*24)
	MAX_TASK_LOG_LIFETIME        = (60*60*24 * 7)	// 1 week
)

// TODO:
/*
import (
	"fmt"
	"encoding/json"
)
*/


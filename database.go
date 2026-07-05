package main

import (
	"database/sql"
	
	"crypto/sha3"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const DATABASE_FILE       = "autoytdlpmanager.db"
const DATABASE_FILE_DEBUG = "autoytdlpmanager_DEBUG.db"

const db_SQL_Header = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA secure_delete = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = normal;

CREATE TABLE IF NOT EXISTS ArchiveChannels (
	Id                TEXT PRIMARY KEY UNIQUE,
	Name              TEXT NOT NULL,
	Url               TEXT NOT NULL,
	DownloadDir       TEXT NOT NULL,
	OutputTemplate    TEXT NOT NULL,
	CheckInterval     INTEGER NOT NULL,
	FullCheckInterval INTEGER NOT NULL default 172800,
	PlaylistEnd       INTEGER NOT NULL default 20,
	
	QualitySelect INTEGER NOT NULL,
	PreferredVideoFormat TEXT NOT NULL,
	PreferredAudioFormat TEXT NOT NULL,
	
	Type    INTEGER NOT NULL,
	Enabled BOOLEAN,
	
	CreatedAt DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME
);

CREATE TABLE IF NOT EXISTS Videos (
	Id           TEXT PRIMARY KEY,
	FromChannel  TEXT NOT NULL,
	Title        TEXT DEFAULT '',
	Description  TEXT DEFAULT '',
	Url          TEXT NOT NULL,
	Availability TEXT DEFAULT '',
	Filename     TEXT DEFAULT '',
	FileSize     INTEGER DEFAULT 0,
	StreamedDirectory TEXT DEFAULT '',
	
	Resolution   TEXT DEFAULT '',
	Thumbnail       TEXT DEFAULT '',   /* Origin thumbnail */
	StoredThumbnail TEXT DEFAULT '',   /* Downloaded thumbnail id */
	Duration     FLOAT DEFAULT 0,
	
	UploaderUrl  TEXT DEFAULT '',
	UploaderName TEXT DEFAULT '',
	
	RefreshState INTEGER NOT NULL DEFAULT 0,
	Status       INTEGER NOT NULL DEFAULT 0,
	QueuedAction INTEGER NOT NULL DEFAULT 0,
	VideoType    INTEGER NOT NULL DEFAULT 0,
	
	ReleaseDate BIGINT NOT NULL,
	
	AddedAt   DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME,
	
	HistoryRevisionCount INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_videos_releasedate ON Videos(ReleaseDate);
CREATE INDEX IF NOT EXISTS idx_videos_addedat     ON Videos(AddedAt);
CREATE INDEX IF NOT EXISTS idx_videos_updatedat   ON Videos(UpdatedAt);
CREATE INDEX IF NOT EXISTS idx_videos_fromchannel ON Videos(FromChannel);

CREATE TABLE IF NOT EXISTS VideoHistory (
	HId INTEGER PRIMARY KEY AUTOINCREMENT,
	Revision INTEGER,
	
	Id           TEXT,
	Title        TEXT,
	Description  TEXT,
	Availability TEXT,
	Url          TEXT,
	
	Thumbnail       TEXT,   /* Origin thumbnail */
	StoredThumbnail TEXT,   /* Downloaded thumbnail id */
	Duration        FLOAT DEFAULT 0,
	
	UploaderUrl  TEXT,
	UploaderName TEXT,
	
	VideoType INTEGER,
	
	AddedAt   DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME
);

CREATE INDEX IF NOT EXISTS idx_videohistory_revision ON VideoHistory(Revision);
CREATE INDEX IF NOT EXISTS idx_videohistory_id ON VideoHistory(Id);

CREATE TABLE IF NOT EXISTS CommandTasks (
	Id     TEXT PRIMARY KEY,
	Title  TEXT DEFAULT '',
	Type   INTEGER NOT NULL DEFAULT 0,
	Status INTEGER NOT NULL DEFAULT 0,
	
	FromChannel TEXT default '',
	FromVideo   TEXT default '',
	
	RunArgs TEXT default '',
	Output  TEXT default '',
	
	StartTime DATETIME NOT NULL DEFAULT (datetime('now')),
	EndTime   DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_commandtasks_fromchannel ON CommandTasks(FromChannel);
CREATE INDEX IF NOT EXISTS idx_commandtasks_fromvideo   ON CommandTasks(FromVideo);

CREATE TABLE IF NOT EXISTS Images (
	Id         TEXT PRIMARY KEY,  /* Should a hashed value of ImageData */
	Sha256Hash TEXT NOT NULL DEFAULT '',
	Filename   TEXT NOT NULL DEFAULT '',
	
	ImageData BLOB,
	
	Type INTEGER NOT NULL DEFAULT 0,   /* Where did this image come from? */
	OriginUrl TEXT,
	
	AddedAt   DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME
);



/*
	Set all videos that were previously "downloading" videos to queued.
*/
UPDATE Videos
SET Status = 0
WHERE Status = 1;

/*
	Set all tasks that were previously "running" videos to canceled.
*/
UPDATE CommandTasks
SET Status = 3,
    Output = IFNULL(Output, '') || '
This task was abruptly canceled because the server closed before this task could finish.
'
WHERE Status = 0;

`

const (
	DB_IMAGE_TYPE_UPLOADED  = 0
	DB_IMAGE_TYPE_THUMBNAIL = 1
	
	DB_IMAGE_MAX_FILESIZE = (10 * 1000 * 1000)  // 10 MB
)

type DB_Image struct {
	Id         string `json:"id"`  /* Should a hashed value of ImageData */
	Sha256Hash string `json:"sha256_hash"`  /* Sha256 hash encoded to base64 */
	Filename   string `json:"filename"`
	
	Type int `json:"type"`
	OriginUrl string `json:"origin_url"`
	
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var GDB *sql.DB
var VideoDBLock sync.RWMutex

// Add or update archive channel.
func DB_UpdateArchiveChannel(AChannel *ArchiveChannel) error {
	AChannel.Lock.RLock()
	defer AChannel.Lock.RUnlock()
	TimeNow := time.Now().UTC()
	_, err := GDB.Exec(`
	INSERT INTO ArchiveChannels(Id, Name, Url, DownloadDir, OutputTemplate, QualitySelect, PreferredVideoFormat, PreferredAudioFormat, CheckInterval, FullCheckInterval, Type, PlaylistEnd, Enabled, UpdatedAt, CreatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Name=excluded.Name,
	Url=excluded.Url,
	DownloadDir=excluded.DownloadDir,
	OutputTemplate=excluded.OutputTemplate,
	
	QualitySelect=excluded.QualitySelect,
	PreferredVideoFormat=excluded.PreferredVideoFormat,
	PreferredAudioFormat=excluded.PreferredAudioFormat,
	
	CheckInterval=excluded.CheckInterval,
	FullCheckInterval=excluded.FullCheckInterval,
	Type=excluded.Type,
	PlaylistEnd=excluded.PlaylistEnd,
	Enabled=excluded.Enabled,
	UpdatedAt=excluded.UpdatedAt
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.OutputTemplate, AChannel.QualitySelect, AChannel.PreferredVideoFormat, AChannel.PreferredAudioFormat, AChannel.CheckInterval, AChannel.FullCheckInterval, AChannel.Type, AChannel.PlaylistEnd, AChannel.Enabled, TimeNow, TimeNow)
	
	if err != nil {
		L_Printf("DB_UpdateArchiveChannel ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_RemoveChannel(ChannelId string) error {
	_, err := GDB.Exec(`
	DELETE FROM ArchiveChannels WHERE Id = ?
	`, ChannelId)
	if err != nil {
		L_Printf("DB_RemoveChannel ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_ListChannels(Condition string) ([]*ArchiveChannel, error) {
	if Condition == "" {
		Condition = "ORDER BY CreatedAt ASC"
	}
	Rows, err := GDB.Query(fmt.Sprintf(`SELECT
	Id,
	Name,
	Url,
	DownloadDir, OutputTemplate, CheckInterval, FullCheckInterval,
	
	QualitySelect,
	PreferredVideoFormat,
	PreferredAudioFormat,
	
	Type, PlaylistEnd,
	Enabled FROM ArchiveChannels %s`, Condition))
	if err != nil {
		return nil, err
	}
	ChannelsList := []*ArchiveChannel{}
	for Rows.Next() {
		Channel := &ArchiveChannel{}
		err := Rows.Scan(
			&Channel.Id,
			&Channel.Name,
			&Channel.Url,
			&Channel.DownloadDir, &Channel.OutputTemplate, &Channel.CheckInterval, &Channel.FullCheckInterval,
			
			&Channel.QualitySelect,
			&Channel.PreferredVideoFormat,
			&Channel.PreferredAudioFormat,
			
			&Channel.Type,
			&Channel.PlaylistEnd,
			&Channel.Enabled)
		if err != nil {
			return nil, err
		}
		
		ChannelsList = append(ChannelsList, Channel)
	}
	
	return ChannelsList, nil
}

func DB_LoadChannels(WD *ArchiveChannelsBundle) error {
	ChannelsList, err := DB_ListChannels("")
	if err != nil {
		return err
	}
	WD.ChannelsLock.Lock()
	defer WD.ChannelsLock.Unlock()
	for _, Channel := range(ChannelsList) {
		Channel.NextCheckMSEC = 0
		Channel.NextFullChannelCheckMSEC = 0
		
		WD.Channels = append(WD.Channels, Channel)
	}
	
	return nil
}

func DB_UpdateVideoInfo(Video *VideoInfo) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	TimeNow := time.Now().UTC()
	_, err := GDB.Exec(`
	INSERT INTO Videos(Id, FromChannel, Title, Description, Url, Availability, Resolution, Thumbnail, ReleaseDate, Duration, UploaderName, UploaderUrl, VideoType, UpdatedAt, AddedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	FromChannel=excluded.FromChannel,
	Title=excluded.Title,
	Description=excluded.Description,
	Url=excluded.Url,
	Availability=excluded.Availability,
	Resolution=excluded.Resolution,
	Thumbnail=excluded.Thumbnail,
	
	ReleaseDate=excluded.ReleaseDate,
	Duration=excluded.Duration,
	
	UploaderName=excluded.UploaderName,
	UploaderUrl=excluded.UploaderUrl,
	
	VideoType=excluded.VideoType,
	UpdatedAt=excluded.UpdatedAt
	`, Video.Id, Video.FromChannel, Video.Title, Video.Description, Video.Url, Video.Availability, Video.Resolution, Video.OriginThumbnail, Video.ReleaseDate, Video.Duration, Video.UploaderName, Video.UploaderUrl, Video.VideoType, TimeNow, TimeNow)
	
	if err != nil {
		L_Printf("DB_UpdateVideoInfo ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_UpdateVideoStatus(Video *VideoInfo, NewStatus int) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	if Video.Status == NewStatus {
		// Nothing has changed. Don't update?
		return nil
	}
	
	Video.Status = NewStatus
	_, err := GDB.Exec(`
	UPDATE Videos SET Status = ?, UpdatedAt = ? WHERE Id = ?
	`, NewStatus, time.Now().UTC(), Video.Id)
	return err
}

func DB_UpdateVideoQueuedAction(Video *VideoInfo, NewQueuedAction int) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	if Video.QueuedAction == NewQueuedAction {
		return nil
	}
	
	Video.QueuedAction = NewQueuedAction
	_, err := GDB.Exec(`
	UPDATE Videos SET QueuedAction = ?, UpdatedAt = ? WHERE Id = ?
	`, NewQueuedAction, time.Now().UTC(), Video.Id)
	return err
}

func DB_UpdateVideoAvailability(Video *VideoInfo, Availability string) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.Availability = Availability
	_, err := GDB.Exec(`
	UPDATE Videos SET Availability = ?, UpdatedAt = ? WHERE Id = ?
	`, Availability, time.Now().UTC(), Video.Id)
	return err
}

func DB_UpdateVideoFilename(Video *VideoInfo, Filename string) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.DownloadedFilename = Filename
	_, err := GDB.Exec(`
	UPDATE Videos SET Filename = ?, UpdatedAt = ? WHERE Id = ?
	`, Filename, time.Now().UTC(), Video.Id)
	return err
}
func DB_UpdateVideoFileSize(Video *VideoInfo, FileSize uint64) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.FileSize = FileSize
	_, err := GDB.Exec(`
	UPDATE Videos SET FileSize = ?, UpdatedAt = ? WHERE Id = ?
	`, FileSize, time.Now().UTC(), Video.Id)
	return err
}
func DB_UpdateVideoStreamedDirectory(Video *VideoInfo, StreamedDirectory string) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.StreamedDirectory = StreamedDirectory
	_, err := GDB.Exec(`
	UPDATE Videos SET StreamedDirectory = ?, UpdatedAt = ? WHERE Id = ?
	`, StreamedDirectory, time.Now().UTC(), Video.Id)
	return err
}
func DB_UpdateVideoStoredThumbnail(Video *VideoInfo, StoredThumbnail string) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.Thumbnail = StoredThumbnail
	_, err := GDB.Exec(`
	UPDATE Videos SET StoredThumbnail = ?, UpdatedAt = ? WHERE Id = ?
	`, StoredThumbnail, time.Now().UTC(), Video.Id)
	return err
}

func DB_UpdateVideoRefreshState(Video *VideoInfo, RefreshState int) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.RefreshState = RefreshState
	_, err := GDB.Exec(`
	UPDATE Videos SET RefreshState = ? WHERE Id = ?
	`, RefreshState, Video.Id)
	if err != nil {
		L_Printf("DB_UpdateVideoRefreshState err: %v\n", err)
	}
	return err
}

func DB_DeleteVideo(Video *VideoInfo) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	_, err := GDB.Exec(`
	DELETE FROM Videos WHERE Id = ?
	`, Video.Id)
	return err
}

func DB_GetVideo(VideoId string) (*VideoInfo, error) {
	VideoDBLock.RLock()
	defer VideoDBLock.RUnlock()
	VideoInfo := &VideoInfo{}
	VideoRow := GDB.QueryRow(`
	SELECT FromChannel, Id, Title, Description, Url, Availability, Resolution, Thumbnail, StoredThumbnail, Filename, FileSize, StreamedDirectory, Status, QueuedAction, ReleaseDate, Duration,
	UploaderName, UploaderUrl,
	VideoType,
	AddedAt, UpdatedAt, RefreshState FROM Videos WHERE Id = ?
	`, VideoId)
	err := VideoRow.Scan(
		&VideoInfo.FromChannel,
		&VideoInfo.Id,
		&VideoInfo.Title,
		&VideoInfo.Description,
		&VideoInfo.Url,
		&VideoInfo.Availability,
		&VideoInfo.Resolution,
		&VideoInfo.OriginThumbnail,
		&VideoInfo.Thumbnail,
		&VideoInfo.DownloadedFilename,
		&VideoInfo.FileSize,
		&VideoInfo.StreamedDirectory,
		
		&VideoInfo.Status,
		&VideoInfo.QueuedAction,
		&VideoInfo.ReleaseDate,
		&VideoInfo.Duration,
		
		&VideoInfo.UploaderName,
		&VideoInfo.UploaderUrl,
		
		&VideoInfo.VideoType,
		
		&VideoInfo.AddedAt,
		&VideoInfo.UpdatedAt,
		&VideoInfo.RefreshState,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	
	return VideoInfo, nil
}

const (
	DB_VIDEO_ORDERBY_AddedAt     = 0
	DB_VIDEO_ORDERBY_ReleaseDate = 1
	DB_VIDEO_ORDERBY_UpdatedAt   = 2
)

type ListVideosQuery struct {
	Status int
	FromChannelId string
	SearchQuery string
	VideoType   int
	RefreshState int
	QueuedAction int
	
	OrderBy int
	OrderDirection int
	
	IgnoreFullInformation bool
}

func DB_ConstructQuery_ListVideos(Limit int, Offset int, Query ListVideosQuery, Statement *string, Args *[]interface{}) {
	if Query.Status != -1 || Query.FromChannelId != "" || Query.RefreshState != -1 || Query.QueuedAction != -1 || Query.VideoType != -1 {
		*Statement += " WHERE "
		AddAnd := false
		if Query.Status != -1 {
			*Statement += " Status = ?"
			*Args = append(*Args, Query.Status)
			AddAnd = true
		}
		if Query.FromChannelId != "" {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " FromChannel = ?"
			*Args = append(*Args, Query.FromChannelId)
			AddAnd = true
		}
		if Query.VideoType != -1 {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " VideoType = ?"
			*Args = append(*Args, Query.VideoType)
			AddAnd = true
		}
		if Query.RefreshState != -1 {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " RefreshState = ?"
			*Args = append(*Args, Query.RefreshState)
			AddAnd = true
		}
		if Query.QueuedAction != -1 {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " QueuedAction = ?"
			*Args = append(*Args, Query.QueuedAction)
			AddAnd = true
		}
	}
	
	OrderBy := "AddedAt"
	switch Query.OrderBy {
	case DB_VIDEO_ORDERBY_ReleaseDate:
		OrderBy = "ReleaseDate"
	case DB_VIDEO_ORDERBY_UpdatedAt:
		OrderBy = "UpdatedAt"
	}
	
	OrderDirection := "DESC"
	if Query.OrderDirection != 0 {
		OrderDirection = "ASC"
		if Query.OrderDirection == -1 {
			OrderDirection = "DESC"
		}
	}
	
	*Statement += fmt.Sprintf(" ORDER BY %s %s LIMIT ? OFFSET ?", OrderBy, OrderDirection)
	*Args = append(*Args, Limit, Offset)
}

func DB_ListVideos(Limit int, Offset int, Query ListVideosQuery) ([]*VideoInfo, error) {
	Args := []interface{}{}
	
	Statement := `
	SELECT Id, Title, Description, Availability, UploaderName, Status FROM Videos`
	
	QLimit := Limit
	QOffset := Offset
	
	IsSearching := false
	SearchWords := []string{}
	if Query.SearchQuery != "" {
		QLimit = -1
		QOffset = 0
		IsSearching = true
		SplitSearchQuery := strings.Split(Query.SearchQuery, " ")
		for i := 0; i < len(SplitSearchQuery); i++ {
			SearchWord := strings.ToLower(SplitSearchQuery[i])
			if !slices.Contains(SearchWords, SearchWord) {
				SearchWords = append(SearchWords, SearchWord)
			}
		}
	}
	DB_ConstructQuery_ListVideos(QLimit, QOffset, Query, &Statement, &Args)
	
	Rows, err := GDB.Query(Statement, Args...)
	if err != nil {
		return nil, err
	}
	defer Rows.Close()
	
	VideosList := []*VideoInfo{}
	
	Si := 0
	
	for Rows.Next() {
		TinyVideoInfo := &VideoInfo{}
		err := Rows.Scan(
			&TinyVideoInfo.Id,
			&TinyVideoInfo.Title,
			&TinyVideoInfo.Description,
			&TinyVideoInfo.Availability,
			&TinyVideoInfo.UploaderName,
			
			&TinyVideoInfo.Status,
		)
		if err != nil {
			return nil, err
		}
		
		if IsSearching {
			IsWhatWeAreLookingFor := true
			
			// Simple search!
			TitleLowercase        := strings.ToLower(TinyVideoInfo.Title)
			AvailabilityLowercase := strings.ToLower(TinyVideoInfo.Availability)
			IdLowercase           := strings.ToLower(TinyVideoInfo.Id)
			UploaderNameLowercase := strings.ToLower(TinyVideoInfo.UploaderName)
			
			//for _, Word := range(SearchWords) {
			for i := 0; i < len(SearchWords); i++ {
				Word := SearchWords[i]
				if !strings.Contains(TitleLowercase, Word) &&
				   !strings.Contains(AvailabilityLowercase, Word) &&
				   !strings.Contains(IdLowercase, Word) &&
				   !strings.Contains(UploaderNameLowercase, Word) {
					// This video is NOT what we are looking for...
					IsWhatWeAreLookingFor = false
					break
				}
			}
			
			if !IsWhatWeAreLookingFor {
				continue
			}
			// This video contains words from the search query!
			if Si < Offset {
				Si += 1
				continue
			}
		}
		
		VideosList = append(VideosList, TinyVideoInfo)
		if IsSearching {
			if Limit != -1 && len(VideosList) >= Limit {
				break
			}
		}
	}
	
	if !Query.IgnoreFullInformation {
		// We found the videos we want!
		for _, VideoInfo := range(VideosList) {
			FullVideoInfo, err := DB_GetVideo(VideoInfo.Id)
			if err == nil && FullVideoInfo != nil {
				*VideoInfo = *FullVideoInfo
			}
		}
	}
	
	return VideosList, nil
}

type DB_VideoListStats struct{
	Total int `json:"total"`
	
	TotalQueued      int `json:"total_queued"`
	TotalDownloading int `json:"total_downloading"`
	TotalDownloaded  int `json:"total_downloaded"`
	TotalFailed      int `json:"total_failed"`
	TotalIgnored     int `json:"total_ignored"`
}

var DB_VideoStatsCache = NewCache(time.Second * 4)

func DB_GetVideoStatsFromQuery(Query ListVideosQuery) (*DB_VideoListStats, error) {
	CacheKey := fmt.Sprintf("%s %s %d", Query.FromChannelId, Query.SearchQuery, Query.Status)
	
	Query.IgnoreFullInformation = true
	
	DB_VideoStatsCache.CleanUp()
	Stats := &DB_VideoListStats{}
	StatsC, CacheExists := DB_VideoStatsCache.Get(CacheKey)
	if CacheExists {
		Stats = StatsC.(*DB_VideoListStats)
	} else {
		VideosListStats, err := DB_ListVideos(-1, 0, Query)
		if err != nil {
			L_Printf("Failed to get videos list for stats... Err: %v\n", err)
		}
		
		Stats = &DB_VideoListStats{
			Total: len(VideosListStats),
		}
		for _, Video := range(VideosListStats) {
			switch Video.Status {
				case VIDEO_STATUS_QUEUED:      Stats.TotalQueued++
				case VIDEO_STATUS_DOWNLOADING: Stats.TotalDownloading++
				case VIDEO_STATUS_DOWNLOADED:  Stats.TotalDownloaded++
				case VIDEO_STATUS_FAILED:      Stats.TotalFailed++
				case VIDEO_STATUS_IGNORED:     Stats.TotalIgnored++
			}
		}
		
		DB_VideoStatsCache.Set(CacheKey, Stats)
	}
	
	return Stats, nil
}

func DB_IncrementVideoRevisionNumber(VideoId string) (int, error) {
	var RevisionNumber int
	
	Row := GDB.QueryRow(`
	UPDATE Videos
	SET HistoryRevisionCount = HistoryRevisionCount + 1
	WHERE Id = ?
	RETURNING HistoryRevisionCount;
	`, VideoId)
	
	err := Row.Scan(&RevisionNumber)
	if err != nil {
		return 0, fmt.Errorf("Failed to increment HistoryRevisionCount for video: %s error: %v", VideoId, err)
	}
	
	return RevisionNumber-1, nil
}

func DB_AddVideoHistoryPoint(HistoryPoint *VideoInfoHistory) error {
	VideoId := HistoryPoint.Id
	RevisionNumber, err := DB_IncrementVideoRevisionNumber(VideoId)
	if err != nil {
		return err
	}
	HistoryPoint.RevisionNumber = RevisionNumber
	
	Result, err := GDB.Exec(`
	INSERT INTO VideoHistory(Revision, Id, Title, Description, Availability, Url, Thumbnail, StoredThumbnail, Duration, UploaderUrl, UploaderName, VideoType, AddedAt, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, HistoryPoint.RevisionNumber, HistoryPoint.Id, HistoryPoint.Title, HistoryPoint.Description, HistoryPoint.Availability, HistoryPoint.Url, HistoryPoint.OriginThumbnail, HistoryPoint.Thumbnail, HistoryPoint.Duration, HistoryPoint.UploaderUrl, HistoryPoint.UploaderName, HistoryPoint.VideoType, HistoryPoint.AddedAt, HistoryPoint.UpdatedAt)
	if err != nil {
		return err
	}
	
	HId, err := Result.LastInsertId()
	if err != nil {
		return fmt.Errorf("Could not get LastInsertId wat, error: %v", err)
	}
	HistoryPoint.HId = int(HId)
	
	return nil
}


func DB_UpdateCommandTaskInfo(Task *CommandTask) error {
	Task.Lock.Lock()
	defer Task.Lock.Unlock()
	
	Output := Task.Output
	if len(Output) > MAX_TASK_OUTPUT_LOG+100 {
		Output = TruncateOutput(Output)
	}
	Status := Task.Status
	
	_, err := GDB.Exec(`
	INSERT INTO CommandTasks(Id, Title, Type, Status, FromChannel, FromVideo, RunArgs, Output, StartTime, EndTime, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Type=excluded.Type,
	Title=excluded.Title,
	Status=excluded.Status,
	
	FromChannel=excluded.FromChannel,
	FromVideo=excluded.FromVideo,
	
	RunArgs=excluded.RunArgs,
	Output=excluded.Output,
	
	StartTime=excluded.StartTime,
	EndTime=excluded.EndTime,
	UpdatedAt=excluded.UpdatedAt
	`, Task.Id, Task.Title, Task.Type, Status, Task.FromChannelId, Task.FromVideoId, Task.RunArgs, Output, Task.StartTime, Task.EndTime, time.Now().UTC())
	
	if err != nil {
		L_Printf("DB_UpdateCommandTaskInfo ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_PopulateCommandTaskInfo(Task *CommandTask) {
	Task.Lock.Lock()
	defer Task.Lock.Unlock()
	if Task.FromChannelId != "" {
		ChannelInfo := &TaskChannelInfo{}
		AChannel := GetArchiveChannelFromId(&G_ArchiveChannels, Task.FromChannelId)
		if AChannel != nil {
			ChannelInfo.Name = AChannel.Name
			ChannelInfo.Url  = AChannel.Url
			ChannelInfo.Id   = AChannel.Id
		}
		
		Task.ChannelInfo = ChannelInfo
	}
	
	if Task.FromVideoId != "" {
		VideoInfo := &TaskVideoInfo{}
		Video, err := DB_GetVideo(Task.FromVideoId)
		if err == nil && Video != nil {
			VideoInfo.Title = Video.Title
			VideoInfo.Url   = Video.Url
			VideoInfo.Id    = Video.Id
			
			Task.VideoInfo = VideoInfo
		}
	}
}

func DB_GetCommandTask(TaskId string) (*CommandTask, error) {
	CommandTask := &CommandTask{
		Lock: &sync.RWMutex{},
	}
	TaskRow := GDB.QueryRow(`
	SELECT Id, Title, Type, Status, FromChannel, FromVideo, RunArgs, Output,
	StartTime, EndTime, UpdatedAt FROM CommandTasks WHERE Id = ?
	`, TaskId)
	
	err := TaskRow.Scan(
		&CommandTask.Id,
		&CommandTask.Title,
		&CommandTask.Type,
		&CommandTask.Status,
		
		&CommandTask.FromChannelId,
		&CommandTask.FromVideoId,
		
		&CommandTask.RunArgs,
		&CommandTask.Output,
		
		&CommandTask.StartTime,
		&CommandTask.EndTime,
		&CommandTask.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	DB_PopulateCommandTaskInfo(CommandTask)
	
	return CommandTask, nil
}
func DB_GetCommandTaskOutput(TaskId string) (string, error) {
	TaskRow := GDB.QueryRow(`
	SELECT Output FROM CommandTasks WHERE Id = ?
	`, TaskId)
	
	var Output string
	
	err := TaskRow.Scan(&Output)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	
	return Output, nil
}

const (
	DB_CTASK_ORDERBY_StartTime = 0
	DB_CTASK_ORDERBY_EndTime   = 1
	//DB_CTASK_ORDERBY_UpdatedAt = 2
)

type ListCommandTasksQuery struct {
	Status int
	Type int
	FromChannelId string
	FromVideoId string
	SearchQuery string
	
	OrderBy int
	OrderDirection int
}

func DB_ConstructQuery_ListCommandTasks(Limit int, Offset int, Query ListCommandTasksQuery, Statement *string, Args *[]interface{}) {
	WhereAdded := false
	if Query.Status == -2 && Query.Type == -1 && Query.FromVideoId == "" && Query.FromChannelId == "" {
		if !WhereAdded {
			WhereAdded = true
			*Statement += " WHERE "
		}
		*Statement += " ((Status = 0 OR Status = 1 OR Status = 3) OR Type = 2) "
	}
	
	if Query.Status >= 0 || Query.Type != -1 || Query.FromChannelId != "" || Query.FromVideoId != "" {
		AddAnd := false
		if !WhereAdded {
			WhereAdded = true
			*Statement += " WHERE "
		} else if WhereAdded {
			AddAnd = true
		}
		if Query.Status >= 0 {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " Status = ?"
			*Args = append(*Args, Query.Status)
			AddAnd = true
		}
		if Query.Type != -1 {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " Type = ?"
			*Args = append(*Args, Query.Type)
			AddAnd = true
		}
		if Query.FromChannelId != "" {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " FromChannel = ?"
			*Args = append(*Args, Query.FromChannelId)
			AddAnd = true
		}
		if Query.FromVideoId != "" {
			if AddAnd {
				*Statement += " AND "
			}
			*Statement += " FromVideo = ?"
			*Args = append(*Args, Query.FromVideoId)
			AddAnd = true
		}
	}
	
	OrderBy := "EndTime"
	switch Query.OrderBy {
	case DB_CTASK_ORDERBY_StartTime:
		OrderBy = "StartTime"
	}
	
	OrderDirection := "DESC"
	if Query.OrderDirection != 0 {
		OrderDirection = "ASC"
		if Query.OrderDirection == -1 {
			OrderDirection = "DESC"
		}
	}
	
	*Statement += fmt.Sprintf(" ORDER BY %s %s LIMIT ? OFFSET ?", OrderBy, OrderDirection)
	*Args = append(*Args, Limit, Offset)
	//L_Printf("Statement: %s\n Args: %+v\n", *Statement, *Args)
}

func DB_ListCommandTasks(Limit int, Offset int, Query ListCommandTasksQuery) ([]*CommandTask, error) {
	Args := []interface{}{}
	
	Statement := `
	SELECT Id, Title, Type, Status, FromChannel, FromVideo, RunArgs,
	StartTime, EndTime, UpdatedAt FROM CommandTasks`
	
	DB_ConstructQuery_ListCommandTasks(Limit, Offset, Query, &Statement, &Args)
	
	Rows, err := GDB.Query(Statement, Args...)
	if err != nil {
		return nil, err
	}
	defer Rows.Close()
	
	TasksList := []*CommandTask{}
	for Rows.Next() {
		CommandTask := &CommandTask{
			Lock: &sync.RWMutex{},
		}
		err := Rows.Scan(
			&CommandTask.Id,
			&CommandTask.Title,
			&CommandTask.Type,
			&CommandTask.Status,
			&CommandTask.FromChannelId,
			&CommandTask.FromVideoId,
			
			&CommandTask.RunArgs,
			
			&CommandTask.StartTime,
			&CommandTask.EndTime,
			&CommandTask.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		DB_PopulateCommandTaskInfo(CommandTask)
		
		TasksList = append(TasksList, CommandTask)
	}
	
	return TasksList, nil
}

func DB_DeleteCommandTask(TaskId string) error {
	_, err := GDB.Exec(`
	DELETE FROM CommandTasks WHERE Id = ?
	`, TaskId)
	return err
}

func DB_UpdateImage(Image *DB_Image) error {
	_, err := GDB.Exec(`
	INSERT INTO Images(Id, Sha256Hash, Filename, Type, OriginUrl, AddedAt, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Sha256Hash=excluded.Sha256Hash,
	Filename=excluded.Filename,
	Type=excluded.Type,
	OriginUrl=excluded.OriginUrl,
	
	UpdatedAt=excluded.UpdatedAt
	`, Image.Id, Image.Sha256Hash, Image.Filename, Image.Type, Image.OriginUrl, time.Now().UTC(), time.Now().UTC())
	
	if err != nil {
		L_Printf("DB_UpdateImage ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_GetImageInfo(ImageId string) (*DB_Image, error) {
	ImageInfo := &DB_Image{}
	VideoRow := GDB.QueryRow(`
	SELECT Id, Sha256Hash, Filename, Type, OriginUrl, AddedAt, UpdatedAt FROM Images WHERE Id = ?
	`, ImageId)
	err := VideoRow.Scan(
		&ImageInfo.Id,
		&ImageInfo.Sha256Hash,
		&ImageInfo.Filename,
		
		&ImageInfo.Type,
		&ImageInfo.OriginUrl,
		
		&ImageInfo.AddedAt,
		&ImageInfo.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	
	return ImageInfo, nil
}
func DB_GetImageData(ImageId string) ([]byte, error) {
	var ImageData []byte
	
	VideoRow := GDB.QueryRow(`
	SELECT ImageData FROM Images WHERE Id = ?
	`, ImageId)
	err := VideoRow.Scan(
		&ImageData,
	)
	if err != nil {
		return nil, err
	}
	
	return ImageData, nil
}

func DB_SetImageData(Image *DB_Image, ImageContent []byte) error {
	RawHash := sha3.Sum256(ImageContent)
	Sha256ImageHash := base64.RawURLEncoding.EncodeToString(RawHash[0:32])
	
	Image.Sha256Hash = Sha256ImageHash
	
	_, err := GDB.Exec(`
	UPDATE Images SET ImageData = ?, Sha256Hash = ?, UpdatedAt = ? WHERE Id = ?
	`, ImageContent, Sha256ImageHash, time.Now().UTC(), Image.Id)
	
	if err != nil {
		L_Printf("DB_SetImageData ERR: %v\n", err)
		return err
	}
	
	return nil
}

func OpenDB() error {
	DatabaseFilePath := DATABASE_FILE
	if APPLICATION_VERSION_TYPE == "debug" {
		DatabaseFilePath = DATABASE_FILE_DEBUG
	}
	
	db, err := sql.Open("sqlite3", DatabaseFilePath)
	if err != nil {
		return fmt.Errorf("Failed to open database '%s' Error: %v\n", DatabaseFilePath, err)
	}
	
	DatabaseUpgrades := []string{
		"ALTER TABLE Videos ADD COLUMN QueuedAction INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE ArchiveChannels ADD COLUMN PlaylistEnd INTEGER NOT NULL default 20",
		
		"ALTER TABLE Videos ADD COLUMN PlaylistEnd INTEGER NOT NULL default 20",
		
		// v0.12
		"ALTER TABLE Videos ADD COLUMN UploaderName TEXT DEFAULT ''",
		"ALTER TABLE Videos ADD COLUMN UploaderUrl  TEXT DEFAULT ''",
		
		// v0.14
		"ALTER TABLE Videos ADD COLUMN FileSize     INTEGER DEFAULT 0",
		"ALTER TABLE Videos ADD COLUMN StoredThumbnail TEXT DEFAULT ''",
		
		// v0.20
		"ALTER TABLE Videos ADD COLUMN StreamedDirectory TEXT DEFAULT ''",
		"ALTER TABLE Videos ADD COLUMN HistoryRevisionCount INTEGER DEFAULT 0",
		
		"ALTER TABLE ArchiveChannels ADD COLUMN PreferredVideoFormat TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE ArchiveChannels ADD COLUMN PreferredAudioFormat TEXT NOT NULL DEFAULT ''",
		
		"ALTER TABLE Images ADD COLUMN Sha256Hash TEXT NOT NULL DEFAULT ''",
		
		// v0.21
	}
	
	_, err = db.Exec(db_SQL_Header)
	if err != nil {
		return fmt.Errorf("Failed run database header '%s' Error: %v\n", DatabaseFilePath, err)
	}
	
	for i, Upgrade := range(DatabaseUpgrades) {
		_, err = db.Exec(Upgrade)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			L_Printf("Upgrade[%d] failed, error: %v\n", i, err)
		}
	}
	
	GDB = db
	
	return nil
}

func DB_Close() {
	if GDB != nil {
		//L_Printf("Closing database...\n")
		err := GDB.Close()
		if err != nil {
			L_Printf("Failed to close database... Error: %v\n", err)
		} else {
			//L_Printf("Database closed successfully.\n")
		}
	}
}

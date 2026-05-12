package main

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const DATABASE_FILE = "yt_download_manager.db"
const DATABASE_FILE_DEBUG = "yt_download_manager_DEBUG.db"

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
	QualitySelect     INTEGER NOT NULL,
	CheckInterval     INTEGER NOT NULL,
	FullCheckInterval INTEGER NOT NULL default 172800,
	
	Type    INTEGER NOT NULL,
	Enabled BOOLEAN,
	
	CreatedAt DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt DATETIME
);

CREATE TABLE IF NOT EXISTS Videos (
	Id           TEXT PRIMARY KEY,
	FromChannel  TEXT NOT NULL,
	Title        TEXT NOT NULL,
	Url          TEXT NOT NULL,
	Availability TEXT NOT NULL,
	Filename     TEXT DEFAULT '',
	Resolution   TEXT DEFAULT '',
	Duration     FLOAT DEFAULT 0,
	
	RefreshState INTEGER NOT NULL DEFAULT 0,
	Status       INTEGER NOT NULL DEFAULT 0,
	VideoType    INTEGER NOT NULL DEFAULT 0,
	
	ReleaseDate BIGINT NOT NULL,
	
	AddedAt     DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt   DATETIME
);

CREATE TABLE IF NOT EXISTS CommandTasks (
	Id     TEXT PRIMARY KEY,
	Type   INTEGER NOT NULL DEFAULT 0,
	Status INTEGER NOT NULL DEFAULT 0,
	
	FromChannel TEXT,
	FromVideo   TEXT,
	
	RunArgs TEXT NOT NULL,
	Output  TEXT NOT NULL,
	
	StartTime DATETIME NOT NULL DEFAULT (datetime('now')),
	EndTime   DATETIME NOT NULL DEFAULT (datetime('now'))
);

/*
	Set all videos that were previously "downloading" videos to queued.
*/
UPDATE Videos
SET Status = 0
WHERE Status = 1;

`

var GDB *sql.DB
var VideoDBLock sync.RWMutex

func DB_UpdateArchiveChannel(AChannel *ArchiveChannel) error {
	AChannel.Lock.RLock()
	defer AChannel.Lock.RUnlock()
	TimeNow := time.Now().UTC()
	_, err := GDB.Exec(`
	INSERT OR REPLACE INTO ArchiveChannels(Id, Name, Url, DownloadDir, OutputTemplate, QualitySelect, CheckInterval, FullCheckInterval, Type, Enabled, UpdatedAt, CreatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Name=excluded.Name,
	Url=excluded.Url,
	DownloadDir=excluded.DownloadDir,
	OutputTemplate=excluded.OutputTemplate,
	QualitySelect=excluded.QualitySelect,
	CheckInterval=excluded.CheckInterval,
	FullCheckInterval=excluded.FullCheckInterval,
	Type=excluded.Type,
	Enabled=excluded.Enabled,
	UpdatedAt=excluded.UpdatedAt
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.OutputTemplate, AChannel.QualitySelect, AChannel.CheckInterval, AChannel.FullCheckInterval, AChannel.Type, AChannel.Enabled, TimeNow, TimeNow)
	
	if err != nil {
		fmt.Printf("DB_UpdateArchiveChannel ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_RemoveChannel(ChannelId string) error {
	_, err := GDB.Exec(`
	DELETE FROM ArchiveChannels WHERE Id = ?
	`, ChannelId)
	if err != nil {
		fmt.Printf("DB_RemoveChannel ERR: %v\n", err)
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
	DownloadDir, OutputTemplate, QualitySelect, CheckInterval, FullCheckInterval,
	Type,
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
			&Channel.DownloadDir, &Channel.OutputTemplate, &Channel.QualitySelect, &Channel.CheckInterval, &Channel.FullCheckInterval,
			&Channel.Type,
			&Channel.Enabled)
		if err != nil {
			return nil, err
		}
		
		ChannelsList = append(ChannelsList, Channel)
	}
	
	return ChannelsList, nil
}

func DB_LoadChannels(WD *WatchingBundle) error {
	ChannelsList, err := DB_ListChannels("")
	if err != nil {
		return err
	}
	WD.ChannelsLock.Lock()
	defer WD.ChannelsLock.Unlock()
	var i int64
	for _, Channel := range(ChannelsList) {
		Channel.NextFullChannelCheckMSEC = time.Now().UnixMilli() + (i*1000 * 60*2)
		
		WD.Channels = append(WD.Channels, Channel)
		
		i += 1
	}
	
	return nil
}

func DB_UpdateVideoInfo(Video *VideoInfo) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	TimeNow := time.Now().UTC()
	_, err := GDB.Exec(`
	INSERT INTO Videos(Id, FromChannel, Title, Url, Availability, Resolution, Filename, ReleaseDate, Duration, VideoType, UpdatedAt, AddedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	FromChannel=excluded.FromChannel,
	Title=excluded.Title,
	Url=excluded.Url,
	Availability=excluded.Availability,
	Resolution=excluded.Resolution,
	Filename=excluded.Filename,
	
	ReleaseDate=excluded.ReleaseDate,
	Duration=excluded.Duration,
	VideoType=excluded.VideoType,
	UpdatedAt=excluded.UpdatedAt
	`, Video.Id, Video.FromChannel, Video.Title, Video.Url, Video.Availability, Video.Resolution, Video.Filename, Video.ReleaseDate, Video.Duration, Video.VideoType, TimeNow, TimeNow)
	
	if err != nil {
		fmt.Printf("DB_UpdateVideoInfo ERR: %v\n", err)
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

func DB_UpdateVideoAvalibility(Video *VideoInfo, Availability string) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.Availability = Availability
	_, err := GDB.Exec(`
	UPDATE Videos SET Availability = ?, UpdatedAt = ? WHERE Id = ?
	`, Availability, time.Now().UTC(), Video.Id)
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
		fmt.Printf("DB_UpdateVideoRefreshState err: %v\n", err)
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
	SELECT FromChannel, Id, Title, Url, Availability, Resolution, Filename, Status, ReleaseDate, Duration, VideoType,
	AddedAt, UpdatedAt, RefreshState FROM Videos WHERE Id = ?
	`, VideoId)
	err := VideoRow.Scan(
		&VideoInfo.FromChannel,
		&VideoInfo.Id,
		&VideoInfo.Title,
		&VideoInfo.Url,
		&VideoInfo.Availability,
		&VideoInfo.Resolution,
		&VideoInfo.Filename,
		
		&VideoInfo.Status,
		&VideoInfo.ReleaseDate,
		&VideoInfo.Duration,
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
	RefreshState int
	
	OrderBy int
	OrderDirection int
}

func DB_ListVideos(Limit int, Offset int, Query ListVideosQuery) ([]*VideoInfo, error) {
	Args := []interface{}{}
	
	// ORDER BY ReleaseDate DESC
	Statement := `
	SELECT FromChannel, Id, Title, Url, Availability, Resolution, Filename, Status, ReleaseDate, Duration, VideoType,
	AddedAt, UpdatedAt, RefreshState FROM Videos`
	if Query.Status != -1 || Query.FromChannelId != "" || Query.RefreshState != -1 {
		Statement += " WHERE "
		AddAnd := false
		if Query.Status != -1 {
			Statement += " Status = ?"
			Args = append(Args, Query.Status)
			AddAnd = true
		}
		if Query.FromChannelId != "" {
			if AddAnd {
				Statement += " AND "
			}
			Statement += " FromChannel = ?"
			Args = append(Args, Query.FromChannelId)
			AddAnd = true
		}
		if Query.RefreshState != -1 {
			if AddAnd {
				Statement += " AND "
			}
			Statement += " RefreshState = ?"
			Args = append(Args, Query.RefreshState)
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
	
	Statement = Statement + fmt.Sprintf(" ORDER BY %s %s LIMIT ? OFFSET ?", OrderBy, OrderDirection)
	QLimit := Limit
	QOffset := Offset
	
	IsSearching := false
	var SearchWords []string
	if Query.SearchQuery != "" {
		QLimit = -1
		QOffset = 0
		IsSearching = true
		SearchWords = strings.Split(Query.SearchQuery, " ")
		for i := 0; i < len(SearchWords); i++ {
			SearchWords[i] = strings.ToLower(SearchWords[i])
		}
	}
	Args = append(Args, QLimit, QOffset)
	Rows, err := GDB.Query(Statement, Args...)
	if err != nil {
		return nil, err
	}
	defer Rows.Close()
	
	VideosList := []*VideoInfo{}
	
	Si := 0
	
	for Rows.Next() {
		VideoInfo := &VideoInfo{}
		err := Rows.Scan(
			&VideoInfo.FromChannel,
			&VideoInfo.Id,
			&VideoInfo.Title,
			&VideoInfo.Url,
			&VideoInfo.Availability,
			&VideoInfo.Resolution,
			&VideoInfo.Filename,
			
			&VideoInfo.Status,
			&VideoInfo.ReleaseDate,
			&VideoInfo.Duration,
			&VideoInfo.VideoType,
			
			&VideoInfo.AddedAt,
			&VideoInfo.UpdatedAt,
			&VideoInfo.RefreshState,
		)
		if err != nil {
			return nil, err
		}
		
		if IsSearching {
			IsWhatWeAreLookingFor := true
			
			// Simple search!
			TitleLowercase := strings.ToLower(VideoInfo.Title)
			
			for _, Word := range(SearchWords) {
				if !strings.Contains(TitleLowercase, Word) &&
				   !strings.Contains(strings.ToLower(VideoInfo.Availability), Word) &&
				   !strings.Contains(strings.ToLower(VideoInfo.Id), Word) {
					// This video is NOT what we are looking for!
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
		
		VideosList = append(VideosList, VideoInfo)
		if IsSearching {
			if Limit != -1 && len(VideosList) >= Limit {
				break
			}
		}
	}
	
	return VideosList, nil
}

func DB_UpdateCommandTaskInfo(Task *CommandTask) error {
	Task.Lock.Lock()
	defer Task.Lock.Unlock()
	
	Output := Task.Output
	if len(Output) > MAX_TASK_OUTPUT_LOG+100 {
		Output = TruncateOutput(Output)
	}
	Status := Task.Status
	if Status == TASK_STATUS_RUNNING {
		// Don't save the running status incase the program abruptly quits!
		Status = TASK_STATUS_FAILED
	}
	
	_, err := GDB.Exec(`
	INSERT INTO CommandTasks(Id, Type, Status, FromChannel, FromVideo, RunArgs, Output, StartTime, EndTime)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Type=excluded.Type,
	Status=excluded.Status,
	
	FromChannel=excluded.FromChannel,
	FromVideo=excluded.FromVideo,
	
	RunArgs=excluded.RunArgs,
	Output=excluded.Output,
	
	StartTime=excluded.StartTime,
	EndTime=excluded.EndTime
	`, Task.Id, Task.Type, Status, Task.FromChannelId, Task.FromVideoId, Task.RunArgs, Output, Task.StartTime, Task.EndTime)
	
	if err != nil {
		fmt.Printf("DB_UpdateCommandTaskInfo ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_GetCommandTask(TaskId string) (*CommandTask, error) {
	CommandTask := &CommandTask{}
	TaskRow := GDB.QueryRow(`
	SELECT Id, Type, Status, FromChannel, FromVideo, RunArgs, Output,
	StartTime, EndTime FROM CommandTasks WHERE Id = ?
	`, TaskId)
	
	err := TaskRow.Scan(
		&CommandTask.Id,
		&CommandTask.Type,
		&CommandTask.Status,
		
		&CommandTask.FromChannelId,
		&CommandTask.FromVideoId,
		
		&CommandTask.RunArgs,
		&CommandTask.Output,
		&CommandTask.StartTime,
		&CommandTask.EndTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	
	return CommandTask, nil
}

func DB_ListCommandTasks(Limit int, Offset int) ([]*CommandTask, error) {
	Args := []interface{}{}
	
	Statement := `
	SELECT Id, Type, Status, FromChannel, FromVideo, RunArgs, Output,
	StartTime, EndTime FROM CommandTasks`
	
	Statement = Statement + " ORDER BY EndTime DESC LIMIT ? OFFSET ?"
	Args = append(Args, Limit, Offset)
	
	Rows, err := GDB.Query(Statement, Args...)
	if err != nil {
		return nil, err
	}
	defer Rows.Close()
	
	TasksList := []*CommandTask{}
	for Rows.Next() {
		CommandTask := &CommandTask{}
		err := Rows.Scan(
			&CommandTask.Id,
			&CommandTask.Type,
			&CommandTask.Status,
			&CommandTask.FromChannelId,
			&CommandTask.FromVideoId,
			
			&CommandTask.RunArgs,
			&CommandTask.Output,
			
			&CommandTask.StartTime,
			&CommandTask.EndTime,
		)
		if err != nil {
			return nil, err
		}
		
		TasksList = append(TasksList, CommandTask)
	}
	
	return TasksList, nil
}

func OpenDB() error {
	DatabaseFilePath := DATABASE_FILE
	if APPLICATION_VERSION == "debug" {
		DatabaseFilePath = DATABASE_FILE_DEBUG
	}
	
	db, err := sql.Open("sqlite3", DatabaseFilePath)
	if err != nil {
		return fmt.Errorf("Failed to open database '%s' Error: %v\n", DatabaseFilePath, err)
	}
	
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN VideoType INTEGER DEFAULT 0")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN RefreshState INTEGER NOT NULL DEFAULT 0")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN Availability TEXT NOT NULL DEFAULT ''")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN Resolution TEXT DEFAULT ''")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN Filename TEXT DEFAULT ''")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	
	_, err = db.Exec(db_SQL_Header)
	if err != nil {
		return fmt.Errorf("Failed run database header '%s' Error: %v\n", DatabaseFilePath, err)
	}
	
	GDB = db
	
	return nil
}

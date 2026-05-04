package main

import (
	"database/sql"
	"fmt"
	"time"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const DATABASE_FILE = "yt_download_manager.db"

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
	Id    TEXT PRIMARY KEY,
	FromChannel TEXT NOT NULL,
	Title TEXT NOT NULL,
	Url   TEXT NOT NULL,
	Duration FLOAT,
	
	Status    INTEGER NOT NULL DEFAULT 0,
	VideoType INTEGER NOT NULL DEFAULT 0,
	
	ReleaseDate BIGINT NOT NULL,
	
	AddedAt     DATETIME NOT NULL DEFAULT (datetime('now')),
	UpdatedAt   DATETIME
	
);

/*
	Set all videos that were previously "downloading" videos to queued.
*/
UPDATE Videos
SET Status = 0
WHERE Status = 1;

`

const (
	VIDEO_STATUS_QUEUED      = 0
	VIDEO_STATUS_DOWNLOADING = 1
	VIDEO_STATUS_DOWNLOADED  = 2
	VIDEO_STATUS_FAILED      = 3
)

var GDB *sql.DB
var VideoDBLock sync.RWMutex

func DB_UpdateArchiveChannel(AChannel *ArchiveChannel) error {
	_, err := GDB.Exec(`
	INSERT OR REPLACE INTO ArchiveChannels(Id, Name, Url, DownloadDir, OutputTemplate, QualitySelect, CheckInterval, FullCheckInterval, Type, Enabled, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
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
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.OutputTemplate, AChannel.QualitySelect, AChannel.CheckInterval, AChannel.FullCheckInterval, AChannel.Type, AChannel.Enabled, time.Now().UTC())
	
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
	var ChannelsList []*ArchiveChannel
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
	_, err := GDB.Exec(`
	INSERT INTO Videos(Id, FromChannel, Title, Url, ReleaseDate, Duration, VideoType, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	FromChannel=excluded.FromChannel,
	Title=excluded.Title,
	Url=excluded.Url,
	ReleaseDate=excluded.ReleaseDate,
	Duration=excluded.Duration,
	VideoType=excluded.VideoType,
	UpdatedAt=excluded.UpdatedAt
	`, Video.Id, Video.FromChannel, Video.Title, Video.Url, Video.ReleaseDate, Video.Duration, Video.VideoType, time.Now().UTC())
	
	if err != nil {
		fmt.Printf("DB_UpdateVideoInfo ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_UpdateVideoStatus(Video *VideoInfo, Status int) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	Video.Status = Status
	_, err := GDB.Exec(`
	UPDATE Videos SET Status = ?, UpdatedAt = ? WHERE Id = ?
	`, Status, time.Now().UTC(), Video.Id)
	return err
}

func DB_GetVideo(VideoId string) (*VideoInfo, error) {
	VideoDBLock.RLock()
	defer VideoDBLock.RUnlock()
	VideoInfo := &VideoInfo{}
	VideoRow := GDB.QueryRow(`
	SELECT FromChannel, Id, Title, Url, Status, ReleaseDate, Duration, VideoType FROM Videos WHERE Id = ?
	`, VideoId)
	err := VideoRow.Scan(&VideoInfo.FromChannel, &VideoInfo.Id, &VideoInfo.Title, &VideoInfo.Url, &VideoInfo.Status, &VideoInfo.ReleaseDate, &VideoInfo.Duration, &VideoInfo.VideoType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	
	return VideoInfo, nil
}

func DB_ListVideos(Limit int, Offset int, Status int, FromChannel string) ([]*VideoInfo, error) {
	Args := []interface{}{}
	
	// ORDER BY ReleaseDate DESC
	Statement := "SELECT FromChannel, Id, Title, Url, Status, ReleaseDate, Duration, VideoType FROM Videos"
	if Status != -1 || FromChannel != "" {
		Statement += " WHERE "
		AddAnd := false
		if Status != -1 {
			Statement += " Status = ?"
			Args = append(Args, Status)
			AddAnd = true
		}
		if FromChannel != "" {
			if AddAnd {
				Statement += " AND "
			}
			Statement += " FromChannel = ?"
			Args = append(Args, FromChannel)
			AddAnd = true
		}
	}
	
	Statement = Statement + " ORDER BY AddedAt DESC LIMIT ? OFFSET ?"
	Args = append(Args, Limit, Offset)
	Rows, err := GDB.Query(Statement, Args...)
	if err != nil {
		return nil, err
	}
	
	VideosList := []*VideoInfo{}
	
	for Rows.Next() {
		VideoInfo := &VideoInfo{}
		err := Rows.Scan(
			&VideoInfo.FromChannel,
			&VideoInfo.Id,
			&VideoInfo.Title,
			&VideoInfo.Url,
			&VideoInfo.Status,
			&VideoInfo.ReleaseDate,
			&VideoInfo.Duration,
			&VideoInfo.VideoType,
		)
		if err != nil {
			return nil, err
		}
		
		VideosList = append(VideosList, VideoInfo)
	}
	
	return VideosList, nil
}

func OpenDB() error {
	db, err := sql.Open("sqlite3", DATABASE_FILE)
	if err != nil {
		return err
	}
	
	_, err = db.Exec("ALTER TABLE Videos ADD COLUMN VideoType INTEGER DEFAULT 0")
	if err != nil {
	    // Column might already exist, handle accordingly
	}
	
	_, err = db.Exec(db_SQL_Header)
	if err != nil {
		return err
	}
	
	GDB = db
	
	return nil
}

package main

import (
	"database/sql"
	"fmt"
	"time"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const db_SQL_Header = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA secure_delete = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = normal;

CREATE TABLE IF NOT EXISTS ArchiveChannels (
	Id             TEXT PRIMARY KEY UNIQUE,
	Name           TEXT NOT NULL,
	Url            TEXT NOT NULL,
	DownloadDir    TEXT NOT NULL,
	OutputTemplate TEXT NOT NULL,
	QualitySelect  INTEGER NOT NULL,
	Type           INTEGER NOT NULL,
	CheckInterval  INTEGER NOT NULL,
	Enabled BOOLEAN,
	
	CreatedAt BIGINT NOT NULL DEFAULT (unixepoch()),
	UpdatedAt BIGINT
);

CREATE TABLE IF NOT EXISTS Videos (
	Id    TEXT PRIMARY KEY,
	FromChannel TEXT NOT NULL,
	Title TEXT NOT NULL,
	Url   TEXT NOT NULL,
	Duration FLOAT,
	
	Status INTEGER NOT NULL DEFAULT 0,
	
	ReleaseDate BIGINT NOT NULL,
	UpdatedAt   BIGINT
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
	INSERT OR REPLACE INTO ArchiveChannels(Id, Name, Url, DownloadDir, OutputTemplate, QualitySelect, Type, CheckInterval, Enabled, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	Name=excluded.Name,
	Url=excluded.Url,
	DownloadDir=excluded.DownloadDir,
	QualitySelect=excluded.QualitySelect,
	Type=excluded.Type,
	CheckInterval=excluded.CheckInterval,
	Enabled=excluded.Enabled,
	UpdatedAt=excluded.UpdatedAt
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.OutputTemplate, AChannel.QualitySelect, AChannel.Type, AChannel.CheckInterval, AChannel.Enabled, time.Now().UTC().Unix())
	
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

func DB_UpdateVideoInfo(Video *VideoInfo) error {
	VideoDBLock.Lock()
	defer VideoDBLock.Unlock()
	_, err := GDB.Exec(`
	INSERT INTO Videos(Id, FromChannel, Title, Url, ReleaseDate, Duration, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET
	FromChannel=excluded.FromChannel,
	Title=excluded.Title,
	Url=excluded.Url,
	ReleaseDate=excluded.ReleaseDate,
	Duration=excluded.Duration,
	UpdatedAt=excluded.UpdatedAt
	`, Video.Id, Video.FromChannel, Video.Title, Video.Url, Video.ReleaseDate, Video.Duration, time.Now().UTC().Unix())
	
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
	`, Status, time.Now().UTC().Unix(), Video.Id)
	return err
}

func DB_GetVideo(VideoId string) (*VideoInfo, error) {
	VideoDBLock.RLock()
	defer VideoDBLock.RUnlock()
	VideoInfo := &VideoInfo{}
	VideoRow := GDB.QueryRow(`
	SELECT FromChannel, Id, Title, Url, Status, ReleaseDate, Duration FROM Videos WHERE Id = ?
	`, VideoId)
	err := VideoRow.Scan(&VideoInfo.FromChannel, &VideoInfo.Id, &VideoInfo.Title, &VideoInfo.Url, &VideoInfo.Status, &VideoInfo.ReleaseDate, &VideoInfo.Duration)
	if err != nil {
		return nil, err
	}
	
	return VideoInfo, nil
}

func DB_LoadChannels(WD *WatchingBundle) error {
	Rows, err := GDB.Query(`SELECT Id, Name, Url, DownloadDir, OutputTemplate, QualitySelect, Type, CheckInterval, Enabled FROM ArchiveChannels ORDER BY CreatedAt DESC`)
	if err != nil {
		return err
	}
	WD.ChannelsLock.Lock()
	defer WD.ChannelsLock.Unlock()
	var i int64
	for Rows.Next() {
		Channel := &ArchiveChannel{}
		err := Rows.Scan(&Channel.Id, &Channel.Name, &Channel.Url, &Channel.DownloadDir, &Channel.OutputTemplate, &Channel.QualitySelect, &Channel.Type, &Channel.CheckInterval, &Channel.Enabled)
		if err != nil {
			return err
		}
		
		Channel.NextFullChannelCheckMSEC = time.Now().UnixMilli() + (i*1000 * 60*2)
		
		WD.Channels = append(WD.Channels, Channel)
		
		i += 1
	}
	
	return nil
}

func OpenDB() error {
	db, err := sql.Open("sqlite3", "yt_live_manager.db")
	if err != nil {
		return err
	}
	
	_, err = db.Exec(db_SQL_Header)
	if err != nil {
		return err
	}
	
	GDB = db
	
	return nil
}

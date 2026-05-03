package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const db_SQL_Header = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA secure_delete = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = normal;

CREATE TABLE IF NOT EXISTS ArchiveChannels (
	Id   TEXT PRIMARY KEY UNIQUE,
	Name TEXT NOT NULL,
	Url  TEXT NOT NULL,
	DownloadDir TEXT NOT NULL,
	Type    INTEGER NOT NULL,
	Enabled INTEGER
	
	CreatedAt BIGINT NOT NULL DEFAULT (unixepoch()),
	UpdatedAt BIGINT
);

CREATE TABLE IF NOT EXISTS Videos (
	FromChannel TEXT NOT NULL,
	Id    TEXT PRIMARY KEY AUTOINCREMENT,
	Title TEXT NOT NULL,
	Url   TEXT NOT NULL,
	
	Status INTEGER NOT NULL DEFAULT 0
	
	ReleaseDate BIGINT NOT NULL
	UpdatedAt   BIGINT
);
`

const (
	VIDEO_STATUS_QUEUED      = 0
	VIDEO_STATUS_DOWNLOADING = 1
	VIDEO_STATUS_DOWNLOADED  = 2
	VIDEO_STATUS_FAILED      = 3
)

var GDB *sql.DB

func DB_UpdateArchiveChannel(AChannel *ArchiveChannel) error {
	_, err := GDB.Exec(`
	INSERT OR REPLACE INTO ArchiveChannels(Id, Name, Url, DownloadDir, Type, Enabled, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET Name=excluded.Name, Url=excluded.Url, DownloadDir=excluded.DownloadDir, Type=excluded.Type, Enabled=excluded.Enabled, UpdatedAt=excluded.UpdatedAt
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.Type, AChannel.Enabled, time.Now().UTC().Unix())
	
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
	_, err := GDB.Exec(`
	INSERT INTO Videos(Id, FromChannel, Title, Url, ReleaseDate, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(Id)
	DO UPDATE SET FromChannel=excluded.FromChannel, Title=excluded.Title, Url=excluded.Url, ReleaseDate=excluded.ReleaseDate, UpdatedAt=excluded.UpdatedAt
	`, Video.Id, Video.FromChannel, Video.Title, Video.Url, Video.ReleaseDate, time.Now().UTC().Unix())
	
	if err != nil {
		fmt.Printf("DB_UpdateVideoInfo ERR: %v\n", err)
		return err
	}
	
	return nil
}

func DB_UpdateVideoStatus(Video *VideoInfo, Status int) error {
	Video.Status = Status
	_, err := GDB.Exec(`
	UPDATE Videos SET Status = ?, UpdatedAt = ? WHERE Id = ?
	`, Status, time.Now().UTC().Unix(), Video.Id)
	return err
}

func DB_GetVideo(VideoId string) (*VideoInfo, error) {
	VideoInfo := &VideoInfo{}
	VideoRow := GDB.QueryRow(`
	SELECT FromChannel, Id, Title, Url, Status, ReleaseDate FROM Videos WHERE Id = ?
	`, VideoId)
	err := VideoRow.Scan(&VideoInfo.FromChannel, &VideoInfo.Id, &VideoInfo.Title, &VideoInfo.Url, &VideoInfo.Status, &VideoInfo.Status, &VideoInfo.ReleaseDate)
	if err != nil {
		return nil, err
	}
	
	return VideoInfo, nil
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

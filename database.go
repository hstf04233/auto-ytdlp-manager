package main

import (
	"time"
	"database/sql"
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
	Id    TEXT PRIMARY KEY AUTOINCREMENT,
	Title TEXT NOT NULL,
	Url   TEXT NOT NULL,
	
	Status INTEGER NOT NULL DEFAULT 0
	
	ReleaseDate BIGINT NOT NULL
);
`

var GDB *sql.DB

func DB_UpdateArchiveChannel(AChannel *ArchiveChannel) error {
	_, err := GDB.Exec(`
	INSERT OR REPLACE INTO ArchiveChannels(Id, Name, Url, DownloadDir, Type, Enabled, UpdatedAt)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`, AChannel.Id, AChannel.Name, AChannel.Url, AChannel.DownloadDir, AChannel.Type, AChannel.Enabled, time.Now().UTC().Unix())
	
	if err != nil {
		return err
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

// TODO:
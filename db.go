// Copyright 2009 The freegeoip authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package freegeoip

import (
	"compress/gzip"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/oschwald/maxminddb-golang"
)

var (
	// ErrUnavailable may be returned by DB.Lookup when the database
	// points to a URL and is not yet available because it's being
	// downloaded in background.
	ErrUnavailable = errors.New("no database available")

	// Local cached copy of a database downloaded from a URL.
	defaultDB = "./db.gz"

	// This URL is outdated and should only be used as fallback
	MaxMindDBURL = "https://download.db-ip.com/free/dbip-city-lite-2022-04.mmdb.gz"
)

// DB is the IP geolocation database.
type DB struct {
	file        string            // Database file name.
	checksum    string            // MD5 of the unzipped database file
	reader      *maxminddb.Reader // Actual db object.
	notifyQuit  chan struct{}     // Stop auto-update and watch goroutines.
	notifyOpen  chan string       // Notify when a db file is open.
	notifyError chan error        // Notify when an error occurs.
	notifyInfo  chan string       // Notify random actions for logging
	closed      bool              // Mark this db as closed.
	lastUpdated time.Time         // Last time the db was updated.
	mu          sync.RWMutex      // Protects all the above.
}

// Open creates and initializes a DB from a local file.
//
// The database file is monitored by fsnotify and automatically
// reloads when the file is updated or overwritten.
func Open(dsn string) (*DB, error) {
	db := &DB{
		file:        dsn,
		notifyQuit:  make(chan struct{}),
		notifyOpen:  make(chan string, 1),
		notifyError: make(chan error, 1),
		notifyInfo:  make(chan string, 1),
	}
	err := db.openFile()
	if err != nil {
		db.Close()
		return nil, err
	}
	err = db.watchFile()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("fsnotify failed for %s: %s", dsn, err)
	}
	return db, nil
}

// OpenURL creates and initializes a DB from a URL.
// It automatically downloads and updates the file in background, and
// keeps a local copy on $TMPDIR.
func OpenURL(url string) (*DB, error) {
	db := &DB{
		file:        defaultDB,
		notifyQuit:  make(chan struct{}),
		notifyOpen:  make(chan string, 1),
		notifyError: make(chan error, 1),
		notifyInfo:  make(chan string, 1),
	}
	db.openFile()
	go db.tryUpdate(url)
	err := db.watchFile()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("fsnotify failed for %s: %s", db.file, err)
	}
	return db, nil
}

func (db *DB) watchFile() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dbdir, err := db.makeDir()
	if err != nil {
		return err
	}
	go db.watchEvents(watcher)
	return watcher.Add(dbdir)
}

func (db *DB) watchEvents(watcher *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if ev.Name == db.file {
				fmt.Println("event", ev)
				if ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
					db.openFile()
				}
			}
		case <-watcher.Errors:
		case <-db.notifyQuit:
			fmt.Println("error during watching")
			watcher.Close()
			return
		}
	}
}

func (db *DB) openFile() error {
	reader, checksum, err := db.newReader(db.file)
	if err != nil {
		return err
	}
	stat, err := os.Stat(db.file)
	if err != nil {
		return err
	}
	db.setReader(reader, stat.ModTime(), checksum)
	return nil
}

func (db *DB) newReader(dbfile string) (*maxminddb.Reader, string, error) {
	f, err := os.Open(dbfile)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	gzf, err := gzip.NewReader(f)
	if err != nil {
		return nil, "", err
	}
	defer gzf.Close()
	b, err := ioutil.ReadAll(gzf)
	if err != nil {
		return nil, "", err
	}
	checksum := fmt.Sprintf("%x", md5.Sum(b))
	mmdb, err := maxminddb.FromBytes(b)
	return mmdb, checksum, err
}

func (db *DB) setReader(reader *maxminddb.Reader, modtime time.Time, checksum string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		reader.Close()
		return
	}
	if db.reader != nil {
		db.reader.Close()
	}
	db.reader = reader
	db.lastUpdated = modtime.UTC()
	db.checksum = checksum
	db.notifyOpen <- db.file
}

func (db *DB) tryUpdate(url string) {
	db.sendInfo("starting update")
	err := db.runUpdate(url)
	if err != nil {
		db.sendError(fmt.Errorf("download failed"))
	}
	db.sendInfo("finished update")
}

func (db *DB) runUpdate(url string) error {
	yes, err := db.needUpdate(url)
	if err != nil {
		return err
	}
	if !yes {
		db.sendInfo("no update needed")
		return nil
	}
	db.sendInfo(fmt.Sprintf("downloading db from %s", url))
	tmpfile, err := db.download(url)
	if err != nil {
		return err
	}
	err = db.renameFile(tmpfile)
	if err != nil {
		// Cleanup the tempfile if renaming failed.
		os.RemoveAll(tmpfile)
	}
	return err
}

func (db *DB) needUpdate(url string) (bool, error) {
	_, err := os.Stat(db.file)
	if err != nil {
		return true, nil // Local db is missing, must be downloaded.
	}

	return false, nil
}

func (db *DB) download(url string) (tmpfile string, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	tmpfile = filepath.Join(os.TempDir(),
		fmt.Sprintf("_freegeoip.%d.db.gz", time.Now().UnixNano()))
	f, err := os.Create(tmpfile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return "", err
	}
	return tmpfile, nil
}

func (db *DB) makeDir() (dbdir string, err error) {
	dbdir = filepath.Dir(db.file)
	_, err = os.Stat(dbdir)
	if err != nil {
		err = os.MkdirAll(dbdir, 0755)
		if err != nil {
			return "", err
		}
	}
	return dbdir, nil
}

func (db *DB) renameFile(name string) error {
	os.Rename(db.file, db.file+".bak") // Optional, might fail.
	_, err := db.makeDir()
	if err != nil {
		return err
	}
	return os.Rename(name, db.file)
}

// Date returns the UTC date the database file was last modified.
// If no database file has been opened the behaviour of Date is undefined.
func (db *DB) Date() time.Time {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.lastUpdated
}

// NotifyClose returns a channel that is closed when the database is closed.
func (db *DB) NotifyClose() <-chan struct{} {
	return db.notifyQuit
}

// NotifyOpen returns a channel that notifies when a new database is
// loaded or reloaded. This can be used to monitor background updates
// when the DB points to a URL.
func (db *DB) NotifyOpen() (filename <-chan string) {
	return db.notifyOpen
}

// NotifyError returns a channel that notifies when an error occurs
// while downloading or reloading a DB that points to a URL.
func (db *DB) NotifyError() (errChan <-chan error) {
	return db.notifyError
}

// NotifyInfo returns a channel that notifies informational messages
// while downloading or reloading.
func (db *DB) NotifyInfo() <-chan string {
	return db.notifyInfo
}

func (db *DB) sendError(err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return
	}
	db.notifyError <- err
}

func (db *DB) sendInfo(message string) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return
	}
	db.notifyInfo <- message
}

// Lookup performs a database lookup of the given IP address, and stores
// the response into the result value. The result value must be a struct
// with specific fields and tags as described here:
// https://godoc.org/github.com/oschwald/maxminddb-golang#Reader.Lookup
//
// See the DefaultQuery for an example of the result struct.
func (db *DB) Lookup(addr net.IP, result interface{}) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.reader != nil {
		return db.reader.Lookup(addr, result)
	}
	return ErrUnavailable
}

// DefaultQuery is the default query used for database lookups.
type DefaultQuery struct {
	Continent struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"continent"`
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Region []struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
		MetroCode uint    `maxminddb:"metro_code"`
		TimeZone  string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`
	Postal struct {
		Code string `maxminddb:"code"`
	} `maxminddb:"postal"`
}

// Close closes the database.
func (db *DB) Close() {
	db.mu.Lock()
	defer db.mu.Unlock()
	if !db.closed {
		db.closed = true
		close(db.notifyQuit)
		close(db.notifyOpen)
		close(db.notifyError)
		close(db.notifyInfo)
	}
	if db.reader != nil {
		db.reader.Close()
		db.reader = nil
	}
}

package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type UsageRecord struct {
	Count    int       `json:"count"`
	LastUsed time.Time `json:"lastUsed"`
}

type repoData map[string]UsageRecord // branch -> record
type frecencyDB map[string]repoData  // repo_root -> repoData

func dbPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wt", "frecency.json")
}

func loadDB() frecencyDB {
	data, err := os.ReadFile(dbPath())
	if os.IsNotExist(err) {
		return make(frecencyDB)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt: warning: could not read frecency db: %v\n", err)
		return make(frecencyDB)
	}
	var db frecencyDB
	if err := json.Unmarshal(data, &db); err != nil {
		fmt.Fprintf(os.Stderr, "wt: warning: frecency db is corrupt, starting fresh: %v\n", err)
		return make(frecencyDB)
	}
	return db
}

func saveDB(db frecencyDB) error {
	path := dbPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomic replacement.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func GetUsage(repoRoot string) map[string]UsageRecord {
	db := loadDB()
	if repo, ok := db[repoRoot]; ok {
		return repo
	}
	return make(map[string]UsageRecord)
}

func RecordUsage(repoRoot, branch string) error {
	db := loadDB()
	if db[repoRoot] == nil {
		db[repoRoot] = make(repoData)
	}
	r := db[repoRoot][branch]
	r.Count++
	r.LastUsed = time.Now()
	db[repoRoot][branch] = r
	return saveDB(db)
}

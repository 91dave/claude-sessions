package index

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/transcript"
)

type Options struct {
	ClaudeDir string
	CacheFile string
	Blocklist string
	Alive     func(pid int) bool
}

type Entry struct {
	transcript.Session
	ID      string
	Path    string
	ModTime time.Time
	Live    bool
	Marker  live.Marker
}

type cacheRecord struct {
	ModTime int64              `json:"modTime"`
	Size    int64              `json:"size"`
	Session transcript.Session `json:"session"`
}

type file struct {
	id, path string
	modTime  time.Time
	size     int64
}

func Load(opts Options) ([]Entry, error) {
	files, err := transcriptFiles(filepath.Join(opts.ClaudeDir, "projects"))
	if err != nil {
		return nil, err
	}
	cache := readCache(opts.CacheFile)
	fresh := parseChanged(files, cache)
	writeCache(opts.CacheFile, fresh)

	markers := live.Read(filepath.Join(opts.ClaudeDir, "sessions"), opts.Alive)
	blocked := readBlocklist(opts.Blocklist)

	var entries []Entry
	for _, f := range files {
		record, ok := fresh[f.id]
		if !ok || !record.Session.HasUser || blocked[f.id] {
			continue
		}
		marker, running := markers[f.id]
		if !record.Session.HasTurns && !running {
			continue
		}
		entries = append(entries, Entry{
			Session: record.Session,
			ID:      f.id,
			Path:    f.path,
			ModTime: f.modTime,
			Live:    running,
			Marker:  marker,
		})
	}
	return entries, nil
}

func transcriptFiles(projectsDir string) ([]file, error) {
	paths, err := filepath.Glob(filepath.Join(projectsDir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	files := make([]file, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			continue
		}
		files = append(files, file{
			id:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
			path:    path,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	return files, nil
}

func parseChanged(files []file, cache map[string]cacheRecord) map[string]cacheRecord {
	fresh := make(map[string]cacheRecord, len(files))
	var stale []file
	for _, f := range files {
		if r, ok := cache[f.id]; ok && r.ModTime == f.modTime.UnixNano() && r.Size == f.size {
			fresh[f.id] = r
		} else {
			stale = append(stale, f)
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan file)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				s, err := transcript.ParseFile(f.path)
				if err != nil {
					continue
				}
				mu.Lock()
				fresh[f.id] = cacheRecord{ModTime: f.modTime.UnixNano(), Size: f.size, Session: s}
				mu.Unlock()
			}
		}()
	}
	for _, f := range stale {
		work <- f
	}
	close(work)
	wg.Wait()
	return fresh
}

func readCache(path string) map[string]cacheRecord {
	cache := map[string]cacheRecord{}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &cache) != nil {
		return map[string]cacheRecord{}
	}
	return cache
}

func writeCache(path string, records map[string]cacheRecord) {
	data, err := json.Marshal(records)
	if err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, path)
	}
}

func readBlocklist(path string) map[string]bool {
	blocked := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return blocked
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if id := strings.TrimSpace(scanner.Text()); id != "" {
			blocked[id] = true
		}
	}
	return blocked
}

func Block(path, id string) error {
	if readBlocklist(path)[id] {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(id + "\n")
	return err
}

func Match(e Entry, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{e.Title(), e.Project(), e.Branch, e.SearchText()}, " "))
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

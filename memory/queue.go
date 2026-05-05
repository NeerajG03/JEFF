// queue.go — JEFF_HOME/queue/sessions/*.json CRUD primitives.
// SessionEnd hooks (Worker A) write entries; marlowe (C) consumes them during
// `jeff memory curate`. Processed entries move to archive/<iso-week>/.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionQueueEntry is the JSON record dropped by SessionEnd hooks.
type SessionQueueEntry struct {
	Task           string    `json:"task"`
	Persona        string    `json:"persona"`
	Repos          []string  `json:"repos"`
	TranscriptPath string    `json:"transcript_path"`
	Reason         string    `json:"reason"`
	Proposals      []string  `json:"proposals"`
	EndedAt        time.Time `json:"ended_at"`
}

// WriteQueueEntry writes e to queue/sessions/<task>-<unix>.json and returns
// the resulting absolute path.
func WriteQueueEntry(jeffHome string, e SessionQueueEntry) (string, error) {
	if e.Task == "" {
		return "", fmt.Errorf("WriteQueueEntry: task is required")
	}
	if e.EndedAt.IsZero() {
		e.EndedAt = time.Now().UTC()
	}
	dir := QueueSessionsRoot(jeffHome)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("WriteQueueEntry: mkdir: %w", err)
	}
	name := fmt.Sprintf("%s-%d.json", sanitizeTaskID(e.Task), e.EndedAt.UnixNano())
	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return "", fmt.Errorf("WriteQueueEntry: marshal: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("WriteQueueEntry: write: %w", err)
	}
	return path, nil
}

// ListQueueEntries returns all queued session entries, sorted by EndedAt asc.
func ListQueueEntries(jeffHome string) ([]SessionQueueEntry, error) {
	dir := QueueSessionsRoot(jeffHome)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ListQueueEntries: read dir: %w", err)
	}
	var out []SessionQueueEntry
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ListQueueEntries: read %s: %w", path, err)
		}
		var e SessionQueueEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("ListQueueEntries: unmarshal %s: %w", path, err)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndedAt.Before(out[j].EndedAt) })
	return out, nil
}

// ArchiveQueueEntry moves the queue file at path into archive/<iso-week>/.
// The destination week bucket is derived from the file's mtime.
func ArchiveQueueEntry(jeffHome string, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("ArchiveQueueEntry: stat: %w", err)
	}
	week := isoWeek(info.ModTime())
	destDir := filepath.Join(ArchiveRoot(jeffHome), week, "queue")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("ArchiveQueueEntry: mkdir: %w", err)
	}
	dest := filepath.Join(destDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("ArchiveQueueEntry: rename: %w", err)
	}
	return nil
}

// sanitizeTaskID makes a task ID safe for use as a filename component.
func sanitizeTaskID(s string) string {
	r := strings.NewReplacer(string(filepath.Separator), "_", " ", "_", ":", "_")
	return r.Replace(s)
}

// isoWeek returns the ISO 8601 year-week of t, formatted YYYY-Www.
func isoWeek(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

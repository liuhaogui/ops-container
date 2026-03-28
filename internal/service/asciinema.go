package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const recordingsDir = "recordings"

type CastRecorder struct {
	file      *os.File
	startedAt time.Time
	mu        sync.Mutex
}

type RecordingInfo struct {
	Name        string `json:"name"`
	ContainerID string `json:"container_id"`
	CreatedAt   string `json:"created_at"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

type castHeader struct {
	Version   int               `json:"version"`
	Width     uint              `json:"width"`
	Height    uint              `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Title     string            `json:"title,omitempty"`
	Command   string            `json:"command,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

func NewCastRecorder(containerID string, cols, rows uint, cmd []string) (*CastRecorder, string, error) {
	if err := os.MkdirAll(recordingsDir, 0o755); err != nil {
		return nil, "", err
	}

	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 40
	}

	filename := fmt.Sprintf("%s_%d.cast", containerID, time.Now().Unix())
	fullPath := filepath.Join(recordingsDir, filename)

	file, err := os.Create(fullPath)
	if err != nil {
		return nil, "", err
	}

	header := castHeader{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: time.Now().Unix(),
		Title:     fmt.Sprintf("container %s", containerID),
		Command:   strings.Join(cmd, " "),
		Env: map[string]string{
			"SHELL": firstOrDefault(cmd, "/bin/sh"),
			"TERM":  "xterm-256color",
		},
	}

	body, err := json.Marshal(header)
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}

	if _, err := file.Write(append(body, '\n')); err != nil {
		_ = file.Close()
		return nil, "", err
	}

	return &CastRecorder{
		file:      file,
		startedAt: time.Now(),
	}, filename, nil
}

func (r *CastRecorder) RecordOutput(data string) error {
	if r == nil || r.file == nil || data == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	event := []interface{}{
		roundDuration(time.Since(r.startedAt).Seconds()),
		"o",
		data,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = r.file.Write(append(body, '\n'))
	return err
}

func (r *CastRecorder) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

func ListRecordings(containerID string, token string) ([]RecordingInfo, error) {
	entries, err := os.ReadDir(recordingsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RecordingInfo{}, nil
		}
		return nil, err
	}

	items := make([]RecordingInfo, 0)
	prefix := containerID + "_"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cast") {
			continue
		}
		if containerID != "" && !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		url := "/api/v1/recordings/" + entry.Name()
		if token != "" {
			url += "?token=" + token
		}

		items = append(items, RecordingInfo{
			Name:        entry.Name(),
			ContainerID: parseContainerID(entry.Name()),
			CreatedAt:   info.ModTime().Format(time.RFC3339),
			Size:        info.Size(),
			URL:         url,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func RecordingPath(name string) (string, error) {
	clean := filepath.Base(name)
	if clean == "." || clean == "" || !strings.HasSuffix(clean, ".cast") {
		return "", fmt.Errorf("invalid recording name")
	}
	return filepath.Join(recordingsDir, clean), nil
}

func parseContainerID(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), ".cast")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return base
	}
	return strings.Join(parts[:len(parts)-1], "_")
}

func firstOrDefault(items []string, fallback string) string {
	if len(items) == 0 || strings.TrimSpace(items[0]) == "" {
		return fallback
	}
	return items[0]
}

func roundDuration(seconds float64) float64 {
	return float64(int(seconds*1000)) / 1000
}

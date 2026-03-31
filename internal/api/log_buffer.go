package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// LogEntry represents a single captured log line.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Raw       string    `json:"raw"`
}

// LogRingBuffer is a fixed-capacity, thread-safe ring buffer of log entries.
// It implements io.Writer so it can be composed via io.MultiWriter.
type LogRingBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	head    int
	count   int
	cap     int
	seq     int64
}

// NewLogRingBuffer creates a ring buffer with the given capacity.
func NewLogRingBuffer(capacity int) *LogRingBuffer {
	if capacity < 1 {
		capacity = 100
	}
	return &LogRingBuffer{
		entries: make([]LogEntry, capacity),
		cap:     capacity,
	}
}

// Write implements io.Writer. Each call may contain one or more log lines.
// It parses structured slog output to extract level and message.
func (b *LogRingBuffer) Write(p []byte) (n int, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(p))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		entry := parseLine(line)
		b.mu.Lock()
		b.entries[b.head] = entry
		b.head = (b.head + 1) % b.cap
		if b.count < b.cap {
			b.count++
		}
		b.seq++
		b.mu.Unlock()
	}
	return len(p), nil
}

// Entries returns the most recent limit entries, newest first.
func (b *LogRingBuffer) Entries(limit int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 || limit > b.count {
		limit = b.count
	}
	result := make([]LogEntry, limit)
	for i := 0; i < limit; i++ {
		idx := (b.head - 1 - i + b.cap) % b.cap
		result[i] = b.entries[idx]
	}
	return result
}

// Seq returns the current sequence number for change detection.
func (b *LogRingBuffer) Seq() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.seq
}

// parseLine extracts level and message from a slog-formatted log line.
func parseLine(line string) LogEntry {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Raw:       line,
	}

	// Try JSON format first: {"time":"...","level":"INFO","msg":"..."}
	if len(line) > 0 && line[0] == '{' {
		var j struct {
			Time  string `json:"time"`
			Level string `json:"level"`
			Msg   string `json:"msg"`
		}
		if json.Unmarshal([]byte(line), &j) == nil && j.Msg != "" {
			entry.Message = j.Msg
			entry.Level = strings.ToUpper(j.Level)
			if t, err := time.Parse(time.RFC3339Nano, j.Time); err == nil {
				entry.Timestamp = t
			}
			return entry
		}
	}

	// Text format: time=2026-03-30T... level=INFO msg="something"
	if idx := strings.Index(line, "level="); idx >= 0 {
		rest := line[idx+6:]
		if sp := strings.IndexByte(rest, ' '); sp > 0 {
			entry.Level = strings.ToUpper(rest[:sp])
		} else {
			entry.Level = strings.ToUpper(rest)
		}
	}
	if idx := strings.Index(line, "msg="); idx >= 0 {
		rest := line[idx+4:]
		if len(rest) > 0 && rest[0] == '"' {
			if end := strings.Index(rest[1:], "\""); end >= 0 {
				entry.Message = rest[1 : end+1]
			}
		} else if sp := strings.IndexByte(rest, ' '); sp > 0 {
			entry.Message = rest[:sp]
		} else {
			entry.Message = rest
		}
	}
	if entry.Message == "" {
		entry.Message = line
	}
	return entry
}

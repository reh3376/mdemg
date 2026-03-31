package api

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLogRingBuffer_BasicWrite(t *testing.T) {
	buf := NewLogRingBuffer(10)

	_, _ = buf.Write([]byte("time=2026-03-30T12:00:00Z level=INFO msg=\"hello world\"\n"))
	entries := buf.Entries(10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != "INFO" {
		t.Errorf("expected level INFO, got %s", entries[0].Level)
	}
	if entries[0].Message != "hello world" {
		t.Errorf("expected message 'hello world', got %q", entries[0].Message)
	}
}

func TestLogRingBuffer_WrapAround(t *testing.T) {
	buf := NewLogRingBuffer(3)

	for i := 0; i < 5; i++ {
		_, _ = buf.Write([]byte(fmt.Sprintf("time=2026-03-30T12:00:0%dZ level=INFO msg=\"entry %d\"\n", i, i)))
	}

	entries := buf.Entries(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (capacity), got %d", len(entries))
	}
	// Newest first
	if entries[0].Message != "entry 4" {
		t.Errorf("expected newest 'entry 4', got %q", entries[0].Message)
	}
	if entries[2].Message != "entry 2" {
		t.Errorf("expected oldest 'entry 2', got %q", entries[2].Message)
	}
}

func TestLogRingBuffer_ConcurrentSafety(t *testing.T) {
	buf := NewLogRingBuffer(100)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = buf.Write([]byte(fmt.Sprintf("level=INFO msg=\"goroutine %d entry %d\"\n", n, j)))
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = buf.Entries(10)
				_ = buf.Seq()
			}
		}()
	}

	wg.Wait()

	entries := buf.Entries(100)
	if len(entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(entries))
	}
}

func TestLogRingBuffer_ParsesJSONLog(t *testing.T) {
	buf := NewLogRingBuffer(10)
	line := `{"time":"2026-03-30T12:00:00.000Z","level":"ERROR","msg":"something failed","key":"val"}` + "\n"
	_, _ = buf.Write([]byte(line))

	entries := buf.Entries(1)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != "ERROR" {
		t.Errorf("expected level ERROR, got %s", entries[0].Level)
	}
	if entries[0].Message != "something failed" {
		t.Errorf("expected message 'something failed', got %q", entries[0].Message)
	}
	if entries[0].Timestamp.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", entries[0].Timestamp.Year())
	}
}

func TestLogRingBuffer_ParsesTextLog(t *testing.T) {
	buf := NewLogRingBuffer(10)
	line := "time=2026-03-30T12:00:00.000Z level=WARN msg=\"disk almost full\" path=/dev/sda1\n"
	_, _ = buf.Write([]byte(line))

	entries := buf.Entries(1)
	if entries[0].Level != "WARN" {
		t.Errorf("expected level WARN, got %s", entries[0].Level)
	}
	if entries[0].Message != "disk almost full" {
		t.Errorf("expected message 'disk almost full', got %q", entries[0].Message)
	}
}

func TestLogRingBuffer_Seq(t *testing.T) {
	buf := NewLogRingBuffer(10)

	if buf.Seq() != 0 {
		t.Errorf("expected initial seq 0, got %d", buf.Seq())
	}

	_, _ = buf.Write([]byte("level=INFO msg=\"one\"\n"))
	if buf.Seq() != 1 {
		t.Errorf("expected seq 1 after one write, got %d", buf.Seq())
	}

	_, _ = buf.Write([]byte("level=INFO msg=\"two\"\nlevel=INFO msg=\"three\"\n"))
	if buf.Seq() != 3 {
		t.Errorf("expected seq 3 after three lines, got %d", buf.Seq())
	}
}

func TestLogRingBuffer_LimitParam(t *testing.T) {
	buf := NewLogRingBuffer(10)
	for i := 0; i < 5; i++ {
		_, _ = buf.Write([]byte(fmt.Sprintf("level=INFO msg=\"entry %d\"\n", i)))
	}

	entries := buf.Entries(2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries with limit=2, got %d", len(entries))
	}
	if entries[0].Message != "entry 4" {
		t.Errorf("expected newest 'entry 4', got %q", entries[0].Message)
	}
}

func TestLogRingBuffer_EmptyBuffer(t *testing.T) {
	buf := NewLogRingBuffer(10)
	entries := buf.Entries(10)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from empty buffer, got %d", len(entries))
	}
}

func TestLogRingBuffer_MultilineWrite(t *testing.T) {
	buf := NewLogRingBuffer(10)
	lines := "level=INFO msg=\"line one\"\nlevel=ERROR msg=\"line two\"\nlevel=DEBUG msg=\"line three\"\n"
	_, _ = buf.Write([]byte(lines))

	entries := buf.Entries(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries from multiline write, got %d", len(entries))
	}
}

func TestLogRingBuffer_DefaultCapacity(t *testing.T) {
	buf := NewLogRingBuffer(0)
	if buf.cap != 100 {
		t.Errorf("expected default capacity 100 for 0 input, got %d", buf.cap)
	}
	buf2 := NewLogRingBuffer(-5)
	if buf2.cap != 100 {
		t.Errorf("expected default capacity 100 for negative input, got %d", buf2.cap)
	}
}

func TestLogRingBuffer_FallbackParsing(t *testing.T) {
	buf := NewLogRingBuffer(10)
	// Line with no recognized format
	_, _ = buf.Write([]byte("just a plain log line with no structure\n"))

	entries := buf.Entries(1)
	if entries[0].Level != "INFO" {
		t.Errorf("expected fallback level INFO, got %s", entries[0].Level)
	}
	if entries[0].Message != "just a plain log line with no structure" {
		t.Errorf("expected raw line as message, got %q", entries[0].Message)
	}
}

func TestLogRingBuffer_TimestampPopulated(t *testing.T) {
	buf := NewLogRingBuffer(10)
	before := time.Now()
	_, _ = buf.Write([]byte("level=INFO msg=\"test\"\n"))
	after := time.Now()

	entries := buf.Entries(1)
	ts := entries[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

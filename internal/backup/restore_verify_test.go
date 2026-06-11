package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "mdemg/api/transferpb"
	"mdemg/internal/jobs"
	"mdemg/internal/transfer"
)

// testChunk builds a SpaceChunk with n nodes, e edges, o observations.
func testChunk(n, e, o int) *pb.SpaceChunk {
	c := &pb.SpaceChunk{}
	if n > 0 {
		nb := &pb.NodeBatch{}
		for range n {
			nb.Nodes = append(nb.Nodes, &pb.NodeData{NodeId: "n"})
		}
		c.Nodes = nb
	}
	if e > 0 {
		eb := &pb.EdgeBatch{}
		for range e {
			eb.Edges = append(eb.Edges, &pb.EdgeData{})
		}
		c.Edges = eb
	}
	if o > 0 {
		ob := &pb.ObservationBatch{}
		for range o {
			ob.Observations = append(ob.Observations, &pb.ObservationData{})
		}
		c.Observations = ob
	}
	return c
}

func TestCountChunkContents(t *testing.T) {
	chunks := []*pb.SpaceChunk{
		testChunk(3, 2, 1),
		testChunk(0, 5, 0),
		nil, // must not panic
		testChunk(1, 0, 4),
	}
	n, e, o := countChunkContents(chunks)
	if n != 4 || e != 7 || o != 5 {
		t.Fatalf("expected 4/7/5, got %d/%d/%d", n, e, o)
	}
}

func newTestJob(t *testing.T, id string) *jobs.Job {
	t.Helper()
	job, _ := jobs.GetQueue().CreateJob(id, "test", nil)
	return job
}

func TestRestoreChecksumGate_RejectsCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bk-test.mdemg")
	if err := os.WriteFile(path, []byte("corrupted bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Service{cfg: Config{StorageDir: dir}}
	manifest := &BackupManifest{Checksum: "sha256:deadbeef"}

	_, err := s.restoreFromMdemg(context.Background(), newTestJob(t, "rt-corrupt"), path, manifest)
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("expected integrity failure before import, got %v", err)
	}
}

func TestRestoreChecksumGate_LegacyManifestProceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bk-legacy.mdemg")
	// Invalid transfer format: the gate must pass (warn) and the failure
	// must come from reading the file, proving no checksum rejection.
	if err := os.WriteFile(path, []byte("not a transfer file"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Service{cfg: Config{StorageDir: dir}}
	manifest := &BackupManifest{Checksum: ""} // legacy

	_, err := s.restoreFromMdemg(context.Background(), newTestJob(t, "rt-legacy"), path, manifest)
	if err == nil || !strings.Contains(err.Error(), "read .mdemg file") {
		t.Fatalf("expected read error (gate passed), got %v", err)
	}
}

func TestRestoreCompleteness_RejectsTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bk-trunc.mdemg")
	// Real transfer file containing 1 node...
	if err := transfer.WriteFile(path, &transfer.ExportResult{
		Chunks: []*pb.SpaceChunk{testChunk(1, 0, 0)},
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	// ...but the manifest says it should contain 5 nodes.
	manifest := &BackupManifest{Checksum: sum, FileNodeCount: 5}

	s := &Service{cfg: Config{StorageDir: dir}}
	_, err = s.restoreFromMdemg(context.Background(), newTestJob(t, "rt-trunc"), path, manifest)
	if err == nil || !strings.Contains(err.Error(), "completeness check failed") {
		t.Fatalf("expected completeness failure, got %v", err)
	}
}

func TestWaitForBackupJob_Completes(t *testing.T) {
	s := &Service{cfg: Config{SnapshotWaitTimeoutSec: 5}}
	q := jobs.GetQueue()
	job, _ := q.CreateJob("snap-ok", "backup_full", nil)
	go func() {
		time.Sleep(100 * time.Millisecond)
		job.Complete(map[string]any{})
	}()
	if err := s.waitForBackupJob(context.Background(), "snap-ok"); err != nil {
		t.Fatalf("expected completion, got %v", err)
	}
}

func TestWaitForBackupJob_FailsClosed(t *testing.T) {
	s := &Service{cfg: Config{SnapshotWaitTimeoutSec: 5}}
	q := jobs.GetQueue()
	job, _ := q.CreateJob("snap-fail", "backup_full", nil)
	job.Fail(os.ErrInvalid)
	if err := s.waitForBackupJob(context.Background(), "snap-fail"); err == nil || !strings.Contains(err.Error(), "snapshot job failed") {
		t.Fatalf("expected failure propagation, got %v", err)
	}

	// Unknown job ID also fails closed.
	if err := s.waitForBackupJob(context.Background(), "snap-ghost"); err == nil {
		t.Fatal("expected error for unknown snapshot job")
	}
}

func TestWaitForBackupJob_Timeout(t *testing.T) {
	s := &Service{cfg: Config{SnapshotWaitTimeoutSec: 1}}
	q := jobs.GetQueue()
	_, _ = q.CreateJob("snap-stuck", "backup_full", nil) // never completes
	start := time.Now()
	err := s.waitForBackupJob(context.Background(), "snap-stuck")
	if err == nil || !strings.Contains(err.Error(), "did not complete within") {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
}

func TestDockerCommand_RoutingTable(t *testing.T) {
	cases := []struct {
		fullCmd    string
		wantCustom string // empty = expect dockerbin routing (absolute path or "docker")
	}{
		{"", ""},
		{"docker", ""},
		{"/custom/bin/docker", "/custom/bin/docker"},
		{"podman", "podman"},
	}
	for _, tc := range cases {
		s := &Service{cfg: Config{FullCmd: tc.fullCmd}}
		got := s.dockerCommand()
		if tc.wantCustom != "" {
			if got != tc.wantCustom {
				t.Errorf("FullCmd=%q: expected operator override %q, got %q", tc.fullCmd, tc.wantCustom, got)
			}
			continue
		}
		// dockerbin.Path() resolves to an absolute docker path or falls
		// back to "docker"; either way it must not be empty.
		if got == "" {
			t.Errorf("FullCmd=%q: dockerbin routing returned empty", tc.fullCmd)
		}
	}
}

// sparseFile creates a file whose logical size is sizeBytes without
// consuming disk (retention sums logical sizes).
func sparseFile(t *testing.T, path string, sizeBytes int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(sizeBytes); err != nil {
		t.Fatal(err)
	}
}

// BACKUP-RESTORE-VERIFY-001 live-caught: a quota smaller than one backup
// must not delete the backup that was just created (the 2 GB default quota
// deleted every 3 GB whole-database backup ~80 ms after completion).
func TestRetention_QuotaNeverDeletesNewestPerType(t *testing.T) {
	dir := t.TempDir()
	s := &Service{cfg: Config{
		StorageDir:            dir,
		RetentionMaxStorageGB: 1, // 1 GB quota
	}}

	mk := func(id, createdAt string) {
		sparseFile(t, filepath.Join(dir, id+".mdemg"), 800*1024*1024)
		m := BackupManifest{BackupID: id, Type: string(BackupTypePartial), CreatedAt: createdAt}
		if err := s.writeManifest(&BackupRecord{BackupID: id}, m); err != nil {
			t.Fatal(err)
		}
	}
	mk("bk-old", "2026-06-10T00:00:00Z")
	mk("bk-new", "2026-06-11T00:00:00Z")

	res, err := s.RunRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "bk-new.mdemg")); err != nil {
		t.Fatalf("newest backup was deleted by quota retention: %v (deleted=%v)", err, res.Deleted)
	}
	for _, id := range res.Deleted {
		if id == "bk-new" {
			t.Fatal("newest backup in deletion list")
		}
	}
}

func TestRetention_QuotaSingleOversizeBackupSurvives(t *testing.T) {
	dir := t.TempDir()
	s := &Service{cfg: Config{
		StorageDir:            dir,
		RetentionMaxStorageGB: 1,
	}}
	// One 2 GB sparse backup — over quota by itself (the live incident shape).
	sparseFile(t, filepath.Join(dir, "bk-only.mdemg"), 2*1024*1024*1024)
	m := BackupManifest{BackupID: "bk-only", Type: string(BackupTypePartial), CreatedAt: "2026-06-11T00:00:00Z"}
	if err := s.writeManifest(&BackupRecord{BackupID: "bk-only"}, m); err != nil {
		t.Fatal(err)
	}

	res, err := s.RunRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 0 {
		t.Fatalf("oversize-but-only backup was deleted: %v", res.Deleted)
	}
	if _, err := os.Stat(filepath.Join(dir, "bk-only.mdemg")); err != nil {
		t.Fatal("the only backup must survive an over-quota condition")
	}
}

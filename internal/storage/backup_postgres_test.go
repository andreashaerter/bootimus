package storage

import (
	"archive/tar"
	"bytes"
	"errors"
	"os/exec"
	"testing"

	"bootimus/internal/models"
)

func TestPostgresBackupRoundTrip(t *testing.T) {
	store := newTestStore(t)
	if !store.postgres() {
		t.Skip(postgresTestEnv + " not set")
	}
	for _, tool := range []string{"pg_dump", "psql"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}

	kept := "aa:bb:cc:dd:ee:50"
	if err := store.CreateClient(&models.Client{MACAddress: kept, Name: "kept", Static: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAuditLog(&models.AuditLog{Username: "admin", Method: "POST", Path: "/api/clients", Status: 201}); err != nil {
		t.Fatal(err)
	}

	var dump bytes.Buffer
	name, err := store.Snapshot(&dump)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if name != postgresDatabaseDump {
		t.Fatalf("snapshot name = %q, want %q", name, postgresDatabaseDump)
	}
	if !bytes.Contains(dump.Bytes(), []byte(store.cfg.SearchPath)) {
		t.Fatalf("dump does not mention the test schema %s", store.cfg.SearchPath)
	}

	later := "aa:bb:cc:dd:ee:51"
	if err := store.CreateClient(&models.Client{MACAddress: later, Name: "after-snapshot"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateClient(kept, &models.Client{MACAddress: kept, Name: "renamed"}); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	archive := buildArchive(t, []archiveEntry{
		{name: postgresDatabaseDump, body: dump.String()},
		{name: "autoinstall/", typeflag: tar.TypeDir},
		{name: "autoinstall/notes.txt", body: "restored"},
	})
	summary, err := StageRestore(dataDir, archive)
	if err != nil {
		t.Fatalf("StageRestore: %v", err)
	}
	if summary.Database != "postgres" {
		t.Fatalf("staged database = %q, want postgres", summary.Database)
	}

	if _, err := ApplyPendingRestore(dataDir, store.cfg); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}
	if _, err := PendingRestore(dataDir); !errors.Is(err, ErrNoPendingRestore) {
		t.Fatalf("staging directory should be gone, got %v", err)
	}

	client, err := store.GetClient(kept)
	if err != nil {
		t.Fatalf("GetClient after restore: %v", err)
	}
	if client.Name != "kept" {
		t.Fatalf("client name after restore = %q, want the snapshot value", client.Name)
	}
	if _, err := store.GetClient(later); err == nil {
		t.Fatal("client created after the snapshot should not survive the restore")
	}
	entries, total, err := store.ListAuditLogs(AuditLogFilter{Limit: 10})
	if err != nil || total != 1 || len(entries) != 1 {
		t.Fatalf("audit log after restore = %d entries (total %d), %v", len(entries), total, err)
	}
}

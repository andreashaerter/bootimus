package storage

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"gorm.io/gorm"
)

const postgresTestEnv = "BOOTIMUS_TEST_POSTGRES_DSN"

type testStore struct {
	Storage
	db  *gorm.DB
	cfg *Config
}

func (s *testStore) postgres() bool {
	return s.cfg != nil
}

func newTestStore(t *testing.T) *testStore {
	t.Helper()
	if dsn := os.Getenv(postgresTestEnv); dsn != "" {
		return newPostgresTestStore(t, dsn)
	}
	store, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &testStore{Storage: store, db: store.db}
}

func testSchemaName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "test_" + hex.EncodeToString(b)
}

func newPostgresTestStore(t *testing.T, dsn string) *testStore {
	t.Helper()
	base, err := ParsePostgresDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := NewPostgresStore(base)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	schema := testSchemaName(t)
	if err := admin.db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	cfg := *base
	cfg.SearchPath = schema
	store, err := NewPostgresStore(&cfg)
	if err != nil {
		t.Fatalf("connect to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		if sqlDB, err := store.db.DB(); err == nil {
			sqlDB.Close()
		}
		admin.db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		if sqlDB, err := admin.db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate on PostgreSQL: %v", err)
	}
	return &testStore{Storage: store, db: store.db, cfg: &cfg}
}

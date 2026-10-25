package storagetest

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"bootimus/internal/storage"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const EnvDSN = "BOOTIMUS_TEST_POSTGRES_DSN"

func New(t testing.TB) storage.Storage {
	t.Helper()
	if dsn := os.Getenv(EnvDSN); dsn != "" {
		return newPostgres(t, dsn)
	}
	store, err := storage.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func UsingPostgres() bool {
	return os.Getenv(EnvDSN) != ""
}

func schemaName(t testing.TB) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "test_" + hex.EncodeToString(b)
}

func newPostgres(t testing.TB, dsn string) storage.Storage {
	t.Helper()
	cfg, err := storage.ParsePostgresDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	schema := schemaName(t)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	cfg.SearchPath = schema
	store, err := storage.NewPostgresStore(cfg)
	if err != nil {
		t.Fatalf("connect to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		store.Close()
		admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate on PostgreSQL: %v", err)
	}
	return store
}

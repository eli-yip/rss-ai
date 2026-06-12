package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ItemTitle is the cached AI-rewritten title for one feed item, keyed by the
// item's id (guid / atom id / link) within a handler prefix.
type ItemTitle struct {
	ID             string `gorm:"primaryKey;column:id"`
	Handler        string `gorm:"primaryKey;column:handler"`
	SourceTitle    string `gorm:"column:source_title"`
	RewrittenTitle string `gorm:"column:rewritten_title"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ItemTitle) TableName() string { return "item_titles" }

type Store struct {
	db *gorm.DB
}

// New opens the database named by dsn and runs AutoMigrate for the cache table.
// The driver is chosen from the dsn scheme (see dialectorFor).
func New(dsn string) (*Store, error) {
	dialector, err := dialectorFor(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.AutoMigrate(&ItemTitle{}); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	return &Store{db: db}, nil
}

// dialectorFor selects the GORM driver from the dsn scheme:
//
//   - postgres:// or postgresql://  → PostgreSQL
//   - sqlite:<path> (e.g. sqlite:///data/rss-ai.db, sqlite://rss-ai.db) or a
//     bare file: DSN → SQLite, via the pure-Go (CGO-free) modernc driver.
//
// The sqlite: prefix and an optional // are stripped to yield the file path, so
// sqlite://:memory: gives an in-memory database.
func dialectorFor(dsn string) (gorm.Dialector, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return postgres.Open(dsn), nil
	case strings.HasPrefix(dsn, "sqlite:"):
		path := strings.TrimPrefix(strings.TrimPrefix(dsn, "sqlite:"), "//")
		return sqlite.Open(path), nil
	case strings.HasPrefix(dsn, "file:"):
		return sqlite.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported db dsn %q: want a postgres:// or sqlite: scheme", dsn)
	}
}

// Ping verifies the underlying connection is alive (used by /readyz).
func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

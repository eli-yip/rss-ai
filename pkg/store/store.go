package store

import (
	"context"
	"fmt"
	"time"

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

// New opens the PostgreSQL connection and runs AutoMigrate for the cache table.
func New(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.AutoMigrate(&ItemTitle{}); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Ping verifies the underlying connection is alive (used by /readyz).
func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

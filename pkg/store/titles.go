package store

import "context"

// LookupTitles returns the cached rows for the given ids under handler, keyed by
// id. Missing ids are simply absent from the map. An empty ids slice returns an
// empty map without querying.
func (s *Store) LookupTitles(ctx context.Context, handler string, ids []string) (map[string]ItemTitle, error) {
	out := make(map[string]ItemTitle, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []ItemTitle
	if err := s.db.WithContext(ctx).
		Where("handler = ? AND id IN ?", handler, ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, nil
}

// SaveTitle inserts or updates the cached title for (id, handler) at the
// application level: it looks the row up by primary key and Updates it when
// present, else Creates it. There is no DB-level upsert — singleflight
// serializes writers per (id, handler), so there is no in-process race (spec §6).
func (s *Store) SaveTitle(ctx context.Context, it ItemTitle) error {
	db := s.db.WithContext(ctx)

	// Limit(1).Find (not First) so a missing row is RowsAffected==0, not a logged
	// ErrRecordNotFound.
	var existing ItemTitle
	res := db.Where("id = ? AND handler = ?", it.ID, it.Handler).Limit(1).Find(&existing)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return db.Create(&it).Error
	}

	return db.Model(&ItemTitle{}).
		Where("id = ? AND handler = ?", it.ID, it.Handler).
		Updates(map[string]any{
			"source_title":    it.SourceTitle,
			"rewritten_title": it.RewrittenTitle,
		}).Error
}

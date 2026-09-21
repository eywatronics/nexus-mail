package store

import (
	"context"
	"fmt"

	"nexusmail/internal/model"
)

// FoldersOfMessages reports which folder each message is in.
//
// The folder rather than its id, because the caller's question is usually
// about what the folder is for — whether it is the trash, which account it
// belongs to — and answering that from an id would mean a second round of
// lookups at every call site.
//
// Messages that are not there are simply absent from the result, the same way
// an action treats them: a selection can name a message the retention window
// removed a moment ago, and that is a stale list rather than an error.
func (s *Store) FoldersOfMessages(ctx context.Context, messageIDs []int64) (map[int64]model.Folder, error) {
	if len(messageIDs) == 0 {
		return map[int64]model.Folder{}, nil
	}

	args := make([]any, 0, len(messageIDs))
	for _, id := range messageIDs {
		args = append(args, id)
	}

	rows, err := s.read.QueryContext(ctx,
		`SELECT m.id, f.id, f.account_id, f.name, f.path, f.delimiter, f.attributes
		   FROM messages m
		   JOIN folders f ON f.id = m.folder_id
		  WHERE m.id IN (`+placeholders(len(messageIDs))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: locating messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[int64]model.Folder{}
	for rows.Next() {
		var (
			messageID int64
			f         model.Folder
			attrs     string
		)
		if err := rows.Scan(&messageID, &f.ID, &f.AccountID, &f.Name, &f.Path,
			&f.Delimiter, &attrs); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(attrs, &f.Attributes); err != nil {
			return nil, fmt.Errorf("store: decode attributes for %q: %w", f.Path, err)
		}
		out[messageID] = f
	}
	return out, rows.Err()
}

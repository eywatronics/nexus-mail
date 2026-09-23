package store

import (
	"context"
	"fmt"

	"nexusmail/internal/model"
)

// ListThreadedMessages returns a folder's messages with each conversation's
// messages next to each other, conversations ordered by their newest message.
//
// The ordering is the whole feature. A conversation is one thing to the reader
// even though it is many rows here, and a list that sorted purely by date
// would scatter a five-message exchange through a week of unrelated mail. The
// window groups runs of adjacent rows sharing a thread id; keeping them
// adjacent is this query's job.
//
// Threads are ranked by their newest message rather than their first, so a
// conversation that gets a reply today comes back to the top — which is what
// "there is something new here" has to look like.
//
// Paging is still by message, not by thread, and a page can end in the middle
// of a conversation. That is deliberate: paging by thread would make the page
// size unpredictable, and a thread of four hundred messages would have to
// arrive all at once. The window continues the group when the next page lands,
// because the order guarantees the rest of it comes first.
func (s *Store) ListThreadedMessages(ctx context.Context, folderID int64, limit, offset int) ([]model.Message, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+prefixedMessageColumns+`, t.thread_count
		   FROM messages m
		   JOIN (
		     SELECT thread_id,
		            MAX(internal_date) AS newest,
		            MAX(uid)           AS newest_uid,
		            COUNT(*)           AS thread_count
		       FROM messages
		      WHERE folder_id = ?
		      GROUP BY thread_id
		   ) t ON t.thread_id = m.thread_id
		  WHERE m.folder_id = ?
		  ORDER BY t.newest DESC, t.newest_uid DESC, t.thread_id,
		           m.internal_date ASC, m.uid ASC
		  LIMIT ? OFFSET ?`, folderID, folderID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list threaded messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanThreadedRows(rows)
}

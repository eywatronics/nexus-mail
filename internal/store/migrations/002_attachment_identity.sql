-- A message's attachments are re-described on every header sync, because the
-- server sends the BODYSTRUCTURE again each time. Without an identity for a
-- part, each sync would insert the same rows afresh and a message would grow
-- attachments it never had.
--
-- The identity is (message_id, part_id): a part number means one thing within
-- one message, for as long as its UID is valid. When the UID stops being valid
-- the message row goes, and these go with it.
--
-- SQLite cannot add a constraint to an existing table, so the table is rebuilt.
-- It is safe to do bluntly here: nothing has ever written to it, and the rows
-- it would carry are derived from the server on the next sync anyway.

DROP TABLE attachments;

CREATE TABLE attachments (
  id          INTEGER PRIMARY KEY,
  message_id  INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  part_id     TEXT NOT NULL,
  filename    TEXT NOT NULL DEFAULT '',
  mime_type   TEXT NOT NULL DEFAULT '',
  size        INTEGER NOT NULL DEFAULT 0,
  -- local_path is where the bytes were saved. Empty until somebody opens it:
  -- a twenty-megabyte file is not worth fetching with every header sync.
  local_path  TEXT NOT NULL DEFAULT '',
  downloaded  INTEGER NOT NULL DEFAULT 0,

  UNIQUE(message_id, part_id)
);

CREATE INDEX idx_attachments_message ON attachments(message_id);

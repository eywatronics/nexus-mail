CREATE TABLE accounts (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  display_name  TEXT NOT NULL DEFAULT '',
  provider      TEXT NOT NULL,
  auth_kind     TEXT NOT NULL,
  imap_host     TEXT NOT NULL,
  imap_port     INTEGER NOT NULL,
  smtp_host     TEXT NOT NULL DEFAULT '',
  smtp_port     INTEGER NOT NULL DEFAULT 0,
  -- Names the entry in the SecretStore. The secret itself is never stored here.
  secret_ref    TEXT NOT NULL,
  created_at    INTEGER NOT NULL
);

CREATE TABLE folders (
  id              INTEGER PRIMARY KEY,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  path            TEXT NOT NULL,
  delimiter       TEXT NOT NULL DEFAULT '/',
  attributes      TEXT NOT NULL DEFAULT '',
  -- When the server changes this, every UID we hold for the folder becomes
  -- meaningless and the folder must be re-fetched from scratch.
  uid_validity    INTEGER NOT NULL DEFAULT 0,
  uid_next        INTEGER NOT NULL DEFAULT 0,
  -- Zero when the server does not advertise CONDSTORE.
  highest_modseq  INTEGER NOT NULL DEFAULT 0,
  total_count     INTEGER NOT NULL DEFAULT 0,
  unread_count    INTEGER NOT NULL DEFAULT 0,
  last_synced_at  INTEGER NOT NULL DEFAULT 0,
  UNIQUE(account_id, path)
);

CREATE TABLE messages (
  id              INTEGER PRIMARY KEY,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  folder_id       INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  uid             INTEGER NOT NULL,
  message_id      TEXT NOT NULL DEFAULT '',
  thread_id       TEXT NOT NULL DEFAULT '',
  in_reply_to     TEXT NOT NULL DEFAULT '',
  refs            TEXT NOT NULL DEFAULT '',
  subject         TEXT NOT NULL DEFAULT '',
  from_name       TEXT NOT NULL DEFAULT '',
  from_addr       TEXT NOT NULL DEFAULT '',
  to_addrs        TEXT NOT NULL DEFAULT '',
  cc_addrs        TEXT NOT NULL DEFAULT '',
  -- Reconciled against internal_date on ingest: an unparseable or implausible
  -- Date: header is replaced by the server's delivery time.
  date            INTEGER NOT NULL DEFAULT 0,
  internal_date   INTEGER NOT NULL DEFAULT 0,
  size            INTEGER NOT NULL DEFAULT 0,
  snippet         TEXT NOT NULL DEFAULT '',
  flags           TEXT NOT NULL DEFAULT '',
  has_attachments INTEGER NOT NULL DEFAULT 0,
  body_fetched    INTEGER NOT NULL DEFAULT 0,
  -- The database, not application logic, is what keeps a reconnect from
  -- duplicating a mailbox.
  UNIQUE(account_id, folder_id, uid)
);

CREATE INDEX idx_messages_list   ON messages(folder_id, internal_date DESC);
CREATE INDEX idx_messages_thread ON messages(account_id, thread_id);

CREATE TABLE message_bodies (
  message_id  INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
  html_body   TEXT NOT NULL DEFAULT '',
  text_body   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE attachments (
  id          INTEGER PRIMARY KEY,
  message_id  INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  part_id     TEXT NOT NULL,
  filename    TEXT NOT NULL DEFAULT '',
  mime_type   TEXT NOT NULL DEFAULT '',
  size        INTEGER NOT NULL DEFAULT 0,
  local_path  TEXT NOT NULL DEFAULT '',
  downloaded  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE operations (
  id              INTEGER PRIMARY KEY,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  -- folder_id and uid_validity together stamp the operation with the mailbox
  -- generation it was queued against. The M3 worker compares the stamp before
  -- applying: if the server recreated the mailbox while we were offline, the
  -- UID now refers to a different message and the operation is dropped rather
  -- than applied to the wrong mail.
  folder_id       INTEGER REFERENCES folders(id) ON DELETE CASCADE,
  uid_validity    INTEGER NOT NULL DEFAULT 0,
  kind            TEXT NOT NULL,
  payload         TEXT NOT NULL,
  state           TEXT NOT NULL,
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT NOT NULL DEFAULT '',
  created_at      INTEGER NOT NULL,
  next_attempt_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_operations_queue ON operations(account_id, state, next_attempt_at);

-- Search index over the columns that arrive with every message. Bodies are
-- fetched lazily, so a body index would be inherently incomplete in P0; it is
-- built in P3 with a one-time backfill over message_bodies.
CREATE VIRTUAL TABLE fts_messages USING fts5(
  subject,
  from_addr,
  snippet,
  content='messages',
  content_rowid='id'
);

-- FTS5 does not track its content table on its own. Doing this in application
-- code means every write path has to remember, and one that forgets leaves the
-- index silently wrong. Triggers cannot be forgotten.
CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages BEGIN
  INSERT INTO fts_messages(rowid, subject, from_addr, snippet)
  VALUES (new.id, new.subject, new.from_addr, new.snippet);
END;

-- Deleting from an FTS5 table is not a DELETE: it is a 'delete' command that
-- must be handed the values exactly as they were indexed. Wrong values corrupt
-- the index silently. old.* is correct precisely because every write goes
-- through these triggers and nothing else touches fts_messages.
CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet)
  VALUES ('delete', old.id, old.subject, old.from_addr, old.snippet);
END;

-- FTS5 has no in-place update, so this is delete-then-insert. It also covers
-- the upsert path: ON CONFLICT DO UPDATE fires this trigger, not the insert one.
CREATE TRIGGER messages_fts_update AFTER UPDATE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet)
  VALUES ('delete', old.id, old.subject, old.from_addr, old.snippet);
  INSERT INTO fts_messages(rowid, subject, from_addr, snippet)
  VALUES (new.id, new.subject, new.from_addr, new.snippet);
END;

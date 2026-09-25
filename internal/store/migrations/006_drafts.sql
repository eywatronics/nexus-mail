-- A message somebody is still writing.
--
-- Until now a composer held the only copy of what was in it. Closing the
-- window, or a crash, took the message with it — which is the worst thing a
-- mail client can do, because the one thing in it the user made themselves is
-- the part nothing else can recover.
--
-- Held here rather than sent straight to the server's Drafts folder. Every
-- keystroke cannot be an IMAP APPEND, and a client that could only save a
-- draft while online would lose the message written on a train, which is
-- exactly when people write long ones. Uploading these to the Drafts mailbox
-- is the next step and needs nothing from this table beyond a column to
-- remember which UID the last upload produced.
--
-- The address lines are stored as typed, not parsed. Parsing lives in one
-- place, in the send path; a draft that had been parsed and reassembled would
-- come back subtly different from what the person typed, and a half-typed
-- address would not survive at all.
CREATE TABLE drafts (
  id          INTEGER PRIMARY KEY,
  account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  -- Which identity it is from. Zero means the account's default, resolved at
  -- send time rather than frozen here: an identity deleted between writing and
  -- sending should fall back, not fail.
  identity_id INTEGER NOT NULL DEFAULT 0,

  to_line     TEXT NOT NULL DEFAULT '',
  cc_line     TEXT NOT NULL DEFAULT '',
  bcc_line    TEXT NOT NULL DEFAULT '',
  subject     TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT '',

  -- Threading, so a reply left half-written stays a reply.
  in_reply_to TEXT NOT NULL DEFAULT '',
  refs        TEXT NOT NULL DEFAULT '',

  -- Attachments by path, as a JSON array. The files themselves are not copied:
  -- a draft is not a place to duplicate a twenty-megabyte PDF, and the paths
  -- are re-checked against the file dialog's list when the message is sent.
  -- A file moved or deleted in the meantime is reported then.
  attachment_paths TEXT NOT NULL DEFAULT '',

  updated_at  INTEGER NOT NULL
);

-- Drafts are always read for one account, newest first, which is the order the
-- list shows them in.
CREATE INDEX idx_drafts_account ON drafts(account_id, updated_at DESC);

-- Images embedded in a formatted draft.
--
-- The HTML body refers to them as cid:<id>. The files stay where the person
-- chose them, as attachments do; this column keeps the identifier, the path
-- and the name, so reopening the draft can render the preview and sending can
-- read the same bytes. An empty column means a draft with no inline images,
-- which is every draft written before this migration.
ALTER TABLE drafts ADD COLUMN inline_images TEXT NOT NULL DEFAULT '';

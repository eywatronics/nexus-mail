-- A draft now has an HTML body as well as its plain-text one.
--
-- The composer can write formatted text, and a pause that saved only the text
-- would reopen the draft with every bold word and list flattened. The text
-- column stays the authority for the plain-text alternative and for drafts
-- written before this column existed; an empty body_html means "plain".
ALTER TABLE drafts ADD COLUMN body_html TEXT NOT NULL DEFAULT '';

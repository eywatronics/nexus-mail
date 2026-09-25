-- Who a message is from, which is not the same question as which account it
-- was sent through.
--
-- Until now `accounts.display_name` and `accounts.email` were the answer, and
-- that assumes one identity per account. It is a common assumption and a wrong
-- one: a person answers support@ and their own address from the same mailbox,
-- a consultant writes as themselves and as their company, and an alias is the
-- ordinary way an employer hands somebody a second address. Each of those
-- wants its own display name, its own reply-to and its own signature, and none
-- of them wants a second account with a second password.
--
-- SMTP submission stays on the account, because that is what it is: the
-- server you are allowed to send through. Which address you send *as* is the
-- identity. Conflating the two is why some clients make you create a whole
-- account to add an alias.

CREATE TABLE identities (
  id           INTEGER PRIMARY KEY,
  account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,

  email        TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  -- Where replies should go when that is not the From address. Empty means the
  -- From address, which is what it means in the header too.
  reply_to     TEXT NOT NULL DEFAULT '',

  -- The signature as text and, optionally, as HTML. Both, because a message
  -- with an HTML signature and no text alternative reads as a blank space in
  -- anything that will not render HTML — including this client's own plain
  -- text mode.
  signature_text TEXT NOT NULL DEFAULT '',
  signature_html TEXT NOT NULL DEFAULT '',

  -- Exactly one identity per account is the default. Enforced below rather
  -- than in application code: a mailbox with two defaults has no answer to
  -- "who is this from", and a mailbox with none cannot open a composer.
  is_default   INTEGER NOT NULL DEFAULT 0,

  -- Position in the picker. Not the id, because the order a person wants is
  -- not the order they happened to add them in.
  sort_order   INTEGER NOT NULL DEFAULT 0,

  created_at   INTEGER NOT NULL,

  -- One row per address per account. The same address on two accounts is
  -- legitimate — a shared alias delivered to two mailboxes — so the constraint
  -- is on the pair.
  UNIQUE(account_id, email)
);

CREATE INDEX idx_identities_account ON identities(account_id, sort_order);

-- At most one default per account, expressed as a partial index rather than a
-- trigger: the database refuses the second one outright, and there is no
-- window in which two rows both claim it.
CREATE UNIQUE INDEX idx_identities_one_default
  ON identities(account_id) WHERE is_default = 1;

-- Every existing account gets the identity it already had.
--
-- Without this the first account created before this migration would have no
-- identity at all, and the composer would have nothing to send as. The values
-- are the ones the account was already using, so nothing the user sees
-- changes; what changes is that the answer now lives somewhere it can grow a
-- second row.
INSERT INTO identities (account_id, email, display_name, is_default, sort_order, created_at)
SELECT id, email, display_name, 1, 0, created_at FROM accounts;

-- How the IMAP connection is encrypted, per account.
--
-- Until now every connection was implicit TLS on 993, hardcoded. That leaves
-- out on-premises Exchange, whose IMAP4 service is usually published on 143
-- with STARTTLS: its default LoginType of SecureLogin will not accept a
-- password until the connection has been upgraded, and plenty of deployments
-- never open 993 at all.
--
-- The default matters. Existing rows carry no value, and an empty string has
-- to mean the stronger option — guessing downwards would silently move an
-- account that works today onto a weaker connection.

ALTER TABLE accounts ADD COLUMN imap_security TEXT NOT NULL DEFAULT 'tls';

<div align="center">

# Nexus Mail

**A local-first, privacy-focused desktop mail client.**

Go · Wails v3 · React · SQLite

[![CI](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml/badge.svg)](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml)
[![Release](https://github.com/eywatronics/nexus-mail/actions/workflows/release.yml/badge.svg)](https://github.com/eywatronics/nexus-mail/actions/workflows/release.yml)
[![License: GPL v3](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8.svg)](https://go.dev)

<a href="README.md"><img alt="English" src="https://img.shields.io/badge/English-0f7490?style=for-the-badge"></a>
<a href="README.tr.md"><img alt="Türkçe" src="https://img.shields.io/badge/Türkçe-4a4a4a?style=for-the-badge"></a>

</div>

---

## Why this exists

Most modern mail clients are a browser wrapped in a desktop window. The result
is familiar: half a gigabyte of memory, a network round trip behind every
click, and an application that becomes useless the moment the connection drops.

The other thing they have in common is quieter. Opening a message tells the
sender you opened it, when, and roughly where from — not because anyone decided
that, but because a remote image in an HTML mail is a network request, and
nobody stopped it.

Nexus Mail is built on one rule: **the data is on disk first and on the network
second.** Your mail lives in a local SQLite database. Reading, listing,
searching and filtering never wait for a server. With no connection the
application still works completely; changes you make are queued and applied
when the connection returns. Offline is not a feature bolted on afterwards — it
is what falls out of the architecture.

## Goals

**Be fast because of where the data is, not because of how it is drawn.** Every
list, every search, every message body comes from a local database. There is no
spinner between you and mail you already have.

**Make privacy the default rather than a setting.** A client that blocks
trackers only after you find the option has already leaked the first message
you opened.

**Be honest about what it cannot do.** Where something is missing, or unsigned,
or unverified, the documentation says so. A tool that reads your mail is a tool
you have to be able to trust, and trust is not built by overstating.

**Stay a program, not a platform.** No telemetry, no accounts with us, no
sync-your-settings-to-a-cloud. The only servers it talks to are yours.

## What makes it different

**No network wait.** The interface draws everything from the local database.
Switching folders, scrolling the list and reading a message you have opened
before all work instantly, offline included.

**Trackers blocked by default.** Remote images and CSS resources do not load
until you ask — and not only `<img src>`, but `background` attributes, `srcset`
and CSS `url()` calls too. When you do allow them, the requests go out through
the application, so your IP address and `Referer` never reach the sender.

**Real isolation.** Message content is rendered inside an `<iframe sandbox>`
that is not granted `allow-same-origin`, under a restrictive Content Security
Policy delivered as a real header. Sanitising is a second layer on top of that,
not a substitute for it.

**Credentials in the OS keyring.** Passwords and OAuth tokens are never written
to the database or to a configuration file. They live in Windows Credential
Manager, macOS Keychain or the Linux Secret Service.

## Download

Every merge to `main` produces installers for three operating systems. Latest build:
**[Releases](https://github.com/eywatronics/nexus-mail/releases)**.

| Platform | File |
|---|---|
| Windows | `*-installer.exe` to install, `nexus-mail-windows-amd64.exe` to run as it is |
| macOS | `nexus-mail-macos-arm64-unsigned.zip` (Apple silicon), `...-amd64-...` (Intel) |
| Linux | `*.AppImage` runs anywhere, `*.rpm` for Fedora and RHEL, `*.deb` for Debian and Ubuntu |

**The macOS and Windows builds are unsigned.** macOS will refuse to open the
application until you allow it in System Settings → Privacy & Security, and
Windows SmartScreen will warn about an unknown publisher. Code signing is
planned; until then, this is what an unsigned build honestly looks like.

Linux packages need GTK 4 and WebKitGTK 6.0. The `.rpm` and `.deb` declare that
as a dependency; the AppImage does not bundle it.

## Status

Under development, but usable day to day as a reading client.

### What it does today

**Accounts.** Any IMAP server with a password or app password; OAuth 2.0
(XOAUTH2) for Google and Microsoft; 143/STARTTLS for on-premises Exchange.
Connection security is chosen per account and there is no unencrypted option.
The authentication mechanism is negotiated against what the server offers
(PLAIN → SASL LOGIN → LOGIN), and when none of them fit, the error names the
mechanisms the server actually advertised.

**Sync.** Initial sync, live updates over IMAP IDLE, delta sync on servers that
support CONDSTORE, and a retention window that stops the database growing
without bound — 365 days or 25,000 messages per folder, starred messages
exempt, removal local only.

**Reading.** A three-column virtualised list, conversation grouping, full-text
search (including the Turkish dotless ı, which no folding rule handles for
you), keyboard navigation, attachment listing and download, view source, save
as `.eml`, repair of a mis-declared character encoding, and three body display
modes: original HTML, simple HTML, plain text.

**Writing state.** Read, star, move and delete appear immediately, are queued,
and reach the server when the connection allows. Deleting moves to the trash;
inside the trash it asks first and then destroys. Emptying the trash is a
separate server-side operation that covers messages this client never
downloaded. The last destructive action can be taken back for five seconds
(Ctrl+Z).

**Background.** Closing the window leaves the application in the tray with sync
still running. New mail raises an operating system notification, with the
content optional — because Windows shows notifications on the lock screen
unless told otherwise.

**Settings.** Theme, body display mode, mark-as-read behaviour, conversation
grouping, notification preview, undo window, retention limits and OAuth client
IDs, all from inside the application.

### Not yet

**Sending** (M6) — this is a reading client; you cannot write a reply.
**NTLM/GSSAPI** (M10) — corporate servers with basic authentication disabled
cannot connect. **Printing** needs a separate window because of the sandboxed
reading pane, and is deferred to M6. **Code signing** (M14).

| Milestone | Scope | Status |
|---|---|---|
| **M1** | Account setup (3 paths), folder and header sync, isolated reading, search, keyboard navigation | Done |
| **M2** | Live sync over IMAP IDLE, delta sync, retention window | Done |
| **M3** | Offline-durable state writes (read, star, move, delete) | Done |
| **M4** | System tray, notifications, running in the background | Partly done |
| **M5** | Attachments, conversation grouping, source/save, encoding repair, body modes | Largely done |
| **M6** | Sending: SMTP, multiple identities, signatures, drafts, outbox, composer | Planned |
| **M7** | Contacts: local address book, vCard, CardDAV, LDAP | Planned |
| **M8** | Tags, archive, unified inbox, body search, filter engine, junk | Planned |
| **M9** | OpenPGP/S-MIME; CalDAV calendar and meeting invitations | Planned |
| **M10** | Microsoft Graph: M365 calendar and contacts | Planned |
| **M11** | Import from Thunderbird, Outlook and Apple Mail | Planned |
| **M12** | Localisation, accessibility, customisable shortcuts | Planned |
| **M13** | Local-first RAG: thread summaries, ask-your-inbox, draft assistance — local model or your own API key, off by default | Planned |
| **M14** | Automatic updates, code signing, installation packages | Planned |

How the roadmap was arrived at, why each feature is in scope, and what was
**deliberately left out**: [docs/plans/roadmap.md](docs/plans/roadmap.md) and
[docs/design/feature-inventory.md](docs/design/feature-inventory.md).

## Supported accounts

| Provider | Authentication | Note |
|---|---|---|
| Microsoft 365 / Outlook.com | OAuth 2.0 (XOAUTH2) | Needs your own Entra app registration |
| Gmail / Google Workspace | OAuth 2.0 (XOAUTH2) | Needs your own Google Cloud client ID |
| On-premises Exchange | Password | Over IMAP; 143/STARTTLS or 993/TLS |
| Generic IMAP | Password or app password | No registration needed |

On-premises Exchange connects over IMAP: choose *Other IMAP server*, use the
internal host name your IT gave you, and pick **STARTTLS** — the Exchange IMAP4
service is published on 143 by default and will not accept a password until the
connection has been upgraded. No registration and no client ID. If the
organisation has disabled basic authentication you will need NTLM, which does
not exist yet; in that case the error names the mechanisms the server offers.

Microsoft and Google need an OAuth client ID of your own. That is a consequence
of being open source and it is genuinely an advantage: access to your mailbox
stays under your control rather than routed through somebody else's app
registration. The setup takes a few minutes and is described in
[docs/oauth-setup.md](docs/oauth-setup.md).

## Architecture

```
frontend/          React + TypeScript + Vite
      │
internal/app       Wails services: the only surface the UI talks to
      │
internal/sync      Sync engine: initial sync, delta, IDLE, operation queue
      │
      ├── internal/imapx    go-imap wrapper (MailBackend interface)
      ├── internal/store    SQLite: schema, migrations, repositories
      └── internal/auth     Credential providers and secret storage
```

Dependencies point one way, and that is not a preference — it is enforced in CI
with `depguard`. The boundary that matters most: the `sync` engine never sees
go-imap directly, only the `MailBackend` interface. That is what lets the whole
of the sync logic be tested against go-imap's own in-memory server, with no
network and no live account.

Details: [docs/design/p0-architecture.md](docs/design/p0-architecture.md)

## Building from source

**Requirements:** Go 1.26+, Node 20+, the Wails v3 CLI.
On Linux also `libgtk-4-dev` and `libwebkitgtk-6.0-dev` — the stack Wails v3
links against without `-tags gtk3`.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9
git clone https://github.com/eywatronics/nexus-mail.git
cd nexus-mail
wails3 generate bindings
cd frontend && npm ci && npm run build && cd ..
wails3 build
```

`main.go` embeds `frontend/dist`, so **no Go package compiles until the
frontend has been built once.** That is why the order above matters.

### Tests

```bash
go test ./...
cd frontend && npm test
```

The race detector (`-race`) needs cgo and runs in CI.

### Installation package

```bash
wails3 task package
```

Produces a package for the platform you are on: an NSIS installer on Windows,
a `.app` on macOS, AppImage / `.deb` / `.rpm` on Linux. Output lands in `bin/`.

There is no cross-compilation: each platform builds on itself. The Wails
WebView binding needs each system's own toolchain, and forcing it through
Docker would produce a build nobody has ever run. That is also why the release
workflow uses four separate runners — Intel and Apple silicon Macs are two of
them, because an Intel Mac cannot run an arm64 build.

A local Linux package also needs the version, which lives in `build/config.yml`
rather than in the packaging config:

```bash
VERSION=0.5.0 wails3 task linux:create:deb
```

### Cutting a release

Every merge to `main` builds all four targets and replaces a rolling prerelease
tagged **`nightly`**. For a permanent release, raise `info.version` in
`build/config.yml` and push a tag with the same number:

```bash
git tag v0.6.0 && git push origin v0.6.0
```

The version lives in one place so that the installer's own version and the
release it appears under cannot drift apart.

## Where your data lives

| Platform | Path |
|---|---|
| Windows | `%APPDATA%\nexus-mail\` |
| macOS | `~/Library/Application Support/nexus-mail/` |
| Linux | `$XDG_DATA_HOME/nexus-mail/` (or `~/.local/share/nexus-mail/`) |

That directory holds `mail.db`, attachments, `config.json` and the logs.
Secrets are **never** written there — they go only to the operating system's
keyring.

Everything below can be changed from the settings screen inside the
application; the JSON is what it writes.

### Retention window

Running live sync for months grows the database indefinitely. By default the
last **365 days or 25,000 messages per folder** are kept, whichever comes
first. Anything outside that is removed from the local database only — **the
server is not touched** and the mail stays there. **Starred messages are exempt
regardless of age.**

```json
{
  "retentionDays": 365,
  "retentionMaxMessages": 25000
}
```

Setting either to `0` turns the window off: **everything is kept.** How you use
your disk is your decision, and on a local-first application that is the right
default.

### Undo window

A delete or a move waits **five seconds** before it goes to the server. During
that time Ctrl+Z, or **Undo** in the window, takes it back completely — the
message was never moved. After the window closes the offer disappears, because
a button that would fail is worse than no button.

```json
{
  "undoWindowSeconds": 5
}
```

`0` turns the window off: changes go out at once and no undo is offered.

### Notification preview

A new-mail notification shows the sender and subject by default. Windows shows
notifications on the **lock screen** unless told otherwise; if you would rather
somebody standing at your desk could not read who wrote and about what:

```json
{
  "notificationPreview": false
}
```

The notification then says only how many messages arrived and to which account.
Windows' own "hide notification content when locked" does the same job system
wide.

## Contributing

CI checks the architecture, not only the tests:

- **Layer boundaries** are enforced with `depguard`. `store`, `imapx` and
  `auth` cannot import upwards; `sync` cannot import go-imap directly.
- **Windows and Linux builds with `CGO_ENABLED=0`** are required checks. They
  double as proof that the dependency tree stays pure Go: adding a library that
  needs cgo breaks exactly these two jobs.
- **Writing to the search index from application code is forbidden** and
  checked with grep; the index is maintained by database triggers.
- **`go test -race ./...`**, plus a second run over the pure-Go path.

Commits explain *why*, not *what* — the reasoning and the rejected alternative
are the only things that cannot be recovered by reading the code.

### For repository maintainers

The `build (windows-latest)` and `build (ubuntu-latest)` jobs should be marked
as **required checks** in branch protection. The cgo guard depends on them.

## License

[GNU General Public License v3.0](LICENSE)

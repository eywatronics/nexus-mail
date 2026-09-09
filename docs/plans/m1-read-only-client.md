# Nexus Mail M1 (Salt Okunur İstemci) Implementasyon Planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kullanıcı üç auth yolundan biriyle hesabını bağlayıp, klasörlerini ve son 1000 mailinin başlığını yerel SQLite'a indirip, üç sütunlu arayüzde izole şekilde okuyabiliyor — ve uygulama ağ bağlantısı olmadan açıldığında mailler anında ekranda.

**Architecture:** Go tarafı beş katmana bölünür (`store`, `auth`, `imapx`, `sync`, `app`); bağımlılık tek yönlü akar ve `depguard` ile CI'da zorlanır. Senkron motoru IMAP'e `MailBackend` arayüzü üzerinden bağlanır, böylece go-imap v2'nin kendi `imapserver` paketiyle kurulan bellek içi sahte sunucuya karşı gerçek ağ olmadan test edilir. Frontend React 19 + Wails v3 binding'leriyle konuşur; mail HTML'i `allow-same-origin` olmayan `<iframe sandbox>` içinde render edilir.

**Tech Stack:** Go 1.26, Wails v3.0.0-beta.9, modernc.org/sqlite, emersion/go-imap v2, emersion/go-message, zalando/go-keyring, golang.org/x/oauth2, React 19, TypeScript, Tailwind, shadcn/ui, TanStack Virtual, Zustand, DOMPurify, Vitest.

## Global Constraints

Bu kısıtlar her görevin gereksinimlerine örtük olarak dahildir.

- **Modül adı:** `nexusmail`. `.golangci.yml` içindeki depguard kuralları bu ada bağlıdır.
- **Go sürüm tabanı:** 1.26. `go.mod` içinde `go 1.26`.
- **Wails sürümü sabit:** `github.com/wailsapp/wails/v3 v3.0.0-beta.9`. `go get -u` **çalıştırılmayacak**. CLI sürümü kütüphane sürümüyle eşleşmeli.
- **cgo politikası:** Windows ve Linux derlemeleri `CGO_ENABLED=0` ile yapılır ve CI'da zorunlu check'tir. macOS'ta Wails WebView bağlaması için cgo açıktır. Bağımlılık ağacımıza cgo gerektiren paket eklenmeyecek — bunun muhafızı Windows/Linux build'inin kırılmasıdır.
- **Bağımlılık yönü:** `app → sync → {imapx, store, auth}`. Alt katman üst katmanı import etmez. `sync` paketi `github.com/emersion/go-imap/v2`'yi **doğrudan import etmez**; `MailBackend` arayüzü üzerinden çalışır.
- **Sır saklama:** Parola, access token veya refresh token **hiçbir koşulda** SQLite'a veya `config.json`'a yazılmaz. Yalnızca `SecretStore` üzerinden OS anahtarlığına gider.
- **Veri dizini:** Windows `%APPDATA%\nexus-mail\`, macOS `~/Library/Application Support/nexus-mail/`, Linux `$XDG_DATA_HOME/nexus-mail/` (yoksa `~/.local/share/nexus-mail/`).
- **Test disiplini:** Her görev testle başlar. `go test -race ./...` yeşil olmadan commit yapılmaz.
- **Kod dili:** Tanımlayıcılar, yorumlar ve commit mesajları İngilizce (açık kaynak proje). Kullanıcıya görünen arayüz metinleri Türkçe ve İngilizce olarak i18n dosyalarında tutulur; M1'de yalnızca İngilizce dosya doldurulur.

## Dosya Yapısı

Görev sınırları bu yapıdan türetilmiştir. Her dosyanın tek bir sorumluluğu var.

```
go.mod
Taskfile.yml                          Wails v3 tarafından üretilir
.golangci.yml                         depguard katman kuralları
.github/workflows/ci.yml              lint / test / build matrisi
main.go                               Wails uygulama girişi, servis kaydı

internal/paths/paths.go               Platforma göre veri dizini çözümleme
internal/store/store.go               Açma, pragma'lar, okuma+yazma havuzları
internal/store/migrate.go             Sıralı migration uygulayıcı
internal/store/migrations/001_init.sql
internal/store/accounts.go            Account CRUD
internal/store/folders.go             Folder CRUD + UIDVALIDITY sıfırlama
internal/store/messages.go            Message toplu yazma + listeleme
internal/store/bodies.go              Gövde okuma/yazma

internal/auth/secretstore.go          SecretStore arayüzü
internal/auth/keyring_store.go        OS anahtarlığı uygulaması
internal/auth/file_store.go           Argon2id + AES-GCM yedek uygulama
internal/auth/provider.go             CredentialProvider arayüzü + AuthKind
internal/auth/xoauth2.go              XOAUTH2 sasl.Client (go-sasl'da yok)
internal/auth/password_provider.go    PLAIN sağlayıcı
internal/auth/oauth_provider.go       OAuth sağlayıcı (Google + Microsoft)
internal/auth/loopback.go             RFC 8252 loopback + PKCE akışı
internal/auth/presets.go              Yaygın sağlayıcı host/port ön ayarları

internal/imapx/backend.go             MailBackend arayüzü + veri tipleri
internal/imapx/client.go              go-imap v2 uygulaması
internal/imapx/envelope.go            ENVELOPE/BODYSTRUCTURE → model dönüşümü

internal/sync/engine.go               Motor kurulumu, hesap yaşam döngüsü
internal/sync/initial.go              İlk senkron (klasörler + başlıklar)
internal/sync/body.go                 Talep üzerine gövde indirme
internal/sync/errors.go               Hata sınıflandırma

internal/app/service.go               Wails servis metotları
internal/app/events.go                Olay adları ve yayın yardımcıları

frontend/src/lib/bindings/            Wails tarafından üretilir
frontend/src/store/useMailStore.ts    Zustand global durum
frontend/src/components/Layout.tsx    Üç sütunlu iskelet + tema
frontend/src/components/FolderList.tsx
frontend/src/components/MessageList.tsx   TanStack Virtual
frontend/src/components/MessageView.tsx   İzole iframe render
frontend/src/lib/sanitize.ts          DOMPurify + uzak kaynak yeniden yazma
frontend/src/components/AddAccount.tsx    Üç auth yolu için form akışı
```

---

### Task 1: Proje iskeleti ve mimari muhafızlar

Bu görev CI muhafızlarını **ilk günden** kurar. Sebebi şu: katman ihlali sonradan
temizlenmez, çünkü ihlal edildiği anda üzerine kod yazılır. Muhafız ilk commit'te
yoksa hiç olmaz.

**Files:**
- Create: `go.mod`, `main.go`, `.golangci.yml`, `.github/workflows/ci.yml`
- Create: `internal/store/store.go` (yalnızca paket bildirimi — depguard'ın test edilebilmesi için)
- Create: `internal/sync/engine.go` (aynı sebeple)

**Interfaces:**
- Consumes: yok (ilk görev)
- Produces: `nexusmail` modül adı; `internal/{store,sync,imapx,auth,app}` paket iskeletleri

- [ ] **Step 1: Wails v3 CLI'yi kur ve şablon adını doğrula**

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9
wails3 version
wails3 init -h
```

Beklenen: `wails3 version` çıktısı `v3.0.0-beta.9` içerir. `wails3 init -h` mevcut
şablonları listeler. React + TypeScript şablonunun tam adını bu çıktıdan al —
beta sürümlerinde şablon adları değişebiliyor, bu yüzden belgeye güvenmek yerine
CLI'ye soruyoruz. Aşağıdaki adımda `react-ts` varsayılıyor; çıktı farklıysa doğru
adı kullan.

- [ ] **Step 2: Projeyi üret**

```bash
cd /c/Users/ismet.kabatepe/MailBox
wails3 init -n nexus-mail -t react-ts
```

Üretilen dosyalar depo köküne gelmeli. Eğer CLI `nexus-mail/` alt dizini
oluşturursa içeriğini köke taşı (`mv nexus-mail/* nexus-mail/.* . ; rmdir nexus-mail`)
— `docs/` ve `.git/` zaten kökte olduğu için proje kökü değişmemeli.

- [ ] **Step 3: Modül adını düzelt ve derlemeyi doğrula**

`go.mod` içindeki modül adını `nexusmail` yap (CLI `nexus-mail` üretebilir; tire Go
import yollarında kullanılabilir ama depguard kurallarımız `nexusmail` bekliyor).
`main.go` içindeki import yollarını buna göre güncelle.

```bash
go mod tidy
CGO_ENABLED=0 go build ./...
```

Beklenen: hata yok.

- [ ] **Step 4: Katman iskeletlerini oluştur**

```bash
mkdir -p internal/{paths,store,auth,imapx,sync,app}
printf 'package store\n' > internal/store/store.go
printf 'package sync\n' > internal/sync/engine.go
printf 'package auth\n' > internal/auth/provider.go
printf 'package imapx\n' > internal/imapx/backend.go
printf 'package app\n' > internal/app/service.go
printf 'package paths\n' > internal/paths/paths.go
```

- [ ] **Step 5: depguard kurallarını yaz**

`.golangci.yml`:

```yaml
version: "2"

linters:
  enable:
    - depguard
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused

  settings:
    depguard:
      rules:
        store-layer:
          files: ["**/internal/store/**"]
          deny:
            - pkg: "nexusmail/internal/sync"
              desc: "store is a bottom layer; it must not depend on sync"
            - pkg: "nexusmail/internal/imapx"
              desc: "store is a bottom layer; it must not depend on imapx"
            - pkg: "nexusmail/internal/app"
              desc: "store is a bottom layer; it must not depend on app"
        imapx-layer:
          files: ["**/internal/imapx/**"]
          deny:
            - pkg: "nexusmail/internal/sync"
              desc: "imapx must not depend on sync"
            - pkg: "nexusmail/internal/app"
              desc: "imapx must not depend on app"
            - pkg: "nexusmail/internal/store"
              desc: "imapx speaks IMAP only; persistence belongs to sync"
        sync-layer:
          files: ["**/internal/sync/**"]
          deny:
            - pkg: "nexusmail/internal/app"
              desc: "sync must not depend on app"
            - pkg: "github.com/emersion/go-imap/v2"
              desc: "sync must go through the MailBackend interface so it stays testable against a fake server"
        auth-layer:
          files: ["**/internal/auth/**"]
          deny:
            - pkg: "nexusmail/internal/sync"
            - pkg: "nexusmail/internal/store"
            - pkg: "nexusmail/internal/app"
            - pkg: "nexusmail/internal/imapx"
```

- [ ] **Step 6: Muhafızın gerçekten çalıştığını kanıtla (kasıtlı ihlal)**

`internal/store/store.go` dosyasını geçici olarak şu hale getir:

```go
package store

import _ "nexusmail/internal/sync"
```

Sonra çalıştır:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
golangci-lint run ./internal/store/...
```

Beklenen: **FAIL** — `import 'nexusmail/internal/sync' is not allowed from list 'store-layer': store is a bottom layer; it must not depend on sync`

Bu adım atlanamaz. Yapılandırdığın ama tetiklendiğini görmediğin bir linter kuralı,
olmayan bir kuraldır.

- [ ] **Step 7: İhlali geri al ve lint'in geçtiğini doğrula**

```bash
printf 'package store\n' > internal/store/store.go
golangci-lint run ./...
```

Beklenen: PASS, çıktı boş.

- [ ] **Step 8: CI iş akışını yaz**

`.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: gofmt
        run: test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
      - name: go.mod is tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: Go tests
        run: go test -race ./...

  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - os: windows-latest
            cgo: "0"
          - os: ubuntu-latest
            cgo: "0"
          - os: macos-latest
            cgo: "1"
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: Linux WebKit dependencies
        if: matrix.os == 'ubuntu-latest'
        run: |
          sudo apt-get update
          sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
      - name: Install Wails CLI
        run: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9
      - name: Verify CLI matches library version
        shell: bash
        run: |
          lib=$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)
          cli=$(wails3 version | grep -oE 'v3\.[0-9]+\.[0-9]+[^ ]*' | head -1)
          echo "library=$lib cli=$cli"
          test "$lib" = "$cli"
      - name: Build
        env:
          CGO_ENABLED: ${{ matrix.cgo }}
        run: wails3 build
```

`build` job'ının Windows ve Linux ayakları GitHub'da **required check** olarak
işaretlenecek — cgo muhafızı bunlara dayanıyor. (Bu ayar depo ayarlarından yapılır,
kodda değil; README'ye not düşülecek.)

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "chore: scaffold Wails v3 project with architecture guards

Layer boundaries from the design doc are enforced by depguard in CI
rather than by convention. Verified the rule fires by introducing a
deliberate violation before committing.

CGO_ENABLED=0 is required on Windows and Linux, which doubles as the
guard proving our dependencies stay pure Go. macOS keeps cgo enabled
because the Wails WebView binding requires it."
```

---

### Task 2: Veri dizini ve veritabanı açma

**Files:**
- Create: `internal/paths/paths.go`, `internal/paths/paths_test.go`
- Create: `internal/store/store.go`, `internal/store/store_test.go`
- Create: `internal/store/migrate.go`
- Create: `internal/store/migrations/001_init.sql`

**Interfaces:**
- Consumes: `nexusmail` modül adı (Task 1)
- Produces:
  - `paths.DataDir() (string, error)` — platforma özgü veri dizini, oluşturulmuş halde
  - `store.Open(dir string) (*Store, error)`
  - `(*Store).Close() error`
  - `(*Store).Read() *sql.DB` — okuma havuzu
  - `(*Store).Write() *sql.DB` — yazma havuzu, tek bağlantı

- [ ] **Step 1: Veri dizini için başarısız test yaz**

`internal/paths/paths_test.go`:

```go
package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirIsCreatedAndNamed(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("APPDATA", base)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	if !strings.HasSuffix(filepath.Clean(dir), "nexus-mail") {
		t.Errorf("expected path to end with nexus-mail, got %q", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected a directory, got a file")
	}
}
```

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/paths/ -run TestDataDirIsCreatedAndNamed -v
```

Beklenen: FAIL — `undefined: DataDir`

- [ ] **Step 3: `paths.DataDir` implementasyonu**

`internal/paths/paths.go`:

```go
// Package paths resolves per-platform locations for user data.
package paths

import (
	"os"
	"path/filepath"
)

const appDirName = "nexus-mail"

// DataDir returns the directory holding the database, attachments, config and
// logs, creating it if necessary. os.UserConfigDir already implements the
// per-platform rules we need: %APPDATA% on Windows, ~/Library/Application
// Support on macOS, and $XDG_CONFIG_HOME on Linux. Linux is overridden below to
// use $XDG_DATA_HOME instead, because this is data rather than configuration.
func DataDir() (string, error) {
	base, err := platformBase()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
```

`internal/paths/paths_unix.go`:

```go
//go:build linux

package paths

import (
	"os"
	"path/filepath"
)

func platformBase() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}
```

`internal/paths/paths_other.go`:

```go
//go:build !linux

package paths

import "os"

func platformBase() (string, error) {
	return os.UserConfigDir()
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

```bash
go test ./internal/paths/ -v
```

Beklenen: PASS

- [ ] **Step 5: Şema migration'ını yaz**

`internal/store/migrations/001_init.sql` — tasarım dokümanının §5'indeki şemanın
birebir aynısı, sonuna FTS5 tablosu eklenmiş:

```sql
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
  uid_validity    INTEGER NOT NULL DEFAULT 0,
  uid_next        INTEGER NOT NULL DEFAULT 0,
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
  date            INTEGER NOT NULL DEFAULT 0,
  internal_date   INTEGER NOT NULL DEFAULT 0,
  size            INTEGER NOT NULL DEFAULT 0,
  snippet         TEXT NOT NULL DEFAULT '',
  flags           TEXT NOT NULL DEFAULT '',
  has_attachments INTEGER NOT NULL DEFAULT 0,
  body_fetched    INTEGER NOT NULL DEFAULT 0,
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
  kind            TEXT NOT NULL,
  payload         TEXT NOT NULL,
  state           TEXT NOT NULL,
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT NOT NULL DEFAULT '',
  created_at      INTEGER NOT NULL,
  next_attempt_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_operations_queue ON operations(account_id, state, next_attempt_at);

CREATE VIRTUAL TABLE fts_messages USING fts5(
  subject,
  from_addr,
  snippet,
  body,
  content=''
);
```

`content=''` ile FTS5 "contentless" modda kurulur: metni ikinci kez saklamaz,
yalnızca indeksi tutar. Satırlar `rowid` üzerinden `messages.id` ile eşleşir.

- [ ] **Step 6: Veritabanı açma için başarısız test yaz**

`internal/store/store_test.go`:

```go
package store

import (
	"testing"
)

func TestOpenAppliesMigrationsAndPragmas(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.Read().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode query: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}

	var fk int
	if err := s.Read().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys query: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	for _, table := range []string{"accounts", "folders", "messages", "message_bodies", "attachments", "operations", "fts_messages"} {
		var n int
		err := s.Read().QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE name = ?", table).Scan(&n)
		if err != nil {
			t.Fatalf("sqlite_master query for %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s: found %d entries, want 1", table, n)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	s1.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open() error: %v", err)
	}
	defer s2.Close()

	var version int
	if err := s2.Read().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version query: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}
}

func TestWritePoolHasSingleConnection(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	if got := s.Write().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write pool MaxOpenConnections = %d, want 1", got)
	}
}
```

- [ ] **Step 7: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/store/ -v
```

Beklenen: FAIL — `undefined: Open`

- [ ] **Step 8: `store.Open` ve migration uygulayıcısını yaz**

`internal/store/store.go`:

```go
// Package store owns all SQLite persistence for Nexus Mail.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store holds two connection pools over the same database file.
//
// The design doc calls for "a single writer goroutine plus a read pool". We get
// the same guarantee more cheaply: a write pool capped at one connection
// serialises writes through database/sql itself, with no channel plumbing to
// write or test. Reads go to a separate pool and run concurrently under WAL.
type Store struct {
	read  *sql.DB
	write *sql.DB
}

const dsnOptions = "?_pragma=journal_mode(WAL)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=synchronous(NORMAL)"

// Open opens (creating if needed) mail.db inside dir and applies migrations.
func Open(dir string) (*Store, error) {
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "mail.db")) + dsnOptions

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, fmt.Errorf("open read pool: %w", err)
	}
	read.SetMaxOpenConns(4)

	s := &Store{read: read, write: write}

	if err := migrate(s.write); err != nil {
		s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Read returns the concurrent read pool.
func (s *Store) Read() *sql.DB { return s.read }

// Write returns the serialised write pool.
func (s *Store) Write() *sql.DB { return s.write }

func (s *Store) Close() error {
	rerr := s.read.Close()
	werr := s.write.Close()
	if werr != nil {
		return werr
	}
	return rerr
}
```

`internal/store/migrate.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrate applies every migration whose ordinal exceeds PRAGMA user_version,
// in filename order, each inside its own transaction. user_version is the
// bookkeeping mechanism, so no extra table is needed.
func migrate(db *sql.DB) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	for i, name := range names {
		version := i + 1
		if version <= current {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		// PRAGMA does not accept placeholders.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set user_version after %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 9: Testlerin geçtiğini doğrula**

```bash
go test ./internal/store/ -race -v
```

Beklenen: PASS — üç test de yeşil.

- [ ] **Step 10: Commit**

```bash
git add internal/paths internal/store
git commit -m "feat(store): open SQLite with WAL and apply embedded migrations

Two pools over one file: writes are serialised by capping the write
pool at one connection, reads run concurrently under WAL. This gives
the design doc's single-writer guarantee without a writer goroutine.

Schema matches the design doc, plus a contentless FTS5 table that is
created and populated in P0 so P3 needs no full reindex."
```

---

### Task 3: Eşzamanlılık altında kilit çekişmesi olmadığını kanıtla

Bu görev yeni işlevsellik eklemez; tasarımın en kırılgan varsayımını kanıtlar.
`go test -race` bu sınıfı yakalayamaz, çünkü SQLITE_BUSY bir veri yarışı değil kilit
çekişmesidir.

**Files:**
- Create: `internal/store/concurrency_test.go`

**Interfaces:**
- Consumes: `store.Open`, `(*Store).Read()`, `(*Store).Write()` (Task 2)
- Produces: yok (yalnızca test)

- [ ] **Step 1: Stres testini yaz**

`internal/store/concurrency_test.go`:

```go
package store

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoLockContentionUnderLoad asserts that 50 concurrent readers never
// observe SQLITE_BUSY while a writer works continuously. Passing this by
// leaning on busy_timeout alone is not good enough: writes must be
// serialised, which the single-connection write pool guarantees.
func TestNoLockContentionUnderLoad(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	if _, err := s.Write().Exec(
		`INSERT INTO accounts (email, provider, auth_kind, imap_host, imap_port, secret_ref, created_at)
		 VALUES ('a@example.com', 'generic', 'password', 'localhost', 993, 'ref', 0)`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := s.Write().Exec(
		`INSERT INTO folders (account_id, name, path) VALUES (1, 'INBOX', 'INBOX')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	const (
		readers  = 50
		duration = 3 * time.Second
	)

	var (
		wg       sync.WaitGroup
		writes   atomic.Int64
		busyHits atomic.Int64
		stop     = make(chan struct{})
	)

	recordIfBusy := func(err error) {
		if err == nil {
			return
		}
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "database is locked") ||
			strings.Contains(msg, "sqlite_busy") ||
			strings.Contains(msg, "database table is locked") {
			busyHits.Add(1)
			return
		}
		t.Errorf("unexpected error: %v", err)
	}

	// One writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for uid := int64(1); ; uid++ {
			select {
			case <-stop:
				return
			default:
			}
			_, err := s.Write().Exec(
				`INSERT INTO messages (account_id, folder_id, uid, subject, internal_date)
				 VALUES (1, 1, ?, ?, ?)`, uid, "subject", uid)
			recordIfBusy(err)
			if err == nil {
				writes.Add(1)
			}
		}
	}()

	// Fifty readers.
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rows, err := s.Read().Query(
					`SELECT id, subject FROM messages
					 WHERE folder_id = 1 ORDER BY internal_date DESC LIMIT 50`)
				if err != nil {
					recordIfBusy(err)
					continue
				}
				for rows.Next() {
					var id int64
					var subject string
					if err := rows.Scan(&id, &subject); err != nil {
						t.Errorf("scan: %v", err)
						break
					}
				}
				rows.Close()
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	if got := busyHits.Load(); got != 0 {
		t.Errorf("observed %d lock-contention errors, want 0", got)
	}
	if got := writes.Load(); got == 0 {
		t.Fatal("writer made no progress; test proves nothing")
	}
	t.Logf("completed %d writes with %d readers and no contention", writes.Load(), readers)

	// The row count must match exactly what the writer reported.
	var count int64
	if err := s.Read().QueryRow(`SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != writes.Load() {
		t.Errorf("row count = %d, writer reported %d successful writes", count, writes.Load())
	}
}
```

- [ ] **Step 2: Testi çalıştır**

```bash
go test ./internal/store/ -race -run TestNoLockContentionUnderLoad -v
```

Beklenen: PASS. Log satırında binlerce yazma görünmeli — birkaç yüzse yazıcı
bloke olmuş demektir ve pragma'ları gözden geçir.

Eğer `observed N lock-contention errors` ile başarısız olursa, sebep neredeyse her
zaman şudur: yazma işlemi `s.Read()` havuzu üzerinden yapılmıştır. Tüm `INSERT`,
`UPDATE`, `DELETE` çağrıları `s.Write()` kullanmalıdır.

- [ ] **Step 3: Commit**

```bash
git add internal/store/concurrency_test.go
git commit -m "test(store): prove no lock contention with 50 readers and 1 writer

go test -race cannot catch SQLITE_BUSY, which is lock contention rather
than a data race, so this needs its own stress test. Also asserts the
final row count matches the writer's own tally."
```

---

### Task 4: Paylaşılan veri tipleri ve repository'ler

Bu görev dosya haritasına bir ekleme yapıyor: `internal/model`. Sebebi şu — `store`
ve `imapx` aynı kavramları (klasör, mesaj başlığı) temsil ediyor. Her katman kendi
struct'ını tanımlarsa `sync` motoru sürekli birbirinin kopyası olan tipler arasında
dönüşüm yapmak zorunda kalır. `model` yalnızca veri tipi içeren bir yaprak paket
olur; hiçbir şeyi import etmez, herkes onu import eder.

**Files:**
- Create: `internal/model/model.go`
- Create: `internal/store/accounts.go`, `internal/store/folders.go`, `internal/store/messages.go`
- Create: `internal/store/accounts_test.go`, `internal/store/folders_test.go`, `internal/store/messages_test.go`
- Modify: `.golangci.yml` (model katmanı kuralı)

**Interfaces:**
- Consumes: `store.Open`, `(*Store).Read()`, `(*Store).Write()` (Task 2)
- Produces:
  - `model.Account`, `model.Folder`, `model.Message`, `model.Address`
  - `model.AuthKind` (`AuthPassword`, `AuthOAuth`), `model.Provider` (`ProviderMicrosoft`, `ProviderGoogle`, `ProviderGeneric`)
  - `(*Store).InsertAccount(ctx, model.Account) (int64, error)`
  - `(*Store).ListAccounts(ctx) ([]model.Account, error)`
  - `(*Store).UpsertFolders(ctx, accountID int64, []model.Folder) error`
  - `(*Store).ListFolders(ctx, accountID int64) ([]model.Folder, error)`
  - `(*Store).ResetFolder(ctx, folderID int64, newUIDValidity uint32) error`
  - `(*Store).UpsertMessages(ctx, folderID int64, []model.Message) error`
  - `(*Store).ListMessages(ctx, folderID int64, limit, offset int) ([]model.Message, error)`

- [ ] **Step 1: Veri tiplerini yaz**

`internal/model/model.go`:

```go
// Package model holds the data types shared across layers. It imports nothing
// from the rest of the project, so every layer can depend on it freely.
package model

import "time"

type AuthKind string

const (
	AuthPassword AuthKind = "password"
	AuthOAuth    AuthKind = "oauth"
)

type Provider string

const (
	ProviderMicrosoft Provider = "microsoft"
	ProviderGoogle    Provider = "google"
	ProviderGeneric   Provider = "generic"
)

type Account struct {
	ID          int64
	Email       string
	DisplayName string
	Provider    Provider
	AuthKind    AuthKind
	IMAPHost    string
	IMAPPort    int
	SMTPHost    string
	SMTPPort    int
	// SecretRef names the entry in the SecretStore. The secret itself is never
	// stored in the database.
	SecretRef string
	CreatedAt time.Time
}

type Folder struct {
	ID          int64
	AccountID   int64
	Name        string
	Path        string
	Delimiter   string
	Attributes  []string
	UIDValidity uint32
	UIDNext     uint32
	// HighestModSeq is zero when the server does not advertise CONDSTORE.
	HighestModSeq uint64
	TotalCount    int
	UnreadCount   int
	LastSyncedAt  time.Time
}

type Address struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

type Message struct {
	ID             int64
	AccountID      int64
	FolderID       int64
	UID            uint32
	MessageID      string
	ThreadID       string
	InReplyTo      string
	References     []string
	Subject        string
	From           Address
	To             []Address
	Cc             []Address
	Date           time.Time
	InternalDate   time.Time
	Size           int64
	Snippet        string
	Flags          []string
	HasAttachments bool
	BodyFetched    bool
}

// HasFlag reports whether f is present, case-insensitively, since servers vary
// in how they capitalise system flags.
func (m Message) HasFlag(f string) bool {
	for _, got := range m.Flags {
		if equalFold(got, f) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: depguard'a model kuralını ekle**

`.golangci.yml` içindeki `rules:` bloğuna ekle:

```yaml
        model-layer:
          files: ["**/internal/model/**"]
          deny:
            - pkg: "nexusmail/internal/store"
              desc: "model is a leaf package and must import nothing from the project"
            - pkg: "nexusmail/internal/sync"
            - pkg: "nexusmail/internal/imapx"
            - pkg: "nexusmail/internal/auth"
            - pkg: "nexusmail/internal/app"
```

- [ ] **Step 3: Hesap repository'si için başarısız test yaz**

`internal/store/accounts_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func TestInsertAndListAccounts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	want := model.Account{
		Email:       "user@example.com",
		DisplayName: "Test User",
		Provider:    model.ProviderGeneric,
		AuthKind:    model.AuthPassword,
		IMAPHost:    "imap.example.com",
		IMAPPort:    993,
		SecretRef:   "account:user@example.com",
		CreatedAt:   time.Unix(1700000000, 0),
	}

	id, err := s.InsertAccount(ctx, want)
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}
	if id == 0 {
		t.Fatal("InsertAccount() returned id 0")
	}

	got, err := s.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListAccounts() returned %d accounts, want 1", len(got))
	}
	if got[0].Email != want.Email {
		t.Errorf("Email = %q, want %q", got[0].Email, want.Email)
	}
	if got[0].IMAPPort != want.IMAPPort {
		t.Errorf("IMAPPort = %d, want %d", got[0].IMAPPort, want.IMAPPort)
	}
	if got[0].Provider != want.Provider {
		t.Errorf("Provider = %q, want %q", got[0].Provider, want.Provider)
	}
	if !got[0].CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got[0].CreatedAt, want.CreatedAt)
	}
}

func TestInsertAccountRejectsDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	a := model.Account{
		Email: "dup@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "r", CreatedAt: time.Unix(1, 0),
	}
	if _, err := s.InsertAccount(ctx, a); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := s.InsertAccount(ctx, a); err == nil {
		t.Error("second insert with the same email succeeded, want a UNIQUE violation")
	}
}
```

- [ ] **Step 4: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/store/ -run TestInsertAndListAccounts -v
```

Beklenen: FAIL — `s.InsertAccount undefined`

- [ ] **Step 5: Hesap repository'sini yaz**

`internal/store/accounts.go`:

```go
package store

import (
	"context"
	"time"

	"nexusmail/internal/model"
)

func (s *Store) InsertAccount(ctx context.Context, a model.Account) (int64, error) {
	res, err := s.write.ExecContext(ctx,
		`INSERT INTO accounts
		   (email, display_name, provider, auth_kind, imap_host, imap_port,
		    smtp_host, smtp_port, secret_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Email, a.DisplayName, string(a.Provider), string(a.AuthKind),
		a.IMAPHost, a.IMAPPort, a.SMTPHost, a.SMTPPort,
		a.SecretRef, a.CreatedAt.Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, email, display_name, provider, auth_kind, imap_host, imap_port,
		        smtp_host, smtp_port, secret_ref, created_at
		 FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Account
	for rows.Next() {
		var a model.Account
		var provider, authKind string
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.Email, &a.DisplayName, &provider, &authKind,
			&a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort,
			&a.SecretRef, &createdAt); err != nil {
			return nil, err
		}
		a.Provider = model.Provider(provider)
		a.AuthKind = model.AuthKind(authKind)
		a.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}
```

- [ ] **Step 6: Testlerin geçtiğini doğrula**

```bash
go test ./internal/store/ -race -run 'TestInsertAndListAccounts|TestInsertAccountRejectsDuplicateEmail' -v
```

Beklenen: PASS

- [ ] **Step 7: Klasör repository'si için başarısız test yaz**

`internal/store/folders_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func seedAccount(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.InsertAccount(context.Background(), model.Account{
		Email: "seed@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "r", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func TestUpsertFoldersIsIdempotentAndUpdatesCounts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()
	acct := seedAccount(t, s)

	first := []model.Folder{
		{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 10, UIDNext: 100, TotalCount: 5, UnreadCount: 2},
		{Path: "Sent", Name: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}, UIDValidity: 11},
	}
	if err := s.UpsertFolders(ctx, acct, first); err != nil {
		t.Fatalf("first UpsertFolders() error: %v", err)
	}

	// Re-running with changed counts must update rather than duplicate.
	second := []model.Folder{
		{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 10, UIDNext: 140, TotalCount: 9, UnreadCount: 3},
		{Path: "Sent", Name: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}, UIDValidity: 11},
	}
	if err := s.UpsertFolders(ctx, acct, second); err != nil {
		t.Fatalf("second UpsertFolders() error: %v", err)
	}

	got, err := s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListFolders() returned %d folders, want 2", len(got))
	}

	byPath := map[string]model.Folder{}
	for _, f := range got {
		byPath[f.Path] = f
	}
	inbox := byPath["INBOX"]
	if inbox.TotalCount != 9 {
		t.Errorf("INBOX TotalCount = %d, want 9", inbox.TotalCount)
	}
	if inbox.UIDNext != 140 {
		t.Errorf("INBOX UIDNext = %d, want 140", inbox.UIDNext)
	}
	sent := byPath["Sent"]
	if len(sent.Attributes) != 1 || sent.Attributes[0] != "\\Sent" {
		t.Errorf("Sent Attributes = %v, want [\\Sent]", sent.Attributes)
	}
}

func TestResetFolderDeletesMessagesAndSetsNewUIDValidity(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()
	acct := seedAccount(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX", UIDValidity: 10},
		{Path: "Other", Name: "Other", UIDValidity: 20},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, err := s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	var inboxID, otherID int64
	for _, f := range folders {
		switch f.Path {
		case "INBOX":
			inboxID = f.ID
		case "Other":
			otherID = f.ID
		}
	}

	for _, fid := range []int64{inboxID, otherID} {
		if err := s.UpsertMessages(ctx, fid, []model.Message{
			{AccountID: acct, FolderID: fid, UID: 1, Subject: "one", InternalDate: time.Unix(1, 0)},
			{AccountID: acct, FolderID: fid, UID: 2, Subject: "two", InternalDate: time.Unix(2, 0)},
		}); err != nil {
			t.Fatalf("UpsertMessages() error: %v", err)
		}
	}

	if err := s.ResetFolder(ctx, inboxID, 99); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}

	inboxMsgs, err := s.ListMessages(ctx, inboxID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(inbox) error: %v", err)
	}
	if len(inboxMsgs) != 0 {
		t.Errorf("inbox still holds %d messages after reset, want 0", len(inboxMsgs))
	}

	// The critical assertion: resetting one folder must not touch its siblings.
	otherMsgs, err := s.ListMessages(ctx, otherID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(other) error: %v", err)
	}
	if len(otherMsgs) != 2 {
		t.Errorf("sibling folder holds %d messages, want 2 — reset leaked across folders", len(otherMsgs))
	}

	folders, err = s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() after reset error: %v", err)
	}
	for _, f := range folders {
		if f.ID == inboxID {
			if f.UIDValidity != 99 {
				t.Errorf("inbox UIDValidity = %d, want 99", f.UIDValidity)
			}
			if f.HighestModSeq != 0 {
				t.Errorf("inbox HighestModSeq = %d, want 0 after reset", f.HighestModSeq)
			}
		}
	}
}
```

- [ ] **Step 8: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/store/ -run 'TestUpsertFolders|TestResetFolder' -v
```

Beklenen: FAIL — `s.UpsertFolders undefined`

- [ ] **Step 9: Klasör repository'sini yaz**

`internal/store/folders.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"nexusmail/internal/model"
)

// UpsertFolders writes the folder list for an account. Server-reported counts
// overwrite local ones; UIDVALIDITY handling is deliberately NOT done here —
// detecting a change and resetting is the sync engine's decision, made through
// ResetFolder.
func (s *Store) UpsertFolders(ctx context.Context, accountID int64, folders []model.Folder) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO folders
		   (account_id, name, path, delimiter, attributes, uid_validity, uid_next,
		    highest_modseq, total_count, unread_count, last_synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, path) DO UPDATE SET
		   name           = excluded.name,
		   delimiter      = excluded.delimiter,
		   attributes     = excluded.attributes,
		   uid_validity   = excluded.uid_validity,
		   uid_next       = excluded.uid_next,
		   total_count    = excluded.total_count,
		   unread_count   = excluded.unread_count,
		   last_synced_at = excluded.last_synced_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range folders {
		attrs, err := json.Marshal(f.Attributes)
		if err != nil {
			return err
		}
		delim := f.Delimiter
		if delim == "" {
			delim = "/"
		}
		if _, err := stmt.ExecContext(ctx,
			accountID, f.Name, f.Path, delim, string(attrs),
			f.UIDValidity, f.UIDNext, f.HighestModSeq,
			f.TotalCount, f.UnreadCount, f.LastSyncedAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, account_id, name, path, delimiter, attributes, uid_validity,
		        uid_next, highest_modseq, total_count, unread_count, last_synced_at
		 FROM folders WHERE account_id = ? ORDER BY path`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Folder
	for rows.Next() {
		var f model.Folder
		var attrs string
		var synced int64
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Name, &f.Path, &f.Delimiter,
			&attrs, &f.UIDValidity, &f.UIDNext, &f.HighestModSeq,
			&f.TotalCount, &f.UnreadCount, &synced); err != nil {
			return nil, err
		}
		if attrs != "" && attrs != "null" {
			if err := json.Unmarshal([]byte(attrs), &f.Attributes); err != nil {
				return nil, err
			}
		}
		f.LastSyncedAt = time.Unix(synced, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ResetFolder discards every locally cached message for one folder and records
// the server's new UIDVALIDITY. Called when the server changes UIDVALIDITY,
// which invalidates every UID we hold for that folder. Scoped to a single
// folder on purpose: a sibling folder's cache stays valid.
func (s *Store) ResetFolder(ctx context.Context, folderID int64, newUIDValidity uint32) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear FTS rows before the messages disappear; a contentless FTS5 table
	// cannot be cleaned up by foreign keys.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet, body)
		 SELECT 'delete', id, '', '', '', '' FROM messages WHERE folder_id = ?`,
		folderID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE folder_id = ?`, folderID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE folders SET uid_validity = ?, uid_next = 0, highest_modseq = 0,
		                    last_synced_at = 0
		 WHERE id = ?`, newUIDValidity, folderID); err != nil {
		return err
	}
	return tx.Commit()
}

func joinAttrs(attrs []string) string { return strings.Join(attrs, " ") }
```

`joinAttrs` şu an kullanılmıyor; `unused` linter'ı bunu yakalayacak. Fonksiyonu
yazma — yukarıdaki bloktan çıkar. (Bu satır kasıtlı bir tuzak değil, planın
gerçekliği: linter'ı çalıştırdığında bu tür ölü kodu temizlemek adımın parçası.)

- [ ] **Step 10: Mesaj repository'si için başarısız test yaz**

`internal/store/messages_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func TestUpsertMessagesDeduplicatesByUID(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()
	acct := seedAccount(t, s)
	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, _ := s.ListFolders(ctx, acct)
	fid := folders[0].ID

	msgs := []model.Message{{
		AccountID: acct, FolderID: fid, UID: 42,
		MessageID: "<a@example.com>", Subject: "Invoice",
		From:      model.Address{Name: "Ali", Addr: "ali@example.com"},
		To:        []model.Address{{Addr: "me@example.com"}},
		Flags:     []string{"\\Seen"},
		Date:      time.Unix(1700000000, 0),
		InternalDate: time.Unix(1700000001, 0),
		Size:      2048, Snippet: "Please find attached", HasAttachments: true,
	}}

	if err := s.UpsertMessages(ctx, fid, msgs); err != nil {
		t.Fatalf("first UpsertMessages() error: %v", err)
	}
	// Writing the same UID again must update, not duplicate. This is the
	// behaviour that keeps reconnects from doubling the mailbox.
	msgs[0].Flags = []string{"\\Seen", "\\Flagged"}
	if err := s.UpsertMessages(ctx, fid, msgs); err != nil {
		t.Fatalf("second UpsertMessages() error: %v", err)
	}

	got, err := s.ListMessages(ctx, fid, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListMessages() returned %d messages, want 1", len(got))
	}
	m := got[0]
	if m.Subject != "Invoice" {
		t.Errorf("Subject = %q, want Invoice", m.Subject)
	}
	if m.From.Name != "Ali" || m.From.Addr != "ali@example.com" {
		t.Errorf("From = %+v, want {Ali ali@example.com}", m.From)
	}
	if len(m.To) != 1 || m.To[0].Addr != "me@example.com" {
		t.Errorf("To = %+v, want one address me@example.com", m.To)
	}
	if !m.HasFlag("\\flagged") {
		t.Errorf("Flags = %v, want the second write's \\Flagged to be present", m.Flags)
	}
	if !m.HasAttachments {
		t.Error("HasAttachments = false, want true")
	}
}

func TestListMessagesOrdersNewestFirstAndPaginates(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()
	acct := seedAccount(t, s)
	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, _ := s.ListFolders(ctx, acct)
	fid := folders[0].ID

	var batch []model.Message
	for i := 1; i <= 5; i++ {
		batch = append(batch, model.Message{
			AccountID: acct, FolderID: fid, UID: uint32(i),
			Subject:      string(rune('A' + i - 1)),
			InternalDate: time.Unix(int64(1700000000+i), 0),
		})
	}
	if err := s.UpsertMessages(ctx, fid, batch); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	page, err := s.ListMessages(ctx, fid, 2, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page length = %d, want 2", len(page))
	}
	if page[0].UID != 5 || page[1].UID != 4 {
		t.Errorf("first page UIDs = %d,%d; want 5,4 (newest first)", page[0].UID, page[1].UID)
	}

	next, err := s.ListMessages(ctx, fid, 2, 2)
	if err != nil {
		t.Fatalf("ListMessages(offset) error: %v", err)
	}
	if len(next) != 2 || next[0].UID != 3 {
		t.Errorf("second page = %+v, want it to start at UID 3", next)
	}
}
```

- [ ] **Step 11: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/store/ -run TestUpsertMessages -v
```

Beklenen: FAIL — `s.UpsertMessages undefined`

- [ ] **Step 12: Mesaj repository'sini yaz**

`internal/store/messages.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"time"

	"nexusmail/internal/model"
)

// UpsertMessages writes a batch of message headers in one transaction. The
// UNIQUE(account_id, folder_id, uid) constraint plus ON CONFLICT is what makes
// reconnects safe: the same UID arriving twice updates the row instead of
// duplicating it.
func (s *Store) UpsertMessages(ctx context.Context, folderID int64, msgs []model.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO messages
		   (account_id, folder_id, uid, message_id, thread_id, in_reply_to, refs,
		    subject, from_name, from_addr, to_addrs, cc_addrs, date, internal_date,
		    size, snippet, flags, has_attachments, body_fetched)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, folder_id, uid) DO UPDATE SET
		   flags           = excluded.flags,
		   subject         = excluded.subject,
		   snippet         = excluded.snippet,
		   thread_id       = excluded.thread_id,
		   has_attachments = excluded.has_attachments`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ftsStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO fts_messages(rowid, subject, from_addr, snippet, body)
		 VALUES (?, ?, ?, ?, '')`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	for _, m := range msgs {
		to, err := json.Marshal(m.To)
		if err != nil {
			return err
		}
		cc, err := json.Marshal(m.Cc)
		if err != nil {
			return err
		}
		refs, err := json.Marshal(m.References)
		if err != nil {
			return err
		}
		flags, err := json.Marshal(m.Flags)
		if err != nil {
			return err
		}
		res, err := stmt.ExecContext(ctx,
			m.AccountID, folderID, m.UID, m.MessageID, m.ThreadID, m.InReplyTo,
			string(refs), m.Subject, m.From.Name, m.From.Addr, string(to), string(cc),
			m.Date.Unix(), m.InternalDate.Unix(), m.Size, m.Snippet, string(flags),
			boolToInt(m.HasAttachments), boolToInt(m.BodyFetched))
		if err != nil {
			return err
		}
		// Index only genuinely new rows; RowsAffected is 1 on insert and 2 on
		// an upsert that replaced a row in SQLite's ON CONFLICT implementation.
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := ftsStmt.ExecContext(ctx, id, m.Subject, m.From.Addr, m.Snippet); err != nil {
			// A duplicate rowid means the message was already indexed; ignore.
			continue
		}
	}
	return tx.Commit()
}

func (s *Store) ListMessages(ctx context.Context, folderID int64, limit, offset int) ([]model.Message, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, account_id, folder_id, uid, message_id, thread_id, in_reply_to,
		        refs, subject, from_name, from_addr, to_addrs, cc_addrs, date,
		        internal_date, size, snippet, flags, has_attachments, body_fetched
		 FROM messages
		 WHERE folder_id = ?
		 ORDER BY internal_date DESC, uid DESC
		 LIMIT ? OFFSET ?`, folderID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Message
	for rows.Next() {
		var m model.Message
		var refs, to, cc, flags string
		var date, internal int64
		var hasAtt, bodyFetched int
		if err := rows.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.MessageID,
			&m.ThreadID, &m.InReplyTo, &refs, &m.Subject, &m.From.Name, &m.From.Addr,
			&to, &cc, &date, &internal, &m.Size, &m.Snippet, &flags,
			&hasAtt, &bodyFetched); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(refs, &m.References); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(to, &m.To); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(cc, &m.Cc); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(flags, &m.Flags); err != nil {
			return nil, err
		}
		m.Date = time.Unix(date, 0)
		m.InternalDate = time.Unix(internal, 0)
		m.HasAttachments = hasAtt == 1
		m.BodyFetched = bodyFetched == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

func unmarshalIfSet(s string, dst any) error {
	if s == "" || s == "null" {
		return nil
	}
	return json.Unmarshal([]byte(s), dst)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 13: Tüm store testlerini çalıştır**

```bash
go test ./internal/store/ -race -v
golangci-lint run ./...
```

Beklenen: tüm testler PASS, lint temiz. Lint `joinAttrs` gibi kullanılmayan bir
fonksiyon bildirirse sil.

- [ ] **Step 14: Commit**

```bash
git add internal/model internal/store .golangci.yml
git commit -m "feat(store): add shared model types and repositories

Introduces internal/model as a leaf package so store and imapx describe
the same concepts without the sync engine converting between near-identical
structs. depguard forbids model from importing anything in-project.

ResetFolder is scoped to one folder and asserted not to touch siblings:
a UIDVALIDITY change invalidates one mailbox, not the whole account."
```

---

### Task 5: SecretStore — anahtarlık ve şifreli dosya yedeği

**Kapsam kararı:** Yedek dosya deposu M1'de ana parolayı `NEXUSMAIL_MASTER_PASSWORD`
ortam değişkeninden okur. Kullanıcıya parola soran arayüz M2'ye bırakılmıştır. Bu
bilinçli bir sınır: yedek yol Linux CI'da gerçekten çalışır ve test edilir, ama UI
yüzeyi henüz yok. Ortam değişkeni yoksa `Default` anlamlı bir hata döndürür.

**Files:**
- Create: `internal/auth/secretstore.go`, `internal/auth/keyring_store.go`, `internal/auth/file_store.go`
- Create: `internal/auth/file_store_test.go`, `internal/auth/secretstore_test.go`

**Interfaces:**
- Consumes: yok
- Produces:
  - `auth.SecretStore` arayüzü: `Get(ref string) (string, error)`, `Set(ref, secret string) error`, `Delete(ref string) error`
  - `auth.ErrNotFound` — sentinel hata
  - `auth.ErrSecretTooLong` — Windows Credential Manager sınırı için
  - `auth.NewKeyringStore() (SecretStore, error)`
  - `auth.NewFileStore(dir, masterPassword string) (SecretStore, error)`
  - `auth.Default(dir string) (SecretStore, error)`

- [ ] **Step 1: Arayüzü ve sentinel hataları yaz**

`internal/auth/secretstore.go`:

```go
// Package auth acquires and stores credentials. Secrets never reach the
// database or config file: they live only in the OS keyring, or in an
// encrypted file when no keyring is available.
package auth

import (
	"errors"
	"os"
)

var (
	// ErrNotFound reports that no secret is stored under the given ref.
	ErrNotFound = errors.New("auth: secret not found")
	// ErrSecretTooLong reports a secret the backend cannot hold. Windows
	// Credential Manager caps a credential blob at roughly 2.5 KB.
	ErrSecretTooLong = errors.New("auth: secret exceeds backend size limit")
)

// maxSecretBytes is the smallest limit across supported backends, applied
// uniformly so a secret that works on one platform works on all of them.
const maxSecretBytes = 2048

// SecretStore stores one secret per ref. A ref is an opaque key such as
// "account:user@example.com".
type SecretStore interface {
	Get(ref string) (string, error)
	Set(ref, secret string) error
	Delete(ref string) error
}

// Default returns the OS keyring when it is usable, otherwise an encrypted
// file store. Linux systems without a running Secret Service provider fall
// into the second case, which needs NEXUSMAIL_MASTER_PASSWORD to be set.
func Default(dir string) (SecretStore, error) {
	ks, err := NewKeyringStore()
	if err == nil {
		return ks, nil
	}
	master := os.Getenv("NEXUSMAIL_MASTER_PASSWORD")
	if master == "" {
		return nil, errors.New(
			"auth: no OS keyring available and NEXUSMAIL_MASTER_PASSWORD is unset; " +
				"set it to enable the encrypted file store")
	}
	return NewFileStore(dir, master)
}
```

- [ ] **Step 2: Şifreli dosya deposu için başarısız test yaz**

`internal/auth/file_store_test.go`:

```go
package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(dir, "correct horse battery staple")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	if err := s.Set("account:a@example.com", "refresh-token-value"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	got, err := s.Get("account:a@example.com")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "refresh-token-value" {
		t.Errorf("Get() = %q, want refresh-token-value", got)
	}

	if err := s.Delete("account:a@example.com"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, err := s.Get("account:a@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() after Delete returned %v, want ErrNotFound", err)
	}
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	s1, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s1.Set("k", "v"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	s2, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("second NewFileStore() error: %v", err)
	}
	got, err := s2.Get("k")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "v" {
		t.Errorf("Get() = %q, want v", got)
	}
}

func TestFileStoreRejectsWrongMasterPassword(t *testing.T) {
	dir := t.TempDir()
	s1, err := NewFileStore(dir, "right-password")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s1.Set("k", "v"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	s2, err := NewFileStore(dir, "wrong-password")
	if err != nil {
		// Failing at construction is acceptable.
		return
	}
	if _, err := s2.Get("k"); err == nil {
		t.Error("Get() with the wrong master password succeeded; AES-GCM authentication is not being checked")
	}
}

func TestFileStoreCiphertextDoesNotLeakPlaintext(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	const secret = "SUPER-SECRET-REFRESH-TOKEN"
	if err := s.Set("k", secret); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "secrets.enc"))
	if err != nil {
		t.Fatalf("read secrets file: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("secrets file contains the plaintext secret")
	}
}

func TestFileStoreRejectsOversizedSecret(t *testing.T) {
	s, err := NewFileStore(t.TempDir(), "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s.Set("k", strings.Repeat("x", maxSecretBytes+1)); !errors.Is(err, ErrSecretTooLong) {
		t.Errorf("Set() with an oversized secret returned %v, want ErrSecretTooLong", err)
	}
}
```

- [ ] **Step 3: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/auth/ -v
```

Beklenen: FAIL — `undefined: NewFileStore`

- [ ] **Step 4: Bağımlılığı ekle ve dosya deposunu yaz**

```bash
go get golang.org/x/crypto@latest
```

`internal/auth/file_store.go`:

```go
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	secretsFileName = "secrets.enc"
	saltLen         = 16
	keyLen          = 32
)

// fileStore keeps every secret in one AES-GCM sealed JSON blob. The key is
// derived from the master password with Argon2id; the salt is stored next to
// the ciphertext, which is safe and standard.
type fileStore struct {
	mu     sync.Mutex
	path   string
	master string
}

type secretsFile struct {
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// NewFileStore opens or creates the encrypted secrets file in dir. It verifies
// the master password immediately when the file already exists, so a wrong
// password fails here rather than on first Get.
func NewFileStore(dir, masterPassword string) (SecretStore, error) {
	if masterPassword == "" {
		return nil, errors.New("auth: master password must not be empty")
	}
	s := &fileStore{
		path:   filepath.Join(dir, secretsFileName),
		master: masterPassword,
	}
	if _, err := os.Stat(s.path); err == nil {
		if _, err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *fileStore) deriveKey(salt []byte) []byte {
	// Parameters follow the RFC 9106 second recommended option: 64 MiB, 3
	// passes, 4 lanes. Comfortable for an interactive unlock.
	return argon2.IDKey([]byte(s.master), salt, 3, 64*1024, 4, keyLen)
}

func (s *fileStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sf secretsFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("auth: secrets file is corrupt: %w", err)
	}
	block, err := aes.NewCipher(s.deriveKey(sf.Salt))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, sf.Nonce, sf.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: cannot decrypt secrets file (wrong master password?): %w", err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) save(entries map[string]string) error {
	plain, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	block, err := aes.NewCipher(s.deriveKey(salt))
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sf := secretsFile{
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: gcm.Seal(nil, nonce, plain, nil),
	}
	blob, err := json.Marshal(sf)
	if err != nil {
		return err
	}
	// Write to a temp file then rename, so a crash cannot leave a half-written
	// secrets file behind.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *fileStore) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := entries[ref]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *fileStore) Set(ref, secret string) error {
	if len(secret) > maxSecretBytes {
		return ErrSecretTooLong
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	entries[ref] = secret
	return s.save(entries)
}

func (s *fileStore) Delete(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	delete(entries, ref)
	return s.save(entries)
}
```

- [ ] **Step 5: Testlerin geçtiğini doğrula**

```bash
go test ./internal/auth/ -race -v
```

Beklenen: beş testin hepsi PASS.

- [ ] **Step 6: Anahtarlık deposunu yaz**

`internal/auth/keyring_store.go`:

```go
package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "nexus-mail"

// probeRef is written and deleted during construction to find out whether a
// usable keyring backend exists. Checking availability by trying is more
// reliable than inspecting the environment, which cannot tell whether a Secret
// Service provider is actually running.
const probeRef = "__nexusmail_probe__"

type keyringStore struct{}

// NewKeyringStore returns a store backed by the OS keyring, or an error when
// no keyring is reachable.
func NewKeyringStore() (SecretStore, error) {
	if err := keyring.Set(keyringService, probeRef, "probe"); err != nil {
		return nil, fmt.Errorf("auth: OS keyring unavailable: %w", err)
	}
	if err := keyring.Delete(keyringService, probeRef); err != nil {
		return nil, fmt.Errorf("auth: OS keyring is not writable: %w", err)
	}
	return &keyringStore{}, nil
}

func (k *keyringStore) Get(ref string) (string, error) {
	v, err := keyring.Get(keyringService, ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (k *keyringStore) Set(ref, secret string) error {
	if len(secret) > maxSecretBytes {
		return ErrSecretTooLong
	}
	return keyring.Set(keyringService, ref, secret)
}

func (k *keyringStore) Delete(ref string) error {
	err := keyring.Delete(keyringService, ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
```

```bash
go get github.com/zalando/go-keyring@latest
```

- [ ] **Step 7: `Default` seçim mantığı için test yaz**

`internal/auth/secretstore_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

// TestDefaultFallsBackToFileStore covers the Linux-without-Secret-Service case,
// which is exactly what CI runners look like. When a keyring IS available the
// test asserts the keyring is chosen instead.
func TestDefaultChoosesAnAvailableBackend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUSMAIL_MASTER_PASSWORD", "ci-master-password")

	s, err := Default(dir)
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if err := s.Set("probe", "value"); err != nil {
		t.Fatalf("Set() on the chosen backend failed: %v", err)
	}
	got, err := s.Get("probe")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "value" {
		t.Errorf("Get() = %q, want value", got)
	}
	t.Cleanup(func() { s.Delete("probe") })
}

func TestDefaultExplainsItselfWhenNothingIsAvailable(t *testing.T) {
	if _, err := NewKeyringStore(); err == nil {
		t.Skip("an OS keyring is available on this machine, so this path cannot be exercised")
	}
	t.Setenv("NEXUSMAIL_MASTER_PASSWORD", "")

	_, err := Default(t.TempDir())
	if err == nil {
		t.Fatal("Default() succeeded with no keyring and no master password")
	}
	if !strings.Contains(err.Error(), "NEXUSMAIL_MASTER_PASSWORD") {
		t.Errorf("error message %q does not tell the user how to fix it", err)
	}
}
```

- [ ] **Step 8: Testleri çalıştır**

```bash
go test ./internal/auth/ -race -v
```

Beklenen: PASS. Windows'ta ikinci test atlanır (anahtarlık var), Linux CI'da
çalışır. Bu asimetri kasıtlı — her iki yol da bir platformda gerçekten test edilir.

- [ ] **Step 9: Commit**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat(auth): add SecretStore with keyring and encrypted-file backends

Keyring availability is detected by probing rather than by inspecting
the environment, which cannot tell whether a Secret Service provider is
actually running. The encrypted file store covers Linux systems without
one; Argon2id parameters follow RFC 9106's second recommended option.

A test asserts the ciphertext does not contain the plaintext secret,
and another asserts a wrong master password is rejected rather than
silently returning garbage."
```

---

### Task 6: XOAUTH2 mekanizması ve kimlik sağlayıcı arayüzü

**Önemli bulgu:** `go-sasl` kütüphanesi XOAUTH2 **içermiyor** — PLAIN, LOGIN,
ANONYMOUS ve OAUTHBEARER (RFC 7628) var. Gmail ve Exchange Online ise IMAP'te
XOAUTH2 istiyor. Bu yüzden mekanizmayı kendimiz yazıyoruz; yaklaşık 30 satır.

**Files:**
- Create: `internal/auth/xoauth2.go`, `internal/auth/xoauth2_test.go`
- Create: `internal/auth/provider.go` (Task 1'deki iskeleti değiştirir)
- Create: `internal/auth/password_provider.go`, `internal/auth/password_provider_test.go`
- Create: `internal/auth/presets.go`, `internal/auth/presets_test.go`

**Interfaces:**
- Consumes: `auth.SecretStore` (Task 5), `model.Provider`, `model.AuthKind` (Task 4)
- Produces:
  - `auth.NewXOAUTH2Client(username, token string) sasl.Client`
  - `auth.CredentialProvider` arayüzü: `SASLClient(ctx) (sasl.Client, error)`, `Refresh(ctx) error`, `Kind() model.AuthKind`
  - `auth.NewPasswordProvider(username, secretRef string, store SecretStore) CredentialProvider`
  - `auth.PresetFor(email string) (Preset, bool)` ve `auth.Preset{IMAPHost, IMAPPort, SMTPHost, SMTPPort, Provider}`

- [ ] **Step 1: XOAUTH2 için başarısız test yaz**

Kodlama biçimi Microsoft'un dokümanındaki ile birebir aynı olmalı:
`user=<username>\x01auth=Bearer <token>\x01\x01`

`internal/auth/xoauth2_test.go`:

```go
package auth

import "testing"

func TestXOAUTH2InitialResponseFormat(t *testing.T) {
	c := NewXOAUTH2Client("test@contoso.onmicrosoft.com", "EwBAAl3BAAUFFpUAo7J3Ve0bjLBWZWCclRC3EoAA")

	mech, ir, err := c.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}
	want := "user=test@contoso.onmicrosoft.com\x01auth=Bearer EwBAAl3BAAUFFpUAo7J3Ve0bjLBWZWCclRC3EoAA\x01\x01"
	if string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}

// A server that rejects the token replies with a base64 JSON error challenge
// and expects an empty client response before it sends the tagged NO. We must
// surface the challenge as an error rather than hanging.
func TestXOAUTH2SurfacesServerChallengeAsError(t *testing.T) {
	c := NewXOAUTH2Client("user@example.com", "expired-token")
	if _, _, err := c.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	resp, err := c.Next([]byte(`{"status":"400","schemes":"Bearer","scope":"https://mail.google.com/"}`))
	if err == nil {
		t.Fatal("Next() returned no error for a server error challenge")
	}
	if len(resp) != 0 {
		t.Errorf("Next() response = %q, want empty", resp)
	}
}
```

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/auth/ -run TestXOAUTH2 -v
```

Beklenen: FAIL — `undefined: NewXOAUTH2Client`

- [ ] **Step 3: XOAUTH2 mekanizmasını yaz**

```bash
go get github.com/emersion/go-sasl@latest
```

`internal/auth/xoauth2.go`:

```go
package auth

import (
	"fmt"

	"github.com/emersion/go-sasl"
)

// xoauth2Client implements the XOAUTH2 SASL mechanism used by Gmail and
// Exchange Online for IMAP and SMTP. go-sasl ships PLAIN, LOGIN, ANONYMOUS and
// OAUTHBEARER, but not XOAUTH2, so it lives here.
//
// Wire format, per Microsoft's and Google's documentation:
//
//	user=<username>\x01auth=Bearer <token>\x01\x01
type xoauth2Client struct {
	username string
	token    string
}

// NewXOAUTH2Client returns a SASL client for the XOAUTH2 mechanism.
func NewXOAUTH2Client(username, token string) sasl.Client {
	return &xoauth2Client{username: username, token: token}
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	ir = []byte(fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", c.username, c.token))
	return "XOAUTH2", ir, nil
}

// Next is reached only when the server rejects the token: it sends a base64
// JSON error challenge and waits for an empty response before failing the
// command. Returning an error here gives the caller a usable message instead
// of an opaque protocol failure.
func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, fmt.Errorf("auth: XOAUTH2 rejected by server: %s", challenge)
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

```bash
go test ./internal/auth/ -run TestXOAUTH2 -v
```

Beklenen: PASS

- [ ] **Step 5: Sağlayıcı arayüzü ve parola sağlayıcısı için test yaz**

`internal/auth/password_provider_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"testing"

	"nexusmail/internal/model"
)

func TestPasswordProviderReturnsPlainClientWithStoredSecret(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("account:u@example.com", "app-password"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewPasswordProvider("u@example.com", "account:u@example.com", store)
	if p.Kind() != model.AuthPassword {
		t.Errorf("Kind() = %q, want %q", p.Kind(), model.AuthPassword)
	}

	client, err := p.SASLClient(context.Background())
	if err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}
	mech, ir, err := client.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "PLAIN" {
		t.Errorf("mechanism = %q, want PLAIN", mech)
	}
	want := "\x00u@example.com\x00app-password"
	if string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}

func TestPasswordProviderReportsMissingSecret(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	p := NewPasswordProvider("u@example.com", "account:missing", store)
	if _, err := p.SASLClient(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Errorf("SASLClient() error = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 6: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/auth/ -run TestPasswordProvider -v
```

Beklenen: FAIL — `undefined: NewPasswordProvider`

- [ ] **Step 7: Arayüzü ve parola sağlayıcısını yaz**

`internal/auth/provider.go` (Task 1'de oluşturulan tek satırlık iskeletin yerine):

```go
package auth

import (
	"context"

	"github.com/emersion/go-sasl"

	"nexusmail/internal/model"
)

// CredentialProvider hands the IMAP layer a ready SASL client. Every provider
// hides how the credential was obtained, so imapx never learns whether it is
// talking to Gmail, Exchange or a generic server.
type CredentialProvider interface {
	// SASLClient returns a client for a single authentication attempt.
	SASLClient(ctx context.Context) (sasl.Client, error)
	// Refresh renews the credential if the provider supports it. Password
	// providers return nil without doing anything.
	Refresh(ctx context.Context) error
	Kind() model.AuthKind
}
```

`internal/auth/password_provider.go`:

```go
package auth

import (
	"context"

	"github.com/emersion/go-sasl"

	"nexusmail/internal/model"
)

type passwordProvider struct {
	username  string
	secretRef string
	store     SecretStore
}

// NewPasswordProvider authenticates with SASL PLAIN using a password or app
// password read from the SecretStore. Callers must only use it over TLS.
func NewPasswordProvider(username, secretRef string, store SecretStore) CredentialProvider {
	return &passwordProvider{username: username, secretRef: secretRef, store: store}
}

func (p *passwordProvider) SASLClient(ctx context.Context) (sasl.Client, error) {
	secret, err := p.store.Get(p.secretRef)
	if err != nil {
		return nil, err
	}
	return sasl.NewPlainClient("", p.username, secret), nil
}

func (p *passwordProvider) Refresh(ctx context.Context) error { return nil }

func (p *passwordProvider) Kind() model.AuthKind { return model.AuthPassword }
```

- [ ] **Step 8: Sağlayıcı ön ayarları için test yaz**

`internal/auth/presets_test.go`:

```go
package auth

import (
	"testing"

	"nexusmail/internal/model"
)

func TestPresetForKnownDomains(t *testing.T) {
	cases := []struct {
		email        string
		wantHost     string
		wantPort     int
		wantProvider model.Provider
	}{
		{"a@gmail.com", "imap.gmail.com", 993, model.ProviderGoogle},
		{"a@googlemail.com", "imap.gmail.com", 993, model.ProviderGoogle},
		{"a@outlook.com", "outlook.office365.com", 993, model.ProviderMicrosoft},
		{"a@hotmail.com", "outlook.office365.com", 993, model.ProviderMicrosoft},
		{"a@yandex.com", "imap.yandex.com", 993, model.ProviderGeneric},
		{"A@GMAIL.COM", "imap.gmail.com", 993, model.ProviderGoogle},
	}
	for _, tc := range cases {
		got, ok := PresetFor(tc.email)
		if !ok {
			t.Errorf("PresetFor(%q) reported no preset", tc.email)
			continue
		}
		if got.IMAPHost != tc.wantHost {
			t.Errorf("PresetFor(%q).IMAPHost = %q, want %q", tc.email, got.IMAPHost, tc.wantHost)
		}
		if got.IMAPPort != tc.wantPort {
			t.Errorf("PresetFor(%q).IMAPPort = %d, want %d", tc.email, got.IMAPPort, tc.wantPort)
		}
		if got.Provider != tc.wantProvider {
			t.Errorf("PresetFor(%q).Provider = %q, want %q", tc.email, got.Provider, tc.wantProvider)
		}
	}
}

func TestPresetForUnknownDomain(t *testing.T) {
	if _, ok := PresetFor("someone@self-hosted.example"); ok {
		t.Error("PresetFor() claimed a preset for an unknown domain; the UI must ask for host and port instead")
	}
}

func TestPresetForMalformedAddress(t *testing.T) {
	for _, email := range []string{"", "no-at-sign", "@nodomain", "trailing@"} {
		if _, ok := PresetFor(email); ok {
			t.Errorf("PresetFor(%q) returned a preset for a malformed address", email)
		}
	}
}
```

- [ ] **Step 9: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/auth/ -run TestPreset -v
```

Beklenen: FAIL — `undefined: PresetFor`

- [ ] **Step 10: Ön ayarları yaz**

`internal/auth/presets.go`:

```go
package auth

import (
	"strings"

	"nexusmail/internal/model"
)

// Preset holds server settings for a well-known mail domain, so the account
// wizard can skip asking for hosts and ports.
type Preset struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
	Provider model.Provider
}

var presets = map[string]Preset{
	"gmail.com":       {"imap.gmail.com", 993, "smtp.gmail.com", 587, model.ProviderGoogle},
	"googlemail.com":  {"imap.gmail.com", 993, "smtp.gmail.com", 587, model.ProviderGoogle},
	"outlook.com":     {"outlook.office365.com", 993, "smtp.office365.com", 587, model.ProviderMicrosoft},
	"hotmail.com":     {"outlook.office365.com", 993, "smtp.office365.com", 587, model.ProviderMicrosoft},
	"live.com":        {"outlook.office365.com", 993, "smtp.office365.com", 587, model.ProviderMicrosoft},
	"office365.com":   {"outlook.office365.com", 993, "smtp.office365.com", 587, model.ProviderMicrosoft},
	"yandex.com":      {"imap.yandex.com", 993, "smtp.yandex.com", 465, model.ProviderGeneric},
	"yandex.com.tr":   {"imap.yandex.com.tr", 993, "smtp.yandex.com.tr", 465, model.ProviderGeneric},
	"zoho.com":        {"imap.zoho.com", 993, "smtp.zoho.com", 465, model.ProviderGeneric},
	"fastmail.com":    {"imap.fastmail.com", 993, "smtp.fastmail.com", 465, model.ProviderGeneric},
}

// PresetFor looks up settings by the address's domain. It reports false for
// unknown or malformed addresses; the caller must then ask the user for hosts
// and ports.
func PresetFor(email string) (Preset, bool) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return Preset{}, false
	}
	domain := strings.ToLower(email[at+1:])
	p, ok := presets[domain]
	return p, ok
}
```

- [ ] **Step 11: Tüm auth testlerini ve lint'i çalıştır**

```bash
go test ./internal/auth/ -race -v
golangci-lint run ./...
```

Beklenen: hepsi PASS, lint temiz.

- [ ] **Step 12: Commit**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat(auth): add XOAUTH2 mechanism, credential interface and presets

go-sasl ships PLAIN, LOGIN, ANONYMOUS and OAUTHBEARER but not XOAUTH2,
which is what Gmail and Exchange Online actually require for IMAP, so
the mechanism is implemented here against the wire format in Microsoft's
documentation.

CredentialProvider is the seam that keeps imapx unaware of which provider
it is talking to: three auth paths, one IMAP code path."
```

---

### Task 7: OAuth loopback akışı ve PKCE

**Files:**
- Create: `internal/auth/loopback.go`, `internal/auth/loopback_test.go`
- Create: `internal/auth/oauth_provider.go`, `internal/auth/oauth_provider_test.go`
- Create: `internal/auth/endpoints.go`

**Interfaces:**
- Consumes: `auth.SecretStore` (Task 5), `auth.CredentialProvider` (Task 6)
- Produces:
  - `auth.OAuthConfig{ClientID, Scopes, Endpoint, OpenBrowser func(string) error}`
  - `auth.RunLoopbackFlow(ctx, OAuthConfig) (*oauth2.Token, error)`
  - `auth.GoogleEndpoint()`, `auth.MicrosoftEndpoint()`, `auth.GoogleScopes()`, `auth.MicrosoftScopes()`
  - `auth.NewOAuthProvider(username, secretRef string, store SecretStore, cfg OAuthConfig) CredentialProvider`

- [ ] **Step 1: Loopback akışı için başarısız test yaz**

`internal/auth/loopback_test.go`:

```go
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeAuthServer stands in for Google or Microsoft. It records what the client
// sent so the test can assert on PKCE and redirect handling.
type fakeAuthServer struct {
	srv           *httptest.Server
	gotChallenge  string
	gotMethod     string
	gotRedirect   string
	gotVerifier   string
	gotClientID   string
	authorizeHits int
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	t.Helper()
	f := &fakeAuthServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.authorizeHits++
		f.gotChallenge = q.Get("code_challenge")
		f.gotMethod = q.Get("code_challenge_method")
		f.gotRedirect = q.Get("redirect_uri")
		f.gotClientID = q.Get("client_id")
		// Redirect back to the loopback listener the client opened, echoing
		// the state so the client can verify it.
		back := q.Get("redirect_uri") + "?code=test-auth-code&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, back, http.StatusFound)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.gotVerifier = r.Form.Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at-1","refresh_token":"rt-1","token_type":"Bearer","expires_in":3600}`))
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func TestRunLoopbackFlowUsesPKCEAndRandomPort(t *testing.T) {
	fake := newFakeAuthServer(t)

	var openedURL string
	cfg := OAuthConfig{
		ClientID: "client-abc",
		Scopes:   []string{"https://mail.google.com/"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  fake.srv.URL + "/authorize",
			TokenURL: fake.srv.URL + "/token",
		},
		// Instead of launching a real browser, fetch the URL ourselves. The
		// fake server's redirect lands on the loopback listener, completing
		// the flow exactly as a browser would.
		OpenBrowser: func(u string) error {
			openedURL = u
			go func() {
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Get(u)
				if err == nil {
					resp.Body.Close()
				}
			}()
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tok, err := RunLoopbackFlow(ctx, cfg)
	if err != nil {
		t.Fatalf("RunLoopbackFlow() error: %v", err)
	}
	if tok.RefreshToken != "rt-1" {
		t.Errorf("RefreshToken = %q, want rt-1", tok.RefreshToken)
	}
	if tok.AccessToken != "at-1" {
		t.Errorf("AccessToken = %q, want at-1", tok.AccessToken)
	}

	if fake.gotMethod != "S256" {
		t.Errorf("code_challenge_method = %q, want S256 (plain PKCE is not acceptable)", fake.gotMethod)
	}
	if fake.gotVerifier == "" {
		t.Fatal("token request carried no code_verifier; PKCE is not wired up")
	}
	// The challenge must be the S256 hash of the verifier that was later sent.
	sum := sha256.Sum256([]byte(fake.gotVerifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if fake.gotChallenge != wantChallenge {
		t.Errorf("code_challenge = %q, want %q (S256 of the verifier)", fake.gotChallenge, wantChallenge)
	}

	// The redirect must be loopback on an ephemeral port, never a fixed one.
	u, err := url.Parse(fake.gotRedirect)
	if err != nil {
		t.Fatalf("parse redirect_uri %q: %v", fake.gotRedirect, err)
	}
	if u.Hostname() != "127.0.0.1" {
		t.Errorf("redirect host = %q, want 127.0.0.1", u.Hostname())
	}
	if u.Port() == "" || u.Port() == "0" {
		t.Errorf("redirect port = %q, want a concrete ephemeral port", u.Port())
	}
	if u.Port() == "3000" {
		t.Error("redirect uses the fixed port 3000; a busy port would break sign-in")
	}
	if !strings.Contains(openedURL, "client-abc") {
		t.Errorf("browser URL %q does not carry the client ID", openedURL)
	}
}

func TestRunLoopbackFlowRejectsStateMismatch(t *testing.T) {
	f := &fakeAuthServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		// Deliberately return the wrong state, as a CSRF attempt would.
		back := r.URL.Query().Get("redirect_uri") + "?code=evil&state=not-the-state-we-sent"
		http.Redirect(w, r, back, http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		t.Error("token endpoint must not be reached after a state mismatch")
	})
	f.srv = httptest.NewServer(mux)
	defer f.srv.Close()

	cfg := OAuthConfig{
		ClientID: "c",
		Endpoint: oauth2.Endpoint{AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token"},
		OpenBrowser: func(u string) error {
			go func() {
				resp, err := http.Get(u)
				if err == nil {
					resp.Body.Close()
				}
			}()
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := RunLoopbackFlow(ctx, cfg); err == nil {
		t.Fatal("RunLoopbackFlow() accepted a mismatched state")
	}
}
```

- [ ] **Step 2: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/auth/ -run TestRunLoopbackFlow -v
```

Beklenen: FAIL — `undefined: OAuthConfig`

- [ ] **Step 3: Loopback akışını yaz**

```bash
go get golang.org/x/oauth2@latest
```

`internal/auth/loopback.go`:

```go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
)

// OAuthConfig describes one provider's authorization-code flow.
type OAuthConfig struct {
	ClientID string
	// ClientSecret is empty for public clients, which is what a desktop app is.
	ClientSecret string
	Scopes       []string
	Endpoint     oauth2.Endpoint
	// OpenBrowser launches the system browser. Tests substitute their own.
	OpenBrowser func(url string) error
}

// RunLoopbackFlow performs RFC 8252 authorization on a loopback redirect with
// PKCE. It listens on an ephemeral port rather than a fixed one, because a
// fixed port that is already in use would break sign-in with no way for the
// user to recover.
func RunLoopbackFlow(ctx context.Context, cfg OAuthConfig) (*oauth2.Token, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("auth: OAuth client ID is required")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("auth: cannot open loopback listener: %w", err)
	}
	defer ln.Close()

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	conf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       cfg.Scopes,
		Endpoint:     cfg.Endpoint,
		RedirectURL:  redirectURI,
	}

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if gotErr := q.Get("error"); gotErr != "" {
				writeBrowserPage(w, "Sign-in failed. You can close this window.")
				results <- result{err: fmt.Errorf("auth: provider returned error %q: %s", gotErr, q.Get("error_description"))}
				return
			}
			if q.Get("state") != state {
				writeBrowserPage(w, "Sign-in failed. You can close this window.")
				results <- result{err: errors.New("auth: state mismatch; discarding the response")}
				return
			}
			code := q.Get("code")
			if code == "" {
				writeBrowserPage(w, "Sign-in failed. You can close this window.")
				results <- result{err: errors.New("auth: provider returned no authorization code")}
				return
			}
			writeBrowserPage(w, "Signed in. You can close this window and return to Nexus Mail.")
			results <- result{code: code}
		}),
	}
	go srv.Serve(ln)
	defer srv.Close()

	authURL := conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)

	open := cfg.OpenBrowser
	if open == nil {
		open = openSystemBrowser
	}
	if err := open(authURL); err != nil {
		return nil, fmt.Errorf("auth: cannot open browser: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-results:
		if res.err != nil {
			return nil, res.err
		}
		tok, err := conf.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, fmt.Errorf("auth: token exchange failed: %w", err)
		}
		return tok, nil
	}
}

func writeBrowserPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Nexus Mail</title><p>%s</p>", msg)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openSystemBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
```

- [ ] **Step 4: Testlerin geçtiğini doğrula**

```bash
go test ./internal/auth/ -race -run TestRunLoopbackFlow -v
```

Beklenen: iki test de PASS.

- [ ] **Step 5: Sağlayıcı uç noktalarını yaz**

`internal/auth/endpoints.go`:

```go
package auth

import "golang.org/x/oauth2"

// GoogleEndpoint returns Google's OAuth 2.0 endpoints.
func GoogleEndpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	}
}

// MicrosoftEndpoint returns the Microsoft identity platform endpoints for the
// "common" authority, which serves both work/school and personal accounts.
func MicrosoftEndpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	}
}

// GoogleScopes returns the scopes needed for full IMAP access plus the address
// of the signed-in user. https://mail.google.com/ is a restricted scope: users
// supply their own client ID, which is why this app needs no CASA assessment.
func GoogleScopes() []string {
	return []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/userinfo.email",
	}
}

// MicrosoftScopes returns the scopes for IMAP and SMTP against Exchange
// Online, per Microsoft's documented protocol scope strings. offline_access is
// what makes refresh tokens available.
func MicrosoftScopes() []string {
	return []string{
		"https://outlook.office.com/IMAP.AccessAsUser.All",
		"https://outlook.office.com/SMTP.Send",
		"offline_access",
	}
}
```

- [ ] **Step 6: OAuth sağlayıcısı için başarısız test yaz**

`internal/auth/oauth_provider_test.go`:

```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"nexusmail/internal/model"
)

func TestOAuthProviderExchangesStoredRefreshTokenForXOAUTH2Client(t *testing.T) {
	var gotGrant, gotRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"fresh-at","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	store, err := NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("account:u@example.com", "stored-rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "account:u@example.com", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	})
	if p.Kind() != model.AuthOAuth {
		t.Errorf("Kind() = %q, want %q", p.Kind(), model.AuthOAuth)
	}

	client, err := p.SASLClient(context.Background())
	if err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}
	if gotGrant != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotGrant)
	}
	if gotRefresh != "stored-rt" {
		t.Errorf("refresh_token = %q, want stored-rt", gotRefresh)
	}

	mech, ir, err := client.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}
	if !strings.Contains(string(ir), "auth=Bearer fresh-at") {
		t.Errorf("initial response %q does not carry the freshly minted access token", ir)
	}
	if !strings.Contains(string(ir), "user=u@example.com") {
		t.Errorf("initial response %q does not carry the username", ir)
	}
}

// Providers rotate refresh tokens. If we drop a rotated token the account
// silently dies the next time the old one expires, so persistence is asserted.
func TestOAuthProviderPersistsRotatedRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at","refresh_token":"rotated-rt","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	store, err := NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("ref", "original-rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "ref", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	})
	if _, err := p.SASLClient(context.Background()); err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}

	got, err := store.Get("ref")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "rotated-rt" {
		t.Errorf("stored refresh token = %q, want rotated-rt", got)
	}
}
```

- [ ] **Step 7: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/auth/ -run TestOAuthProvider -v
```

Beklenen: FAIL — `undefined: NewOAuthProvider`

- [ ] **Step 8: OAuth sağlayıcısını yaz**

`internal/auth/oauth_provider.go`:

```go
package auth

import (
	"context"
	"fmt"
	"sync"

	"github.com/emersion/go-sasl"
	"golang.org/x/oauth2"

	"nexusmail/internal/model"
)

type oauthProvider struct {
	mu        sync.Mutex
	username  string
	secretRef string
	store     SecretStore
	conf      *oauth2.Config
	// cached holds the last access token so repeated connections within its
	// lifetime do not hit the token endpoint again.
	cached *oauth2.Token
}

// NewOAuthProvider authenticates with XOAUTH2, minting access tokens from the
// refresh token held in the SecretStore.
func NewOAuthProvider(username, secretRef string, store SecretStore, cfg OAuthConfig) CredentialProvider {
	return &oauthProvider{
		username:  username,
		secretRef: secretRef,
		store:     store,
		conf: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Scopes:       cfg.Scopes,
			Endpoint:     cfg.Endpoint,
		},
	}
}

func (p *oauthProvider) SASLClient(ctx context.Context) (sasl.Client, error) {
	tok, err := p.token(ctx)
	if err != nil {
		return nil, err
	}
	return NewXOAUTH2Client(p.username, tok.AccessToken), nil
}

func (p *oauthProvider) Refresh(ctx context.Context) error {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
	_, err := p.token(ctx)
	return err
}

func (p *oauthProvider) Kind() model.AuthKind { return model.AuthOAuth }

func (p *oauthProvider) token(ctx context.Context) (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached != nil && p.cached.Valid() {
		return p.cached, nil
	}

	refresh, err := p.store.Get(p.secretRef)
	if err != nil {
		return nil, fmt.Errorf("auth: cannot read refresh token: %w", err)
	}

	src := p.conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	tok, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: refreshing access token failed: %w", err)
	}

	// Providers may rotate the refresh token. Dropping a rotated value would
	// kill the account once the old one expires, so persist any change.
	if tok.RefreshToken != "" && tok.RefreshToken != refresh {
		if err := p.store.Set(p.secretRef, tok.RefreshToken); err != nil {
			return nil, fmt.Errorf("auth: cannot persist rotated refresh token: %w", err)
		}
	}

	p.cached = tok
	return tok, nil
}
```

- [ ] **Step 9: Tüm auth testlerini ve lint'i çalıştır**

```bash
go test ./internal/auth/ -race -v
golangci-lint run ./...
```

Beklenen: hepsi PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat(auth): implement RFC 8252 loopback flow with PKCE

The listener binds 127.0.0.1:0 rather than a fixed port: a fixed port
already in use would break sign-in with no recovery path for the user.
Tests assert the challenge really is S256 of the verifier that is later
sent, and that a mismatched state never reaches the token endpoint.

Rotated refresh tokens are written back to the SecretStore. Dropping one
would kill the account silently once the previous token expired."
```

---

### Task 8: MailBackend arayüzü, bağlantı ve klasör listeleme

Bu görev projenin test edilebilirliğinin dayanağını kurar: `sync` motoru asla
go-imap'i doğrudan görmez, `MailBackend` arayüzünü görür. Test tarafında ise
go-imap v2'nin kendi `imapmemserver` paketiyle bellek içi gerçek bir IMAP sunucusu
çalıştırırız — mock değil, protokolü konuşan gerçek bir sunucu.

**Files:**
- Create: `internal/imapx/backend.go`, `internal/imapx/client.go`
- Create: `internal/imapx/testserver_test.go`, `internal/imapx/client_test.go`

**Interfaces:**
- Consumes: `auth.CredentialProvider` (Task 6), `model.Folder`, `model.Message` (Task 4)
- Produces:
  - `imapx.MailBackend` arayüzü (aşağıda tam tanımı)
  - `imapx.Config{Host string, Port int, TLS bool, Username string}`
  - `imapx.Dial(ctx, Config, auth.CredentialProvider) (MailBackend, error)`
  - `imapx.SelectResult{UIDValidity uint32, UIDNext uint32, NumMessages uint32, HighestModSeq uint64}`
  - `imapx.Capabilities{CondStore bool, QResync bool, Move bool, Idle bool}`
  - `imapx.UIDRange{Start, End uint32}`
  - `imapx.Body{HTML, Text string}`

- [ ] **Step 1: Sabitlenen sürümdeki API imzalarını doğrula**

`imapmemserver` beta bir paketin alt paketi; imzaları belgeye güvenmek yerine
doğrudan sor:

```bash
go get github.com/emersion/go-imap/v2@v2.0.0-beta.7
go doc github.com/emersion/go-imap/v2/imapserver/imapmemserver
go doc github.com/emersion/go-imap/v2/imapserver Options
go doc github.com/emersion/go-imap/v2/imapclient Client.Fetch
```

Aşağıdaki kodda kullanılan çağrılar bu çıktıyla uyuşmuyorsa (beta sürümlerde
mümkün), imzaları çıktıya göre düzelt — arayüz sözleşmesi (`MailBackend`) aynı
kalır, yalnızca `client.go` içindeki go-imap çağrıları değişir. Sabitlenen sürüm
`v2.0.0-beta.7`; `go.mod`'a bu sürümü yaz ve `go get -u` çalıştırma.

- [ ] **Step 2: Arayüzü yaz**

`internal/imapx/backend.go`:

```go
// Package imapx wraps go-imap so the rest of the app talks to mail servers
// through one narrow interface. The sync engine depends on MailBackend, never
// on go-imap directly, which is what lets it be tested against an in-memory
// server instead of a live account.
package imapx

import (
	"context"

	"nexusmail/internal/model"
)

// Config describes how to reach one server.
type Config struct {
	Host string
	Port int
	// TLS selects implicit TLS on connect (port 993). Tests set it false to
	// talk to a plaintext loopback server.
	TLS bool
	// Username is the address sent in the SASL exchange.
	Username string
}

// Capabilities records what the connected server actually supports, so callers
// can pick a strategy instead of assuming.
type Capabilities struct {
	CondStore bool
	QResync   bool
	Move      bool
	Idle      bool
}

// SelectResult carries the mailbox state a SELECT reports. HighestModSeq is
// zero when the server does not advertise CONDSTORE.
type SelectResult struct {
	UIDValidity   uint32
	UIDNext       uint32
	NumMessages   uint32
	HighestModSeq uint64
}

// UIDRange is an inclusive UID range. End of 0 means "through the highest UID".
type UIDRange struct {
	Start uint32
	End   uint32
}

// Body holds the two renderable representations of a message.
type Body struct {
	HTML string
	Text string
}

// MailBackend is one authenticated connection to one account.
//
// Implementations are NOT safe for concurrent use: IMAP is a stateful,
// sequential protocol and a selected mailbox is per-connection. The sync engine
// owns one backend per connection and never shares it across goroutines.
type MailBackend interface {
	// Capabilities reports what the server advertised after authentication.
	Capabilities() Capabilities

	// ListFolders returns every mailbox with its status counts filled in.
	ListFolders(ctx context.Context) ([]model.Folder, error)

	// Select opens a mailbox for subsequent fetches.
	Select(ctx context.Context, path string) (SelectResult, error)

	// FetchHeaders returns message headers for a UID range in the selected
	// mailbox. Bodies are not fetched.
	FetchHeaders(ctx context.Context, r UIDRange) ([]model.Message, error)

	// FetchBody returns the HTML and plain-text parts of one message.
	FetchBody(ctx context.Context, uid uint32) (Body, error)

	Close() error
}
```

- [ ] **Step 3: Sahte sunucu test koşumunu yaz**

`internal/imapx/testserver_test.go`:

```go
package imapx

import (
	"net"
	"testing"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

const (
	testUser = "user@example.com"
	testPass = "password"
)

// startFakeServer runs a real in-memory IMAP server on loopback and returns its
// address plus the user object, so tests can append messages and change
// mailbox state from the server side.
func startFakeServer(t *testing.T) (addr string, user *imapmemserver.User) {
	t.Helper()

	mem := imapmemserver.New()
	user = imapmemserver.NewUser(testUser, testPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		// The fake server speaks plaintext on loopback; production always uses TLS.
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() {
		srv.Close()
	})
	return ln.Addr().String(), user
}

// splitHostPort is a small helper so tests can build a Config from the
// listener address.
func configFor(t *testing.T, addr string) Config {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return Config{Host: host, Port: port, TLS: false, Username: testUser}
}
```

- [ ] **Step 4: Bağlantı ve klasör listeleme için başarısız test yaz**

`internal/imapx/client_test.go`:

```go
package imapx

import (
	"context"
	"testing"

	"nexusmail/internal/auth"
)

func testProvider(t *testing.T) auth.CredentialProvider {
	t.Helper()
	store, err := auth.NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("ref", testPass); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	return auth.NewPasswordProvider(testUser, "ref", store)
}

func TestDialAuthenticatesAndReportsCapabilities(t *testing.T) {
	addr, _ := startFakeServer(t)
	ctx := context.Background()

	be, err := Dial(ctx, configFor(t, addr), testProvider(t))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	defer be.Close()

	caps := be.Capabilities()
	// imapmemserver implements IMAP4rev2, so MOVE and IDLE are expected. The
	// point of this assertion is that Capabilities() reflects the server rather
	// than returning a zero value.
	if !caps.Move && !caps.Idle && !caps.CondStore {
		t.Error("Capabilities() reported nothing at all; capability detection is not wired up")
	}
}

func TestDialRejectsWrongPassword(t *testing.T) {
	addr, _ := startFakeServer(t)
	store, err := auth.NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("ref", "not-the-password"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	provider := auth.NewPasswordProvider(testUser, "ref", store)

	if _, err := Dial(context.Background(), configFor(t, addr), provider); err == nil {
		t.Fatal("Dial() succeeded with the wrong password")
	}
}

func TestListFoldersReturnsMailboxesWithCounts(t *testing.T) {
	addr, user := startFakeServer(t)
	if err := user.Create("Archive", nil); err != nil {
		t.Fatalf("create Archive: %v", err)
	}

	ctx := context.Background()
	be, err := Dial(ctx, configFor(t, addr), testProvider(t))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	defer be.Close()

	folders, err := be.ListFolders(ctx)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) < 2 {
		t.Fatalf("ListFolders() returned %d folders, want at least INBOX and Archive", len(folders))
	}

	byPath := map[string]bool{}
	for _, f := range folders {
		byPath[f.Path] = true
		if f.Name == "" {
			t.Errorf("folder %q has an empty Name", f.Path)
		}
		if f.Delimiter == "" {
			t.Errorf("folder %q has an empty Delimiter", f.Path)
		}
	}
	if !byPath["INBOX"] {
		t.Error("ListFolders() did not return INBOX")
	}
	if !byPath["Archive"] {
		t.Error("ListFolders() did not return Archive")
	}
}

func TestSelectReportsUIDValidity(t *testing.T) {
	addr, _ := startFakeServer(t)
	ctx := context.Background()

	be, err := Dial(ctx, configFor(t, addr), testProvider(t))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	defer be.Close()

	res, err := be.Select(ctx, "INBOX")
	if err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	if res.UIDValidity == 0 {
		t.Error("Select() reported UIDValidity 0; the sync engine relies on this value to detect resets")
	}
	if res.UIDNext == 0 {
		t.Error("Select() reported UIDNext 0")
	}
}
```

- [ ] **Step 5: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/imapx/ -v
```

Beklenen: FAIL — `undefined: Dial`

- [ ] **Step 6: İstemciyi yaz**

`internal/imapx/client.go`:

```go
package imapx

import (
	"context"
	"fmt"
	"strconv"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"nexusmail/internal/auth"
	"nexusmail/internal/model"
)

type client struct {
	c    *imapclient.Client
	caps Capabilities
}

// Dial connects, authenticates with the provider's SASL client and records the
// server's capabilities.
func Dial(ctx context.Context, cfg Config, provider auth.CredentialProvider) (MailBackend, error) {
	addr := cfg.Host + ":" + strconv.Itoa(cfg.Port)

	var (
		c   *imapclient.Client
		err error
	)
	if cfg.TLS {
		c, err = imapclient.DialTLS(addr, nil)
	} else {
		c, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("imapx: dial %s: %w", addr, err)
	}

	saslClient, err := provider.SASLClient(ctx)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("imapx: obtaining credentials: %w", err)
	}
	if err := c.Authenticate(saslClient); err != nil {
		c.Close()
		return nil, fmt.Errorf("imapx: authentication failed for %s: %w", cfg.Username, err)
	}

	caps, err := c.Capability().Wait()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("imapx: reading capabilities: %w", err)
	}

	return &client{
		c: c,
		caps: Capabilities{
			CondStore: caps.Has(imap.CapCondStore),
			QResync:   caps.Has(imap.CapQResync),
			Move:      caps.Has(imap.CapMove),
			Idle:      caps.Has(imap.CapIdle),
		},
	}, nil
}

func (cl *client) Capabilities() Capabilities { return cl.caps }

func (cl *client) Close() error { return cl.c.Close() }

// ListFolders issues LIST with STATUS return data, so counts and UIDVALIDITY
// arrive in the same round trip instead of one STATUS per mailbox.
func (cl *client) ListFolders(ctx context.Context) ([]model.Folder, error) {
	opts := &imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
			UIDNext:     true,
			UIDValidity: true,
		},
	}
	mailboxes, err := cl.c.List("", "*", opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: LIST failed: %w", err)
	}

	out := make([]model.Folder, 0, len(mailboxes))
	for _, mb := range mailboxes {
		f := model.Folder{
			Path:      mb.Mailbox,
			Name:      leafName(mb.Mailbox, string(mb.Delim)),
			Delimiter: string(mb.Delim),
		}
		for _, attr := range mb.Attrs {
			f.Attributes = append(f.Attributes, string(attr))
		}
		if st := mb.Status; st != nil {
			if st.NumMessages != nil {
				f.TotalCount = int(*st.NumMessages)
			}
			if st.NumUnseen != nil {
				f.UnreadCount = int(*st.NumUnseen)
			}
			if st.UIDNext != 0 {
				f.UIDNext = uint32(st.UIDNext)
			}
			if st.UIDValidity != 0 {
				f.UIDValidity = st.UIDValidity
			}
		}
		out = append(out, f)
	}
	return out, nil
}

func (cl *client) Select(ctx context.Context, path string) (SelectResult, error) {
	data, err := cl.c.Select(path, nil).Wait()
	if err != nil {
		return SelectResult{}, fmt.Errorf("imapx: SELECT %q failed: %w", path, err)
	}
	res := SelectResult{
		UIDValidity: data.UIDValidity,
		UIDNext:     uint32(data.UIDNext),
		NumMessages: data.NumMessages,
	}
	if data.HighestModSeq != 0 {
		res.HighestModSeq = data.HighestModSeq
	}
	return res, nil
}

// leafName returns the display name of a mailbox path: "Parent/Child" becomes
// "Child". A mailbox with no delimiter is its own leaf.
func leafName(path, delim string) string {
	if delim == "" {
		return path
	}
	for i := len(path) - len(delim); i >= 0; i-- {
		if path[i:i+len(delim)] == delim {
			return path[i+len(delim):]
		}
	}
	return path
}
```

- [ ] **Step 7: Testlerin geçtiğini doğrula**

```bash
go test ./internal/imapx/ -race -v
```

Beklenen: dört test PASS. `FetchHeaders` ve `FetchBody` henüz yazılmadığı için
derleme hatası verirse, geçici olarak `panic("not implemented in this task")`
gövdesiyle ekle — Task 9 bunları dolduruyor.

- [ ] **Step 8: `sync`'in go-imap görmediğini doğrula**

```bash
golangci-lint run ./internal/sync/...
go list -deps ./internal/imapx | grep go-imap
```

Beklenen: lint temiz; `go list` çıktısı go-imap'i yalnızca `imapx` altında
gösterir. Bu, arayüz dikişinin gerçekten tuttuğunun kanıtı.

- [ ] **Step 9: Commit**

```bash
git add internal/imapx go.mod go.sum
git commit -m "feat(imapx): add MailBackend interface, dialing and folder listing

Tests run against go-imap's own in-memory server rather than a mock, so
they exercise the real protocol: a wrong password really fails auth, and
UIDVALIDITY really comes from a SELECT response.

LIST carries STATUS return data so counts and UIDVALIDITY arrive in one
round trip instead of one STATUS command per mailbox — noticeable on
corporate accounts with dozens of folders."
```

---

### Task 9: Başlık ve gövde çekme, envelope dönüşümü

**Files:**
- Create: `internal/imapx/envelope.go`, `internal/imapx/envelope_test.go`
- Modify: `internal/imapx/client.go` (`FetchHeaders`, `FetchBody`)
- Modify: `internal/imapx/client_test.go` (yeni testler eklenir)

**Interfaces:**
- Consumes: `imapx.MailBackend`, `imapx.UIDRange`, `imapx.Body` (Task 8)
- Produces:
  - `(*client).FetchHeaders(ctx, UIDRange) ([]model.Message, error)` — `MailBackend`'i tamamlar
  - `(*client).FetchBody(ctx, uid uint32) (Body, error)`
  - `imapx.ThreadKey(messageID, inReplyTo string, references []string, subject string) string`

- [ ] **Step 1: Konu zinciri anahtarı için başarısız test yaz**

Konuşma gruplama M1'de tek bir saf fonksiyona indirgeniyor, çünkü test edilebilir
tek nokta burası.

`internal/imapx/envelope_test.go`:

```go
package imapx

import "testing"

func TestThreadKeyPrefersReferencesRoot(t *testing.T) {
	// A reply carries the whole ancestry; the first entry is the thread root.
	got := ThreadKey("<c@x>", "<b@x>", []string{"<a@x>", "<b@x>"}, "Re: Invoice")
	if got != "<a@x>" {
		t.Errorf("ThreadKey() = %q, want the References root <a@x>", got)
	}
}

func TestThreadKeyFallsBackToInReplyTo(t *testing.T) {
	// Some clients send In-Reply-To without References.
	got := ThreadKey("<c@x>", "<b@x>", nil, "Re: Invoice")
	if got != "<b@x>" {
		t.Errorf("ThreadKey() = %q, want <b@x>", got)
	}
}

func TestThreadKeyUsesOwnMessageIDForThreadStart(t *testing.T) {
	got := ThreadKey("<a@x>", "", nil, "Invoice")
	if got != "<a@x>" {
		t.Errorf("ThreadKey() = %q, want its own Message-ID <a@x>", got)
	}
}

func TestThreadKeyFallsBackToNormalisedSubject(t *testing.T) {
	// Mailing lists and some corporate gateways strip Message-ID. The subject
	// is the last resort, normalised so replies land in the same thread.
	cases := []string{
		"Re: Invoice 2026",
		"RE: Invoice 2026",
		"Fwd: Invoice 2026",
		"FW: Invoice 2026",
		"Re: Re: Invoice 2026",
		"YNT: Invoice 2026",
		"  Invoice 2026  ",
	}
	want := ThreadKey("", "", nil, "Invoice 2026")
	for _, subject := range cases {
		if got := ThreadKey("", "", nil, subject); got != want {
			t.Errorf("ThreadKey(subject=%q) = %q, want %q", subject, got, want)
		}
	}
}

func TestThreadKeyDistinguishesDifferentSubjects(t *testing.T) {
	a := ThreadKey("", "", nil, "Invoice 2026")
	b := ThreadKey("", "", nil, "Invoice 2027")
	if a == b {
		t.Error("different subjects produced the same thread key")
	}
}
```

`YNT:` Türkçe Outlook'un "yanıt" önekidir; kurumsal Türkiye kullanımında sık
görülür ve normalize edilmezse aynı konuşma ikiye bölünür.

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/imapx/ -run TestThreadKey -v
```

Beklenen: FAIL — `undefined: ThreadKey`

- [ ] **Step 3: Envelope dönüşümünü ve konu anahtarını yaz**

`internal/imapx/envelope.go`:

```go
package imapx

import (
	"strings"

	"github.com/emersion/go-imap/v2"

	"nexusmail/internal/model"
)

// replyPrefixes are stripped when deriving a thread key from a subject.
// Turkish Outlook uses YNT (yanıt) and İLT (ilet); without them a corporate
// Turkish thread splits in two.
var replyPrefixes = []string{"re:", "fwd:", "fw:", "ynt:", "ilt:", "İlt:", "aw:", "sv:"}

// ThreadKey derives a stable conversation identifier. Preference order follows
// the simplified JWZ approach: the References root, then In-Reply-To, then the
// message's own ID, and finally a normalised subject for servers that strip
// Message-ID entirely.
func ThreadKey(messageID, inReplyTo string, references []string, subject string) string {
	if len(references) > 0 && references[0] != "" {
		return references[0]
	}
	if inReplyTo != "" {
		return inReplyTo
	}
	if messageID != "" {
		return messageID
	}
	return "subject:" + normaliseSubject(subject)
}

func normaliseSubject(s string) string {
	out := strings.TrimSpace(s)
	for changed := true; changed; {
		changed = false
		lower := strings.ToLower(out)
		for _, p := range replyPrefixes {
			if strings.HasPrefix(lower, strings.ToLower(p)) {
				out = strings.TrimSpace(out[len(p):])
				changed = true
				break
			}
		}
	}
	return strings.ToLower(strings.Join(strings.Fields(out), " "))
}

func addressesFrom(list []imap.Address) []model.Address {
	out := make([]model.Address, 0, len(list))
	for _, a := range list {
		out = append(out, model.Address{
			Name: a.Name,
			Addr: a.Addr(),
		})
	}
	return out
}

func firstAddress(list []imap.Address) model.Address {
	if len(list) == 0 {
		return model.Address{}
	}
	return addressesFrom(list[:1])[0]
}

// hasAttachmentParts walks a BODYSTRUCTURE looking for a part with a
// Content-Disposition of attachment, or a non-text leaf part. Inline images in
// HTML mail count, which matches what users expect from the paperclip icon.
func hasAttachmentParts(bs imap.BodyStructure) bool {
	found := false
	bs.Walk(func(path []int, part imap.BodyStructure) bool {
		if found {
			return false
		}
		switch p := part.(type) {
		case *imap.BodyStructureSinglePart:
			if p.Disposition() != nil &&
				strings.EqualFold(p.Disposition().Value, "attachment") {
				found = true
				return false
			}
			if !strings.EqualFold(p.Type, "text") && !strings.EqualFold(p.Type, "multipart") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

```bash
go test ./internal/imapx/ -run TestThreadKey -v
```

Beklenen: beş test PASS.

- [ ] **Step 5: Başlık ve gövde çekme için başarısız test yaz**

`internal/imapx/client_test.go` dosyasının sonuna ekle:

```go
func appendTestMessage(t *testing.T, user *imapmemserver.User, subject, from, body string) {
	t.Helper()
	raw := "From: " + from + "\r\n" +
		"To: " + testUser + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Message-ID: <" + subject + "@example.com>\r\n" +
		"Date: Mon, 02 Jan 2026 15:04:05 +0000\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" + body + "\r\n"

	mbox, err := user.Mailbox("INBOX")
	if err != nil {
		t.Fatalf("open INBOX: %v", err)
	}
	if err := mbox.Append(strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
		t.Fatalf("append message: %v", err)
	}
}

func TestFetchHeadersMapsEnvelopeFields(t *testing.T) {
	addr, user := startFakeServer(t)
	appendTestMessage(t, user, "Invoice", "Ali <ali@example.com>", "<p>Hello</p>")

	ctx := context.Background()
	be, err := Dial(ctx, configFor(t, addr), testProvider(t))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	defer be.Close()

	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1, End: 0})
	if err != nil {
		t.Fatalf("FetchHeaders() error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("FetchHeaders() returned %d messages, want 1", len(msgs))
	}

	m := msgs[0]
	if m.Subject != "Invoice" {
		t.Errorf("Subject = %q, want Invoice", m.Subject)
	}
	if m.From.Addr != "ali@example.com" {
		t.Errorf("From.Addr = %q, want ali@example.com", m.From.Addr)
	}
	if m.From.Name != "Ali" {
		t.Errorf("From.Name = %q, want Ali", m.From.Name)
	}
	if m.UID == 0 {
		t.Error("UID = 0; the sync engine keys everything on UID")
	}
	if m.ThreadID == "" {
		t.Error("ThreadID is empty")
	}
	if m.InternalDate.IsZero() {
		t.Error("InternalDate is zero; the message list sorts on it")
	}
	if m.Size == 0 {
		t.Error("Size = 0")
	}
	// Bodies must NOT be fetched by FetchHeaders — that is the whole point of
	// splitting the two calls.
	if m.BodyFetched {
		t.Error("BodyFetched = true after FetchHeaders; headers must not pull bodies")
	}
}

func TestFetchBodyReturnsHTML(t *testing.T) {
	addr, user := startFakeServer(t)
	appendTestMessage(t, user, "Body", "b@example.com", "<p>Rendered</p>")

	ctx := context.Background()
	be, err := Dial(ctx, configFor(t, addr), testProvider(t))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	defer be.Close()

	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1, End: 0})
	if err != nil {
		t.Fatalf("FetchHeaders() error: %v", err)
	}

	body, err := be.FetchBody(ctx, msgs[0].UID)
	if err != nil {
		t.Fatalf("FetchBody() error: %v", err)
	}
	if !strings.Contains(body.HTML, "Rendered") {
		t.Errorf("Body.HTML = %q, want it to contain Rendered", body.HTML)
	}
}
```

`client_test.go` dosyasının import bloğuna `strings`, `github.com/emersion/go-imap/v2`
ve `github.com/emersion/go-imap/v2/imapserver/imapmemserver` ekle.

- [ ] **Step 6: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/imapx/ -run 'TestFetchHeaders|TestFetchBody' -v
```

Beklenen: FAIL — `FetchHeaders` panikler veya tanımsız.

- [ ] **Step 7: `FetchHeaders` ve `FetchBody`'yi yaz**

`internal/imapx/client.go` sonuna ekle:

```go
// FetchHeaders pulls envelopes, flags, sizes and body structures for a UID
// range. It deliberately does not fetch bodies: on a 1000-message initial sync
// that difference is tens of megabytes.
func (cl *client) FetchHeaders(ctx context.Context, r UIDRange) ([]model.Message, error) {
	set := imap.UIDSetNum()
	if r.End == 0 {
		set = imap.UIDSet{imap.UIDRange{Start: imap.UID(r.Start), Stop: 0}}
	} else {
		set = imap.UIDSet{imap.UIDRange{Start: imap.UID(r.Start), Stop: imap.UID(r.End)}}
	}

	opts := &imap.FetchOptions{
		UID:           true,
		Flags:         true,
		Envelope:      true,
		InternalDate:  true,
		RFC822Size:    true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}

	buffers, err := cl.c.Fetch(set, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: FETCH failed: %w", err)
	}

	out := make([]model.Message, 0, len(buffers))
	for _, buf := range buffers {
		m := model.Message{
			UID:          uint32(buf.UID),
			InternalDate: buf.InternalDate,
			Size:         buf.RFC822Size,
		}
		for _, f := range buf.Flags {
			m.Flags = append(m.Flags, string(f))
		}
		if env := buf.Envelope; env != nil {
			m.Subject = env.Subject
			m.MessageID = env.MessageID
			m.InReplyTo = strings.Join(env.InReplyTo, " ")
			m.Date = env.Date
			m.From = firstAddress(env.From)
			m.To = addressesFrom(env.To)
			m.Cc = addressesFrom(env.Cc)
		}
		if buf.BodyStructure != nil {
			m.HasAttachments = hasAttachmentParts(buf.BodyStructure)
		}
		m.ThreadID = ThreadKey(m.MessageID, m.InReplyTo, m.References, m.Subject)
		out = append(out, m)
	}
	return out, nil
}

// FetchBody pulls the whole message and extracts its text and HTML parts.
func (cl *client) FetchBody(ctx context.Context, uid uint32) (Body, error) {
	set := imap.UIDSet{imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)}}
	opts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	buffers, err := cl.c.Fetch(set, opts).Collect()
	if err != nil {
		return Body{}, fmt.Errorf("imapx: FETCH body for UID %d failed: %w", uid, err)
	}
	if len(buffers) == 0 {
		return Body{}, fmt.Errorf("imapx: no message with UID %d", uid)
	}

	for _, section := range buffers[0].BodySection {
		html, text, err := splitBodyParts(section.Bytes)
		if err != nil {
			return Body{}, err
		}
		return Body{HTML: html, Text: text}, nil
	}
	return Body{}, fmt.Errorf("imapx: message %d returned no body section", uid)
}
```

`internal/imapx/message_parts.go`:

```go
package imapx

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// splitBodyParts walks a raw RFC 5322 message and returns its HTML and plain
// text representations. Character set decoding is handled by go-message, which
// matters for the ISO-8859-9 mail that Turkish corporate systems still send.
func splitBodyParts(raw []byte) (html, text string, err error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	defer mr.Close()

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// An unknown charset is not fatal: keep whatever parts we already
			// decoded rather than showing the user nothing.
			if message.IsUnknownCharset(err) {
				continue
			}
			return "", "", err
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := h.ContentType()
			body, readErr := io.ReadAll(part.Body)
			if readErr != nil {
				continue
			}
			switch {
			case strings.EqualFold(contentType, "text/html") && html == "":
				html = string(body)
			case strings.EqualFold(contentType, "text/plain") && text == "":
				text = string(body)
			}
		}
	}
	return html, text, nil
}
```

```bash
go get github.com/emersion/go-message@latest
```

- [ ] **Step 8: Testleri ve lint'i çalıştır**

```bash
go test ./internal/imapx/ -race -v
golangci-lint run ./...
```

Beklenen: tüm imapx testleri PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/imapx go.mod go.sum
git commit -m "feat(imapx): fetch headers and bodies, derive thread keys

FetchHeaders deliberately omits bodies: on a 1000-message initial sync
that is the difference between kilobytes and tens of megabytes.

Subject normalisation strips YNT: and ILT: alongside Re: and Fwd:.
Turkish Outlook uses those prefixes, and without them a single corporate
conversation splits into two threads.

Body decoding goes through go-message so ISO-8859-9 mail — still common
from Turkish corporate systems — renders correctly rather than as mojibake."
```

---

### Task 10: Senkron motoru — ilk senkron ve UIDVALIDITY sıfırlaması

Doğrulama dokümanının §2.2'sindeki kabul testi bu görevde yazılıyor. Bu, projenin
sessizce veri kaybedebileceği tek yer.

**Files:**
- Create: `internal/sync/engine.go` (Task 1'deki iskeleti değiştirir)
- Create: `internal/sync/initial.go`, `internal/sync/errors.go`
- Create: `internal/sync/fake_backend_test.go`, `internal/sync/initial_test.go`

**Interfaces:**
- Consumes: `imapx.MailBackend`, `imapx.SelectResult`, `imapx.UIDRange` (Task 8-9); `store.*` (Task 4)
- Produces:
  - `sync.Store` arayüzü — motorun ihtiyaç duyduğu kalıcılık yüzeyi (`*store.Store` bunu karşılar)
  - `sync.Engine`, `sync.New(Store, Dialer) *Engine`
  - `sync.Dialer` func tipi: `func(ctx, accountID int64) (imapx.MailBackend, error)`
  - `(*Engine).InitialSync(ctx, model.Account) error`
  - `sync.ErrorClass` ve `sync.Classify(error) ErrorClass`
  - Sabit: `sync.InitialHeaderCount = 1000`

- [ ] **Step 1: Motorun kalıcılık yüzeyini ve hata sınıflandırmasını yaz**

Motor `*store.Store`'a doğrudan bağlanmıyor; ihtiyaç duyduğu metotları kendi
arayüzü olarak bildiriyor. Böylece testte sahte bir store gerekmez (gerçek SQLite
kullanılır) ama bağımlılık yönü de açıkça görünür.

`internal/sync/engine.go`:

```go
// Package sync keeps the local database in step with mail servers. It talks to
// servers through imapx.MailBackend and never imports go-imap, which is what
// makes it testable against an in-memory server.
package sync

import (
	"context"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// InitialHeaderCount is how many recent messages get headers on first sync.
// Folders beyond the inbox and the special ones are listed only; their headers
// arrive when the user first opens them. On a corporate account with dozens of
// folders, fetching everything up front would take minutes.
const InitialHeaderCount = 1000

// Store is the persistence surface the engine needs. *store.Store satisfies it.
type Store interface {
	UpsertFolders(ctx context.Context, accountID int64, folders []model.Folder) error
	ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error)
	ResetFolder(ctx context.Context, folderID int64, newUIDValidity uint32) error
	UpsertMessages(ctx context.Context, folderID int64, msgs []model.Message) error
	ListMessages(ctx context.Context, folderID int64, limit, offset int) ([]model.Message, error)
	SetMessageBody(ctx context.Context, messageID int64, html, text string) error
}

// Dialer opens an authenticated connection for one account. The engine takes
// this as a function so tests can hand it a fake backend.
type Dialer func(ctx context.Context, accountID int64) (imapx.MailBackend, error)

// Engine performs synchronisation for all accounts.
type Engine struct {
	store Store
	dial  Dialer
}

func New(s Store, d Dialer) *Engine {
	return &Engine{store: s, dial: d}
}
```

`internal/sync/errors.go`:

```go
package sync

import (
	"context"
	"errors"
	"net"
	"strings"
)

// ErrorClass drives how a failure is handled, per the design doc's error table.
type ErrorClass int

const (
	// ClassTransient covers network blips: retry silently with backoff.
	ClassTransient ErrorClass = iota
	// ClassAuth means credentials failed: refresh the token, and if that
	// fails, put the account into "sign-in required" and tell the user.
	ClassAuth
	// ClassProtocol means the server said something we cannot act on: log it,
	// quarantine the affected folder, keep the other folders working.
	ClassProtocol
	// ClassPermanent means retrying cannot help: tell the user, stop.
	ClassPermanent
)

func (c ErrorClass) String() string {
	switch c {
	case ClassTransient:
		return "transient"
	case ClassAuth:
		return "auth"
	case ClassProtocol:
		return "protocol"
	default:
		return "permanent"
	}
}

// Classify sorts an error into one of the four handling classes.
func Classify(err error) ErrorClass {
	if err == nil {
		return ClassTransient
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ClassTransient
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ClassTransient
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "xoauth2") ||
		strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "refresh token") ||
		strings.Contains(msg, "login failed"):
		return ClassAuth
	case strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "timeout"):
		return ClassTransient
	case strings.Contains(msg, "fetch failed") ||
		strings.Contains(msg, "select") ||
		strings.Contains(msg, "list failed") ||
		strings.Contains(msg, "bad command"):
		return ClassProtocol
	}
	return ClassPermanent
}
```

- [ ] **Step 2: Sahte backend'i yaz**

`internal/sync/fake_backend_test.go`:

```go
package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// fakeBackend is a scriptable MailBackend. The imapx package already tests
// against a real in-memory IMAP server; here we need to drive specific server
// behaviours — a UIDVALIDITY bump, a mid-sync failure — which is easier to
// arrange directly.
type fakeBackend struct {
	mu sync.Mutex

	caps     imapx.Capabilities
	folders  []model.Folder
	selected string

	// uidValidity per folder path, so a test can change it between syncs.
	uidValidity map[string]uint32
	// messages per folder path.
	messages map[string][]model.Message
	bodies   map[uint32]imapx.Body

	// failFetchAfter makes FetchHeaders fail once it has served this many
	// messages, simulating a connection drop mid-sync. Zero disables it.
	failFetchAfter int
	served         int

	closed bool
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		caps:        imapx.Capabilities{CondStore: true, QResync: true, Move: true, Idle: true},
		uidValidity: map[string]uint32{},
		messages:    map[string][]model.Message{},
		bodies:      map[uint32]imapx.Body{},
	}
}

func (f *fakeBackend) addFolder(path string, attrs []string, uidValidity uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.folders = append(f.folders, model.Folder{
		Path: path, Name: path, Delimiter: "/", Attributes: attrs,
		UIDValidity: uidValidity,
	})
	f.uidValidity[path] = uidValidity
}

func (f *fakeBackend) addMessages(path string, uids ...uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, uid := range uids {
		f.messages[path] = append(f.messages[path], model.Message{
			UID:          uid,
			MessageID:    fmt.Sprintf("<%s-%d@example.com>", path, uid),
			Subject:      fmt.Sprintf("%s message %d", path, uid),
			From:         model.Address{Name: "Sender", Addr: "sender@example.com"},
			InternalDate: time.Unix(1700000000+int64(uid), 0),
			Size:         1024,
			ThreadID:     fmt.Sprintf("<%s-%d@example.com>", path, uid),
		})
		f.bodies[uid] = imapx.Body{HTML: fmt.Sprintf("<p>body %d</p>", uid), Text: fmt.Sprintf("body %d", uid)}
	}
}

// setUIDValidity simulates the server recreating a mailbox.
func (f *fakeBackend) setUIDValidity(path string, v uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uidValidity[path] = v
	for i := range f.folders {
		if f.folders[i].Path == path {
			f.folders[i].UIDValidity = v
		}
	}
}

func (f *fakeBackend) Capabilities() imapx.Capabilities { return f.caps }

func (f *fakeBackend) ListFolders(ctx context.Context) ([]model.Folder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.Folder, len(f.folders))
	copy(out, f.folders)
	for i := range out {
		out[i].TotalCount = len(f.messages[out[i].Path])
	}
	return out, nil
}

func (f *fakeBackend) Select(ctx context.Context, path string) (imapx.SelectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.uidValidity[path]
	if !ok {
		return imapx.SelectResult{}, fmt.Errorf("SELECT %q failed: no such mailbox", path)
	}
	f.selected = path
	msgs := f.messages[path]
	var next uint32 = 1
	for _, m := range msgs {
		if m.UID >= next {
			next = m.UID + 1
		}
	}
	return imapx.SelectResult{
		UIDValidity:   v,
		UIDNext:       next,
		NumMessages:   uint32(len(msgs)),
		HighestModSeq: 1,
	}, nil
}

func (f *fakeBackend) FetchHeaders(ctx context.Context, r imapx.UIDRange) ([]model.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFetchAfter > 0 && f.served >= f.failFetchAfter {
		return nil, fmt.Errorf("FETCH failed: connection reset by peer")
	}
	var out []model.Message
	for _, m := range f.messages[f.selected] {
		if m.UID < r.Start {
			continue
		}
		if r.End != 0 && m.UID > r.End {
			continue
		}
		out = append(out, m)
	}
	f.served += len(out)
	return out, nil
}

func (f *fakeBackend) FetchBody(ctx context.Context, uid uint32) (imapx.Body, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.bodies[uid]
	if !ok {
		return imapx.Body{}, fmt.Errorf("no message with UID %d", uid)
	}
	return b, nil
}

func (f *fakeBackend) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
```

- [ ] **Step 3: İlk senkron ve UIDVALIDITY testlerini yaz**

`internal/sync/initial_test.go`:

```go
package sync

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

func newTestEngine(t *testing.T, be imapx.MailBackend) (*Engine, *store.Store, model.Account) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	acctID, err := s.InsertAccount(context.Background(), model.Account{
		Email: "user@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "ref", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}

	eng := New(s, func(ctx context.Context, accountID int64) (imapx.MailBackend, error) {
		return be, nil
	})
	return eng, s, model.Account{ID: acctID, Email: "user@example.com"}
}

func folderIDByPath(t *testing.T, s *store.Store, acctID int64, path string) int64 {
	t.Helper()
	folders, err := s.ListFolders(context.Background(), acctID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	for _, f := range folders {
		if f.Path == path {
			return f.ID
		}
	}
	t.Fatalf("no folder with path %q", path)
	return 0
}

func TestInitialSyncStoresFoldersAndInboxHeaders(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addFolder("Projects", nil, 300)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)
	be.addMessages("Projects", 50, 51)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	folders, err := s.ListFolders(ctx, acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("stored %d folders, want 3", len(folders))
	}

	inbox, err := s.ListMessages(ctx, folderIDByPath(t, s, acct.ID, "INBOX"), 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(INBOX) error: %v", err)
	}
	if len(inbox) != 3 {
		t.Errorf("INBOX holds %d messages, want 3", len(inbox))
	}

	sent, err := s.ListMessages(ctx, folderIDByPath(t, s, acct.ID, "Sent"), 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Sent) error: %v", err)
	}
	if len(sent) != 2 {
		t.Errorf("Sent holds %d messages, want 2 (special folders sync eagerly)", len(sent))
	}

	// A plain folder is listed but its headers wait for the user to open it.
	projects, err := s.ListMessages(ctx, folderIDByPath(t, s, acct.ID, "Projects"), 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Projects) error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("ordinary folder holds %d messages after initial sync, want 0 (lazy)", len(projects))
	}
}

func TestInitialSyncIsIdempotent(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 1, 2, 3)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := eng.InitialSync(ctx, acct); err != nil {
			t.Fatalf("InitialSync() run %d error: %v", i+1, err)
		}
	}

	msgs, err := s.ListMessages(ctx, folderIDByPath(t, s, acct.ID, "INBOX"), 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("after three syncs INBOX holds %d messages, want 3", len(msgs))
	}
}

// This is the acceptance test from the verification doc §2.2. Without it the
// app would show — and act on — messages whose UIDs no longer mean anything.
func TestUIDValidityChangeResetsOnlyThatFolder(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("first InitialSync() error: %v", err)
	}
	inboxID := folderIDByPath(t, s, acct.ID, "INBOX")
	sentID := folderIDByPath(t, s, acct.ID, "Sent")

	before, _ := s.ListMessages(ctx, inboxID, 100, 0)
	if len(before) != 3 {
		t.Fatalf("precondition failed: INBOX holds %d messages, want 3", len(before))
	}

	// The server recreates INBOX: same paths, brand new UID space.
	be.setUIDValidity("INBOX", 999)
	be.mu.Lock()
	be.messages["INBOX"] = nil
	be.mu.Unlock()
	be.addMessages("INBOX", 1, 2)

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("second InitialSync() error: %v", err)
	}

	inbox, err := s.ListMessages(ctx, inboxID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(INBOX) error: %v", err)
	}
	if len(inbox) != 2 {
		t.Errorf("INBOX holds %d messages after the UIDVALIDITY change, want exactly the 2 refetched ones", len(inbox))
	}

	// The sibling folder's cache is still valid and must survive untouched.
	sent, err := s.ListMessages(ctx, sentID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Sent) error: %v", err)
	}
	if len(sent) != 2 {
		t.Errorf("sibling folder holds %d messages, want 2 — the reset leaked beyond INBOX", len(sent))
	}

	folders, _ := s.ListFolders(ctx, acct.ID)
	for _, f := range folders {
		if f.Path == "INBOX" && f.UIDValidity != 999 {
			t.Errorf("stored INBOX UIDValidity = %d, want 999", f.UIDValidity)
		}
		if f.Path == "Sent" && f.UIDValidity != 200 {
			t.Errorf("stored Sent UIDValidity = %d, want it unchanged at 200", f.UIDValidity)
		}
	}
}

func TestInitialSyncSurvivesMidSyncConnectionDrop(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)
	// Fail after the first folder's messages have been served.
	be.failFetchAfter = 3

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	err := eng.InitialSync(ctx, acct)
	if err == nil {
		t.Fatal("InitialSync() reported success despite a connection drop")
	}
	if got := Classify(err); got != ClassTransient {
		t.Errorf("Classify(err) = %v, want transient so the engine retries", got)
	}

	// Whatever landed before the failure must still be readable: a partial
	// sync leaves usable data rather than an empty mailbox.
	inbox, listErr := s.ListMessages(ctx, folderIDByPath(t, s, acct.ID, "INBOX"), 100, 0)
	if listErr != nil {
		t.Fatalf("ListMessages() error: %v", listErr)
	}
	if len(inbox) != 3 {
		t.Errorf("INBOX holds %d messages after a partial sync, want the 3 that arrived before the drop", len(inbox))
	}
}
```

- [ ] **Step 4: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/sync/ -v
```

Beklenen: FAIL — `eng.InitialSync undefined`

- [ ] **Step 5: İlk senkronu yaz**

`internal/sync/initial.go`:

```go
package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// eagerAttributes marks the folders worth syncing headers for immediately.
// Everything else waits until the user opens it.
var eagerAttributes = []string{"\\Sent", "\\Drafts", "\\Trash", "\\Junk", "\\Archive"}

// InitialSync discovers folders, then fetches headers for the inbox and the
// special folders. Ordinary folders are recorded but left empty; SyncFolder
// fills them in on first open.
func (e *Engine) InitialSync(ctx context.Context, acct model.Account) error {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}

	remote, err := be.ListFolders(ctx)
	if err != nil {
		return fmt.Errorf("sync: listing folders for account %d: %w", acct.ID, err)
	}
	if err := e.store.UpsertFolders(ctx, acct.ID, remote); err != nil {
		return fmt.Errorf("sync: storing folders: %w", err)
	}

	local, err := e.store.ListFolders(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: reading stored folders: %w", err)
	}

	for _, folder := range local {
		if !isEager(folder) {
			continue
		}
		if err := e.syncFolderHeaders(ctx, be, acct.ID, folder); err != nil {
			return err
		}
	}
	return nil
}

// SyncFolder fetches headers for one folder on demand, used when the user
// opens a folder that initial sync left empty.
func (e *Engine) SyncFolder(ctx context.Context, acct model.Account, folder model.Folder) error {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	return e.syncFolderHeaders(ctx, be, acct.ID, folder)
}

// syncFolderHeaders selects a folder, reconciles UIDVALIDITY and writes headers.
//
// The UIDVALIDITY check must happen before any message is written. If the
// server has recreated the mailbox, every UID we hold refers to a different
// message than it used to, so acting on stale rows would touch the wrong mail.
func (e *Engine) syncFolderHeaders(ctx context.Context, be imapx.MailBackend, accountID int64, folder model.Folder) error {
	sel, err := be.Select(ctx, folder.Path)
	if err != nil {
		return fmt.Errorf("sync: selecting %q: %w", folder.Path, err)
	}

	if folder.UIDValidity != 0 && sel.UIDValidity != folder.UIDValidity {
		if err := e.store.ResetFolder(ctx, folder.ID, sel.UIDValidity); err != nil {
			return fmt.Errorf("sync: resetting %q after a UIDVALIDITY change: %w", folder.Path, err)
		}
		folder.UIDValidity = sel.UIDValidity
	}

	start := uint32(1)
	if sel.UIDNext > InitialHeaderCount {
		start = sel.UIDNext - InitialHeaderCount
	}

	msgs, err := be.FetchHeaders(ctx, imapx.UIDRange{Start: start, End: 0})
	if err != nil {
		return fmt.Errorf("sync: fetching headers for %q: %w", folder.Path, err)
	}

	for i := range msgs {
		msgs[i].AccountID = accountID
		msgs[i].FolderID = folder.ID
	}
	if err := e.store.UpsertMessages(ctx, folder.ID, msgs); err != nil {
		return fmt.Errorf("sync: storing headers for %q: %w", folder.Path, err)
	}

	updated := folder
	updated.UIDNext = sel.UIDNext
	updated.HighestModSeq = sel.HighestModSeq
	updated.LastSyncedAt = time.Now()
	if err := e.store.UpsertFolders(ctx, accountID, []model.Folder{updated}); err != nil {
		return fmt.Errorf("sync: recording sync state for %q: %w", folder.Path, err)
	}
	return nil
}

func isEager(f model.Folder) bool {
	if strings.EqualFold(f.Path, "INBOX") {
		return true
	}
	for _, attr := range f.Attributes {
		for _, eager := range eagerAttributes {
			if strings.EqualFold(attr, eager) {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 6: `SetMessageBody`'yi store'a ekle**

`sync.Store` arayüzü bunu istiyor. `internal/store/bodies.go`:

```go
package store

import "context"

// SetMessageBody stores a fetched body and marks the message as having one.
// The body lives in its own table so listing a mailbox never reads it.
func (s *Store) SetMessageBody(ctx context.Context, messageID int64, html, text string) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO message_bodies (message_id, html_body, text_body)
		 VALUES (?, ?, ?)
		 ON CONFLICT(message_id) DO UPDATE SET
		   html_body = excluded.html_body,
		   text_body = excluded.text_body`,
		messageID, html, text); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE messages SET body_fetched = 1 WHERE id = ?`, messageID); err != nil {
		return err
	}
	// Keep the search index in step: the body column is what P3 will query.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet, body)
		 SELECT 'delete', id, subject, from_addr, snippet, '' FROM messages WHERE id = ?`,
		messageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO fts_messages(rowid, subject, from_addr, snippet, body)
		 SELECT id, subject, from_addr, snippet, ? FROM messages WHERE id = ?`,
		text, messageID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetMessageBody returns a stored body, or empty strings when none is cached.
func (s *Store) GetMessageBody(ctx context.Context, messageID int64) (html, text string, err error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT html_body, text_body FROM message_bodies WHERE message_id = ?`, messageID)
	err = row.Scan(&html, &text)
	if err != nil && err.Error() == "sql: no rows in result set" {
		return "", "", nil
	}
	return html, text, err
}
```

- [ ] **Step 7: Testlerin geçtiğini doğrula**

```bash
go test ./internal/sync/ ./internal/store/ -race -v
golangci-lint run ./...
```

Beklenen: dört sync testi PASS. Lint `sync` paketinin go-imap import etmediğini
de doğrulamış olur.

- [ ] **Step 8: Commit**

```bash
git add internal/sync internal/store/bodies.go
git commit -m "feat(sync): initial sync with per-folder UIDVALIDITY reconciliation

The UIDVALIDITY check runs before any message is written. If the server
recreated a mailbox, every UID we hold now refers to a different message,
so acting on stale rows would touch the wrong mail.

A test asserts the reset is scoped to one folder: its siblings' caches
are still valid and must survive. Another asserts a mid-sync connection
drop leaves the already-fetched headers readable rather than rolling the
mailbox back to empty."
```

---

### Task 11: Talep üzerine gövde indirme

**Files:**
- Create: `internal/sync/body.go`, `internal/sync/body_test.go`

**Interfaces:**
- Consumes: `Engine`, `Store.SetMessageBody` (Task 10); `imapx.MailBackend.FetchBody` (Task 9)
- Produces: `(*Engine).EnsureBody(ctx, acct model.Account, folder model.Folder, msg model.Message) (imapx.Body, error)`

- [ ] **Step 1: Başarısız test yaz**

`internal/sync/body_test.go`:

```go
package sync

import (
	"context"
	"testing"
)

func TestEnsureBodyFetchesThenServesFromCache(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 7)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()
	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	folders, _ := s.ListFolders(ctx, acct.ID)
	msgs, _ := s.ListMessages(ctx, folders[0].ID, 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].BodyFetched {
		t.Fatal("precondition failed: body already marked as fetched")
	}

	body, err := eng.EnsureBody(ctx, acct, folders[0], msgs[0])
	if err != nil {
		t.Fatalf("EnsureBody() error: %v", err)
	}
	if body.HTML != "<p>body 7</p>" {
		t.Errorf("Body.HTML = %q, want <p>body 7</p>", body.HTML)
	}

	// The body must now be cached, so a second call works with the server
	// unreachable — this is the local-first promise.
	be.mu.Lock()
	be.bodies = nil
	be.mu.Unlock()

	again, err := eng.EnsureBody(ctx, acct, folders[0], msgs[0])
	if err != nil {
		t.Fatalf("second EnsureBody() error: %v", err)
	}
	if again.HTML != "<p>body 7</p>" {
		t.Errorf("cached Body.HTML = %q, want <p>body 7</p>", again.HTML)
	}

	stored, _ := s.ListMessages(ctx, folders[0].ID, 10, 0)
	if !stored[0].BodyFetched {
		t.Error("BodyFetched = false after EnsureBody; the message list cannot tell what is cached")
	}
}
```

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu gör**

```bash
go test ./internal/sync/ -run TestEnsureBody -v
```

Beklenen: FAIL — `eng.EnsureBody undefined`

- [ ] **Step 3: `EnsureBody`'yi yaz**

`internal/sync/body.go`:

```go
package sync

import (
	"context"
	"fmt"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// BodyReader is the read side the engine needs for cached bodies. Kept separate
// from Store so a caller can supply a read-only implementation.
type BodyReader interface {
	GetMessageBody(ctx context.Context, messageID int64) (html, text string, err error)
}

// EnsureBody returns a message body, fetching it from the server only when it
// is not already cached. Once cached, the message reads fine with no network —
// which is the whole point of a local-first client.
func (e *Engine) EnsureBody(ctx context.Context, acct model.Account, folder model.Folder, msg model.Message) (imapx.Body, error) {
	if reader, ok := e.store.(BodyReader); ok {
		html, text, err := reader.GetMessageBody(ctx, msg.ID)
		if err != nil {
			return imapx.Body{}, fmt.Errorf("sync: reading cached body: %w", err)
		}
		if html != "" || text != "" {
			return imapx.Body{HTML: html, Text: text}, nil
		}
	}

	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return imapx.Body{}, fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	if _, err := be.Select(ctx, folder.Path); err != nil {
		return imapx.Body{}, fmt.Errorf("sync: selecting %q: %w", folder.Path, err)
	}

	body, err := be.FetchBody(ctx, msg.UID)
	if err != nil {
		return imapx.Body{}, fmt.Errorf("sync: fetching body for UID %d: %w", msg.UID, err)
	}
	if err := e.store.SetMessageBody(ctx, msg.ID, body.HTML, body.Text); err != nil {
		return imapx.Body{}, fmt.Errorf("sync: caching body: %w", err)
	}
	return body, nil
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

```bash
go test ./internal/sync/ -race -v
```

Beklenen: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/sync/body.go internal/sync/body_test.go
git commit -m "feat(sync): fetch message bodies on demand and cache them

The test blanks the fake server's bodies before the second call, so the
cached read is proved to work with nothing reachable. That is the
local-first promise stated as an assertion rather than an intention."
```

---

### Task 12: Wails servisi ve uygulama bağlantısı

**Files:**
- Create: `internal/app/service.go` (Task 1'deki iskeleti değiştirir)
- Create: `internal/app/events.go`, `internal/app/dto.go`
- Create: `internal/app/service_test.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: `store.*` (Task 4, 10), `auth.*` (Task 5-7), `sync.Engine` (Task 10-11)
- Produces:
  - `app.MailService` — Wails'e kaydedilen servis
  - `app.NewMailService(*store.Store, auth.SecretStore, *sync.Engine, Config) *MailService`
  - Frontend'e açılan metotlar: `ListAccounts`, `AddPasswordAccount`, `AddOAuthAccount`, `ListFolders`, `ListMessages`, `GetBody`, `SyncAccount`
  - Olay adları: `app.EventSyncStarted`, `app.EventSyncFinished`, `app.EventSyncFailed`

- [ ] **Step 1: Wails v3 servis ve olay API'sini doğrula**

```bash
go doc github.com/wailsapp/wails/v3/pkg/application Options
go doc github.com/wailsapp/wails/v3/pkg/application NewService
go doc github.com/wailsapp/wails/v3/pkg/application App.EmitEvent
```

Aşağıdaki `main.go` bu imzaları varsayıyor. Beta sürümünde farklıysa çıktıya göre
düzelt; `internal/app` paketi Wails'e bağımlı olmadığı için (olay yayını bir
fonksiyon alanı üzerinden yapılıyor) yalnızca `main.go` değişir.

- [ ] **Step 2: DTO'ları yaz**

Frontend'e model tiplerini doğrudan vermiyoruz: `SecretRef` gibi alanların
arayüze sızmaması gerekiyor ve tarih formatı JSON'da açık olmalı.

`internal/app/dto.go`:

```go
package app

import (
	"nexusmail/internal/model"
)

// AccountDTO is what the UI sees. SecretRef is deliberately absent: the
// frontend has no business knowing where credentials live.
type AccountDTO struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider"`
	AuthKind    string `json:"authKind"`
}

type FolderDTO struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"accountId"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	TotalCount  int    `json:"totalCount"`
	UnreadCount int    `json:"unreadCount"`
	IsInbox     bool   `json:"isInbox"`
}

type MessageDTO struct {
	ID             int64  `json:"id"`
	FolderID       int64  `json:"folderId"`
	UID            uint32 `json:"uid"`
	ThreadID       string `json:"threadId"`
	Subject        string `json:"subject"`
	FromName       string `json:"fromName"`
	FromAddr       string `json:"fromAddr"`
	Snippet        string `json:"snippet"`
	// InternalDateUnix is seconds since the epoch. An explicit integer avoids
	// the timezone ambiguity that string dates cause across the JS boundary.
	InternalDateUnix int64 `json:"internalDateUnix"`
	IsRead           bool  `json:"isRead"`
	IsStarred        bool  `json:"isStarred"`
	HasAttachments   bool  `json:"hasAttachments"`
	BodyFetched      bool  `json:"bodyFetched"`
}

type BodyDTO struct {
	HTML string `json:"html"`
	Text string `json:"text"`
}

func accountToDTO(a model.Account) AccountDTO {
	return AccountDTO{
		ID: a.ID, Email: a.Email, DisplayName: a.DisplayName,
		Provider: string(a.Provider), AuthKind: string(a.AuthKind),
	}
}

func folderToDTO(f model.Folder) FolderDTO {
	isInbox := len(f.Path) == 5 &&
		(f.Path == "INBOX" || f.Path == "inbox" || f.Path == "Inbox")
	return FolderDTO{
		ID: f.ID, AccountID: f.AccountID, Name: f.Name, Path: f.Path,
		TotalCount: f.TotalCount, UnreadCount: f.UnreadCount, IsInbox: isInbox,
	}
}

func messageToDTO(m model.Message) MessageDTO {
	return MessageDTO{
		ID: m.ID, FolderID: m.FolderID, UID: m.UID, ThreadID: m.ThreadID,
		Subject: m.Subject, FromName: m.From.Name, FromAddr: m.From.Addr,
		Snippet: m.Snippet, InternalDateUnix: m.InternalDate.Unix(),
		IsRead:    m.HasFlag("\\Seen"),
		IsStarred: m.HasFlag("\\Flagged"),
		HasAttachments: m.HasAttachments, BodyFetched: m.BodyFetched,
	}
}
```

- [ ] **Step 3: Olay adlarını yaz**

`internal/app/events.go`:

```go
package app

// Event names shared with the frontend. Keep them in sync with
// frontend/src/lib/events.ts.
const (
	EventSyncStarted  = "sync:started"
	EventSyncFinished = "sync:finished"
	EventSyncFailed   = "sync:failed"
)

// SyncEvent is the payload for all three sync events.
type SyncEvent struct {
	AccountID int64  `json:"accountId"`
	Email     string `json:"email"`
	// Error is empty except on sync:failed.
	Error string `json:"error,omitempty"`
	// Class is the error class name on failure: transient, auth, protocol or
	// permanent. The UI decides whether to show a retry or a sign-in prompt.
	Class string `json:"class,omitempty"`
}

// Emitter delivers events to the frontend. main.go supplies a function backed
// by the Wails runtime; tests supply a recorder. This is why internal/app does
// not import Wails at all.
type Emitter func(name string, data any)
```

- [ ] **Step 4: Servis için başarısız test yaz**

`internal/app/service_test.go`:

```go
package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"nexusmail/internal/auth"
	imapsync "nexusmail/internal/sync"
	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

// stubBackend is the smallest MailBackend that lets the service be exercised.
type stubBackend struct{}

func (stubBackend) Capabilities() imapx.Capabilities { return imapx.Capabilities{} }
func (stubBackend) ListFolders(context.Context) ([]model.Folder, error) {
	return []model.Folder{{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 1}}, nil
}
func (stubBackend) Select(context.Context, string) (imapx.SelectResult, error) {
	return imapx.SelectResult{UIDValidity: 1, UIDNext: 2, NumMessages: 1}, nil
}
func (stubBackend) FetchHeaders(context.Context, imapx.UIDRange) ([]model.Message, error) {
	return []model.Message{{
		UID: 1, Subject: "Hello", From: model.Address{Name: "A", Addr: "a@example.com"},
		InternalDate: time.Unix(1700000000, 0), Flags: []string{"\\Seen"},
	}}, nil
}
func (stubBackend) FetchBody(context.Context, uint32) (imapx.Body, error) {
	return imapx.Body{HTML: "<p>hi</p>", Text: "hi"}, nil
}
func (stubBackend) Close() error { return nil }

func newTestService(t *testing.T) (*MailService, *recorder) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	secrets, err := auth.NewFileStore(dir, "test-master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	eng := imapsync.New(s, func(context.Context, int64) (imapx.MailBackend, error) {
		return stubBackend{}, nil
	})

	rec := &recorder{}
	svc := NewMailService(s, secrets, eng, Config{Emit: rec.emit})
	return svc, rec
}

func TestAddPasswordAccountStoresSecretOutsideTheDatabase(t *testing.T) {
	svc, _ := newTestService(t)

	acct, err := svc.AddPasswordAccount("u@example.com", "Test User",
		"imap.example.com", 993, "smtp.example.com", 587, "app-password")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if acct.ID == 0 {
		t.Fatal("returned account has ID 0")
	}
	if acct.AuthKind != string(model.AuthPassword) {
		t.Errorf("AuthKind = %q, want password", acct.AuthKind)
	}

	// The DTO must not carry the secret reference, let alone the secret.
	if strings.Contains(acct.Email, "app-password") {
		t.Error("the DTO leaked the password")
	}

	accounts, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts() returned %d accounts, want 1", len(accounts))
	}
}

func TestSyncAccountEmitsStartAndFinish(t *testing.T) {
	svc, rec := newTestService(t)

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	got := rec.names()
	if len(got) != 2 || got[0] != EventSyncStarted || got[1] != EventSyncFinished {
		t.Errorf("events = %v, want [%s %s]", got, EventSyncStarted, EventSyncFinished)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) != 1 || !folders[0].IsInbox {
		t.Fatalf("folders = %+v, want a single folder flagged as the inbox", folders)
	}

	msgs, err := svc.ListMessages(folders[0].ID, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("ListMessages() returned %d messages, want 1", len(msgs))
	}
	if msgs[0].Subject != "Hello" {
		t.Errorf("Subject = %q, want Hello", msgs[0].Subject)
	}
	if !msgs[0].IsRead {
		t.Error("IsRead = false, but the message carries \\Seen")
	}
	if msgs[0].InternalDateUnix != 1700000000 {
		t.Errorf("InternalDateUnix = %d, want 1700000000", msgs[0].InternalDateUnix)
	}
}

func TestGetBodyReturnsFetchedBody(t *testing.T) {
	svc, _ := newTestService(t)
	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	folders, _ := svc.ListFolders(acct.ID)
	msgs, _ := svc.ListMessages(folders[0].ID, 50, 0)

	body, err := svc.GetBody(msgs[0].ID)
	if err != nil {
		t.Fatalf("GetBody() error: %v", err)
	}
	if body.HTML != "<p>hi</p>" {
		t.Errorf("HTML = %q, want <p>hi</p>", body.HTML)
	}
}
```

- [ ] **Step 5: Testleri çalıştır, başarısız olduklarını gör**

```bash
go test ./internal/app/ -v
```

Beklenen: FAIL — `undefined: NewMailService`

- [ ] **Step 6: Servisi yaz**

`internal/app/service.go`:

```go
// Package app exposes the application's capabilities to the frontend. It is the
// only layer the UI talks to, and it deliberately does not import Wails: event
// delivery arrives as a function so the service stays unit-testable.
package app

import (
	"context"
	"fmt"
	"time"

	"nexusmail/internal/auth"
	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

// Config carries wiring the service cannot construct itself.
type Config struct {
	Emit Emitter
	// GoogleClientID and MicrosoftClientID come from config.json. They are not
	// secrets: a desktop app is a public OAuth client.
	GoogleClientID    string
	MicrosoftClientID string
}

type MailService struct {
	store   *store.Store
	secrets auth.SecretStore
	engine  *imapsync.Engine
	cfg     Config
}

func NewMailService(s *store.Store, secrets auth.SecretStore, eng *imapsync.Engine, cfg Config) *MailService {
	if cfg.Emit == nil {
		cfg.Emit = func(string, any) {}
	}
	return &MailService{store: s, secrets: secrets, engine: eng, cfg: cfg}
}

func (s *MailService) ListAccounts() ([]AccountDTO, error) {
	accounts, err := s.store.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]AccountDTO, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountToDTO(a))
	}
	return out, nil
}

// AddPasswordAccount registers an account authenticated with a password or app
// password. The secret goes straight to the SecretStore; only its ref is
// written to the database.
func (s *MailService) AddPasswordAccount(email, displayName, imapHost string, imapPort int,
	smtpHost string, smtpPort int, password string) (AccountDTO, error) {

	ctx := context.Background()
	provider := model.ProviderGeneric
	if preset, ok := auth.PresetFor(email); ok {
		provider = preset.Provider
		if imapHost == "" {
			imapHost, imapPort = preset.IMAPHost, preset.IMAPPort
			smtpHost, smtpPort = preset.SMTPHost, preset.SMTPPort
		}
	}
	if imapHost == "" {
		return AccountDTO{}, fmt.Errorf("app: no IMAP host given and no preset for %q", email)
	}

	secretRef := "account:" + email
	if err := s.secrets.Set(secretRef, password); err != nil {
		return AccountDTO{}, fmt.Errorf("app: storing credentials: %w", err)
	}

	acct := model.Account{
		Email: email, DisplayName: displayName, Provider: provider,
		AuthKind: model.AuthPassword, IMAPHost: imapHost, IMAPPort: imapPort,
		SMTPHost: smtpHost, SMTPPort: smtpPort,
		SecretRef: secretRef, CreatedAt: time.Now(),
	}
	id, err := s.store.InsertAccount(ctx, acct)
	if err != nil {
		// Do not leave an orphaned secret behind if the row could not be written.
		_ = s.secrets.Delete(secretRef)
		return AccountDTO{}, fmt.Errorf("app: saving account: %w", err)
	}
	acct.ID = id
	return accountToDTO(acct), nil
}

// AddOAuthAccount runs the loopback flow, stores the refresh token and records
// the account. It blocks while the user completes sign-in in their browser.
func (s *MailService) AddOAuthAccount(email, displayName, providerName string) (AccountDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var cfg auth.OAuthConfig
	provider := model.Provider(providerName)
	switch provider {
	case model.ProviderGoogle:
		if s.cfg.GoogleClientID == "" {
			return AccountDTO{}, fmt.Errorf("app: no Google OAuth client ID configured; see the setup guide in README")
		}
		cfg = auth.OAuthConfig{
			ClientID: s.cfg.GoogleClientID,
			Scopes:   auth.GoogleScopes(),
			Endpoint: auth.GoogleEndpoint(),
		}
	case model.ProviderMicrosoft:
		if s.cfg.MicrosoftClientID == "" {
			return AccountDTO{}, fmt.Errorf("app: no Microsoft OAuth client ID configured; see the setup guide in README")
		}
		cfg = auth.OAuthConfig{
			ClientID: s.cfg.MicrosoftClientID,
			Scopes:   auth.MicrosoftScopes(),
			Endpoint: auth.MicrosoftEndpoint(),
		}
	default:
		return AccountDTO{}, fmt.Errorf("app: provider %q does not support OAuth", providerName)
	}

	tok, err := auth.RunLoopbackFlow(ctx, cfg)
	if err != nil {
		return AccountDTO{}, err
	}
	if tok.RefreshToken == "" {
		return AccountDTO{}, fmt.Errorf("app: provider returned no refresh token; offline access was not granted")
	}

	secretRef := "account:" + email
	if err := s.secrets.Set(secretRef, tok.RefreshToken); err != nil {
		return AccountDTO{}, fmt.Errorf("app: storing refresh token: %w", err)
	}

	preset, ok := auth.PresetFor(email)
	if !ok {
		// Work/school addresses are not in the preset table, but Exchange
		// Online always answers on this host.
		preset = auth.Preset{
			IMAPHost: "outlook.office365.com", IMAPPort: 993,
			SMTPHost: "smtp.office365.com", SMTPPort: 587,
		}
		if provider == model.ProviderGoogle {
			preset = auth.Preset{
				IMAPHost: "imap.gmail.com", IMAPPort: 993,
				SMTPHost: "smtp.gmail.com", SMTPPort: 587,
			}
		}
	}

	acct := model.Account{
		Email: email, DisplayName: displayName, Provider: provider,
		AuthKind: model.AuthOAuth,
		IMAPHost: preset.IMAPHost, IMAPPort: preset.IMAPPort,
		SMTPHost: preset.SMTPHost, SMTPPort: preset.SMTPPort,
		SecretRef: secretRef, CreatedAt: time.Now(),
	}
	id, err := s.store.InsertAccount(context.Background(), acct)
	if err != nil {
		_ = s.secrets.Delete(secretRef)
		return AccountDTO{}, fmt.Errorf("app: saving account: %w", err)
	}
	acct.ID = id
	return accountToDTO(acct), nil
}

func (s *MailService) SyncAccount(accountID int64) error {
	ctx := context.Background()
	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return err
	}
	var acct model.Account
	for _, a := range accounts {
		if a.ID == accountID {
			acct = a
			break
		}
	}
	if acct.ID == 0 {
		return fmt.Errorf("app: no account with id %d", accountID)
	}

	s.cfg.Emit(EventSyncStarted, SyncEvent{AccountID: acct.ID, Email: acct.Email})

	if err := s.engine.InitialSync(ctx, acct); err != nil {
		s.cfg.Emit(EventSyncFailed, SyncEvent{
			AccountID: acct.ID, Email: acct.Email,
			Error: err.Error(), Class: imapsync.Classify(err).String(),
		})
		return err
	}

	s.cfg.Emit(EventSyncFinished, SyncEvent{AccountID: acct.ID, Email: acct.Email})
	return nil
}

func (s *MailService) ListFolders(accountID int64) ([]FolderDTO, error) {
	folders, err := s.store.ListFolders(context.Background(), accountID)
	if err != nil {
		return nil, err
	}
	out := make([]FolderDTO, 0, len(folders))
	for _, f := range folders {
		out = append(out, folderToDTO(f))
	}
	return out, nil
}

func (s *MailService) ListMessages(folderID int64, limit, offset int) ([]MessageDTO, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	msgs, err := s.store.ListMessages(context.Background(), folderID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]MessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageToDTO(m))
	}
	return out, nil
}

// GetBody returns a message body, fetching it from the server on first view.
func (s *MailService) GetBody(messageID int64) (BodyDTO, error) {
	ctx := context.Background()

	msg, folder, acct, err := s.locate(ctx, messageID)
	if err != nil {
		return BodyDTO{}, err
	}
	body, err := s.engine.EnsureBody(ctx, acct, folder, msg)
	if err != nil {
		return BodyDTO{}, err
	}
	return BodyDTO{HTML: body.HTML, Text: body.Text}, nil
}

// locate resolves a message id back to its message, folder and account.
func (s *MailService) locate(ctx context.Context, messageID int64) (model.Message, model.Folder, model.Account, error) {
	var folderID, accountID int64
	row := s.store.Read().QueryRowContext(ctx,
		`SELECT folder_id, account_id FROM messages WHERE id = ?`, messageID)
	if err := row.Scan(&folderID, &accountID); err != nil {
		return model.Message{}, model.Folder{}, model.Account{},
			fmt.Errorf("app: no message with id %d: %w", messageID, err)
	}

	folders, err := s.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	var folder model.Folder
	for _, f := range folders {
		if f.ID == folderID {
			folder = f
			break
		}
	}

	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	var acct model.Account
	for _, a := range accounts {
		if a.ID == accountID {
			acct = a
			break
		}
	}

	msgs, err := s.store.ListMessages(ctx, folderID, 500, 0)
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	for _, m := range msgs {
		if m.ID == messageID {
			return m, folder, acct, nil
		}
	}
	return model.Message{}, model.Folder{}, model.Account{},
		fmt.Errorf("app: message %d is not in the first 500 of its folder", messageID)
}

// dialerFor builds the sync engine's dialer: it resolves an account to a
// credential provider and opens an IMAP connection.
func DialerFor(s *store.Store, secrets auth.SecretStore, cfg Config) imapsync.Dialer {
	return func(ctx context.Context, accountID int64) (imapx.MailBackend, error) {
		accounts, err := s.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		var acct model.Account
		for _, a := range accounts {
			if a.ID == accountID {
				acct = a
				break
			}
		}
		if acct.ID == 0 {
			return nil, fmt.Errorf("app: no account with id %d", accountID)
		}

		var provider auth.CredentialProvider
		switch acct.AuthKind {
		case model.AuthPassword:
			provider = auth.NewPasswordProvider(acct.Email, acct.SecretRef, secrets)
		case model.AuthOAuth:
			oauthCfg := auth.OAuthConfig{ClientID: cfg.GoogleClientID, Scopes: auth.GoogleScopes(), Endpoint: auth.GoogleEndpoint()}
			if acct.Provider == model.ProviderMicrosoft {
				oauthCfg = auth.OAuthConfig{ClientID: cfg.MicrosoftClientID, Scopes: auth.MicrosoftScopes(), Endpoint: auth.MicrosoftEndpoint()}
			}
			provider = auth.NewOAuthProvider(acct.Email, acct.SecretRef, secrets, oauthCfg)
		default:
			return nil, fmt.Errorf("app: unknown auth kind %q", acct.AuthKind)
		}

		return imapx.Dial(ctx, imapx.Config{
			Host: acct.IMAPHost, Port: acct.IMAPPort, TLS: true, Username: acct.Email,
		}, provider)
	}
}
```

- [ ] **Step 7: Testlerin geçtiğini doğrula**

```bash
go test ./internal/app/ -race -v
golangci-lint run ./...
```

Beklenen: üç test PASS.

- [ ] **Step 8: `main.go`'yu bağla**

`main.go`:

```go
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"nexusmail/internal/app"
	"nexusmail/internal/auth"
	"nexusmail/internal/paths"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dir, err := paths.DataDir()
	if err != nil {
		log.Fatalf("cannot resolve the data directory: %v", err)
	}

	db, err := store.Open(dir)
	if err != nil {
		log.Fatalf("cannot open the database: %v", err)
	}
	defer db.Close()

	secrets, err := auth.Default(dir)
	if err != nil {
		log.Fatalf("cannot open a credential store: %v", err)
	}

	cfg, err := app.LoadConfig(dir)
	if err != nil {
		log.Fatalf("cannot read config.json: %v", err)
	}

	var wailsApp *application.App
	cfg.Emit = func(name string, data any) {
		if wailsApp != nil {
			wailsApp.EmitEvent(name, data)
		}
	}

	engine := imapsync.New(db, app.DialerFor(db, secrets, cfg))
	service := app.NewMailService(db, secrets, engine, cfg)

	wailsApp = application.New(application.Options{
		Name:        "Nexus Mail",
		Description: "Local-first, privacy-focused mail client",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	wailsApp.NewWebviewWindowWithOptions(application.WebviewWindowOptions{
		Title:  "Nexus Mail",
		Width:  1280,
		Height: 800,
		// A mail client is a dense, information-heavy app; below this width the
		// three-column layout stops being usable.
		MinWidth:  900,
		MinHeight: 600,
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatalf("application exited with an error: %v", err)
	}
}
```

`internal/app/config.go`:

```go
package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

type fileConfig struct {
	GoogleClientID    string `json:"googleClientId"`
	MicrosoftClientID string `json:"microsoftClientId"`
}

// LoadConfig reads config.json from the data directory, creating an empty one
// on first run. OAuth client IDs live here rather than in the SecretStore: for
// a public desktop client they are identifiers, not secrets.
func LoadConfig(dir string) (Config, error) {
	path := filepath.Join(dir, configFileName)

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		blank, marshalErr := json.MarshalIndent(fileConfig{}, "", "  ")
		if marshalErr != nil {
			return Config{}, marshalErr
		}
		if writeErr := os.WriteFile(path, blank, 0o600); writeErr != nil {
			return Config{}, writeErr
		}
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}

	var fc fileConfig
	if err := json.Unmarshal(raw, &fc); err != nil {
		return Config{}, err
	}
	return Config{
		GoogleClientID:    fc.GoogleClientID,
		MicrosoftClientID: fc.MicrosoftClientID,
	}, nil
}
```

- [ ] **Step 9: Derlemeyi ve binding üretimini doğrula**

```bash
CGO_ENABLED=0 go build ./...
wails3 generate bindings
ls frontend/src/lib/bindings
```

Beklenen: derleme başarılı; binding dizininde `MailService` için üretilmiş
TypeScript dosyaları görünür. Komut adı beta sürümde farklıysa Step 1'deki
`wails3 -help` çıktısına bak.

- [ ] **Step 10: Commit**

```bash
git add internal/app main.go
git commit -m "feat(app): expose the mail service to the frontend

internal/app does not import Wails: events are delivered through an
Emitter function supplied by main.go, so every service method is
unit-testable without a running window.

DTOs deliberately omit SecretRef — the frontend has no business knowing
where credentials live — and carry dates as Unix seconds to avoid the
timezone ambiguity string dates cause across the JS boundary.

If InsertAccount fails after the secret is stored, the secret is deleted
rather than left orphaned in the keyring."
```

---

### Task 13: Sanitizer ve izleyici engelleme

**Doğrulama dokümanından sapma, gerekçesiyle:** Doğrulama dokümanı §3.1 "gerçek bir
yerel dinleyiciye istek düşmediğini otomatik doğrula" diyordu. jsdom görüntüleri
gerçekten indirmediği için bu iddia jsdom'da anlamlı biçimde test edilemez; gerçek
bir tarayıcı gerekir. M1'de iki katmanlı doğrulama yapıyoruz:

1. **Otomatik ve deterministik:** Sanitizer'ın çıktısında hiçbir uzak URL kalmadığını
   iddia eden birim testleri. Uzak istek atacak bir öznitelik hiç üretilmiyorsa,
   tarayıcının istek atma imkânı da yoktur.
2. **Elle:** Task 17'deki kabul kontrolünde gerçek dinleyiciyle sayaç kontrolü.

Playwright ile tam otomasyon M2'ye bırakılmıştır. Bu, doğrulama dokümanının
"otomatik" iddiasını daraltıyor; sebebi araç sınırı, ihmal değil.

**Files:**
- Create: `frontend/src/lib/sanitize.ts`, `frontend/src/lib/sanitize.test.ts`
- Modify: `frontend/package.json`, `frontend/vitest.config.ts`

**Interfaces:**
- Consumes: yok
- Produces:
  - `sanitizeMessageHtml(raw: string, opts: SanitizeOptions): SanitizeResult`
  - `SanitizeOptions { allowRemote: boolean }`
  - `SanitizeResult { html: string; blockedRemoteCount: number }`

- [ ] **Step 1: Test altyapısını kur**

```bash
cd frontend
npm install --save dompurify
npm install --save-dev vitest jsdom @vitest/coverage-v8 @types/dompurify
```

`frontend/vitest.config.ts`:

```ts
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
  },
})
```

`frontend/package.json` içindeki `scripts` bloğuna ekle:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 2: Başarısız testleri yaz**

`frontend/src/lib/sanitize.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { sanitizeMessageHtml } from './sanitize'

const blocked = (raw: string) => sanitizeMessageHtml(raw, { allowRemote: false })

describe('script removal', () => {
  it('strips script tags', () => {
    const { html } = blocked('<p>hi</p><script>window.__xssFired = true</script>')
    expect(html).not.toContain('script')
    expect(html).not.toContain('__xssFired')
    expect(html).toContain('hi')
  })

  it('strips inline event handlers', () => {
    const { html } = blocked('<img src="cid:x" onerror="window.__xssFired = true">')
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('__xssFired')
  })

  it('strips javascript: URLs', () => {
    const { html } = blocked('<a href="javascript:alert(1)">click</a>')
    expect(html).not.toContain('javascript:')
  })

  it('removes embedded frames and objects', () => {
    const { html } = blocked('<iframe src="https://evil.example"></iframe><object data="https://evil.example"></object>')
    expect(html).not.toContain('iframe')
    expect(html).not.toContain('object')
  })
})

describe('remote resource blocking', () => {
  // Each of these is a separate tracker vector. Blocking only img/src is the
  // classic mistake: the other three still phone home.
  const vectors: Array<[string, string]> = [
    ['img src', '<img src="http://127.0.0.1:9999/tracker.png">'],
    ['img srcset', '<img srcset="http://127.0.0.1:9999/t1.png 1x, http://127.0.0.1:9999/t2.png 2x">'],
    ['body background attribute', '<div background="http://127.0.0.1:9999/attr.png">x</div>'],
    ['inline style url()', '<div style="background:url(\'http://127.0.0.1:9999/css.png\')">x</div>'],
    ['style element url()', '<style>.a{background-image:url("http://127.0.0.1:9999/sheet.png")}</style><div class="a">x</div>'],
    ['video poster', '<video poster="http://127.0.0.1:9999/poster.png"></video>'],
  ]

  for (const [name, raw] of vectors) {
    it(`blocks ${name}`, () => {
      const { html, blockedRemoteCount } = blocked(raw)
      expect(html).not.toContain('127.0.0.1:9999')
      expect(blockedRemoteCount).toBeGreaterThan(0)
    })
  }

  it('leaves no http or https URL anywhere in the output', () => {
    const raw = vectors.map(([, html]) => html).join('\n')
    const { html } = blocked(raw)
    expect(html).not.toMatch(/https?:\/\//)
  })

  it('keeps cid: references, which are inline parts rather than remote fetches', () => {
    const { html } = blocked('<img src="cid:logo@example.com">')
    expect(html).toContain('cid:logo@example.com')
  })

  it('preserves link hrefs so the reader can still see where a link goes', () => {
    const { html } = blocked('<a href="https://example.com/invoice">invoice</a>')
    // An href is not a fetch: nothing loads until the user clicks.
    expect(html).toContain('https://example.com/invoice')
  })

  it('restores remote images when the user opts in', () => {
    const { html, blockedRemoteCount } = sanitizeMessageHtml(
      '<img src="https://example.com/pic.png">',
      { allowRemote: true },
    )
    expect(html).toContain('https://example.com/pic.png')
    expect(blockedRemoteCount).toBe(0)
  })

  it('records the blocked URL so it can be restored later', () => {
    const { html } = blocked('<img src="https://example.com/pic.png">')
    expect(html).toContain('data-nexus-blocked-src')
  })
})

describe('form neutralisation', () => {
  it('removes form actions so a phishing form cannot submit', () => {
    const { html } = blocked('<form action="https://evil.example/steal"><input name="password"></form>')
    expect(html).not.toContain('evil.example')
  })
})

describe('robustness', () => {
  it('handles an empty document', () => {
    expect(blocked('').html).toBe('')
  })

  it('handles malformed markup without throwing', () => {
    expect(() => blocked('<div><p>unclosed <img src="http://x/y.png"')).not.toThrow()
  })
})
```

- [ ] **Step 3: Testleri çalıştır, başarısız olduklarını gör**

```bash
cd frontend && npm test
```

Beklenen: FAIL — `sanitize.ts` yok.

- [ ] **Step 4: Sanitizer'ı yaz**

`frontend/src/lib/sanitize.ts`:

```ts
import DOMPurify from 'dompurify'

export interface SanitizeOptions {
  /** When false, every remote resource reference is stripped. */
  allowRemote: boolean
}

export interface SanitizeResult {
  html: string
  /** How many remote references were removed, for the "load images" prompt. */
  blockedRemoteCount: number
}

/**
 * Attributes that cause a browser to fetch a resource on render. Blocking only
 * img/src is the classic mistake: `background`, `poster` and CSS `url()` all
 * still phone home, which is exactly what a tracking pixel needs.
 */
const FETCHING_ATTRS = ['src', 'srcset', 'poster', 'background', 'data-src', 'data-srcset']

const REMOTE_URL = /^(https?:)?\/\//i
const CSS_URL = /url\(\s*(['"]?)([^'")]+)\1\s*\)/gi

function isRemote(value: string): boolean {
  return REMOTE_URL.test(value.trim())
}

/** Replaces every remote url() in a CSS string, counting what it removed. */
function stripCssUrls(css: string): { css: string; blocked: number } {
  let blocked = 0
  const out = css.replace(CSS_URL, (match, _quote, url: string) => {
    if (isRemote(url)) {
      blocked++
      return 'url()'
    }
    return match
  })
  return { css: out, blocked }
}

/**
 * sanitizeMessageHtml prepares untrusted mail HTML for display.
 *
 * This is one of two layers, not the whole defence: the caller must still
 * render the result inside an <iframe sandbox> without allow-same-origin.
 * Sanitisation removes what we can recognise; the sandbox contains what we
 * cannot.
 */
export function sanitizeMessageHtml(raw: string, opts: SanitizeOptions): SanitizeResult {
  if (!raw) {
    return { html: '', blockedRemoteCount: 0 }
  }

  let blockedRemoteCount = 0

  // DOMPurify hooks run per node during sanitisation, which is the only place
  // we can see attributes before they reach the document.
  const hook = (node: Element) => {
    if (!(node instanceof Element)) return

    if (!opts.allowRemote) {
      for (const attr of FETCHING_ATTRS) {
        const value = node.getAttribute(attr)
        if (value && isRemote(value)) {
          node.removeAttribute(attr)
          // Keep the original so the UI can restore it on user consent.
          node.setAttribute(`data-nexus-blocked-${attr}`, value)
          blockedRemoteCount++
        }
      }

      const style = node.getAttribute('style')
      if (style) {
        const { css, blocked } = stripCssUrls(style)
        if (blocked > 0) {
          node.setAttribute('style', css)
          blockedRemoteCount += blocked
        }
      }

      if (node.tagName === 'STYLE' && node.textContent) {
        const { css, blocked } = stripCssUrls(node.textContent)
        if (blocked > 0) {
          node.textContent = css
          blockedRemoteCount += blocked
        }
      }
    }

    // A form that can submit is a phishing tool, and mail has no legitimate
    // need to post anywhere.
    if (node.tagName === 'FORM') {
      node.removeAttribute('action')
      node.removeAttribute('method')
    }
  }

  DOMPurify.addHook('afterSanitizeAttributes', hook)
  try {
    const html = DOMPurify.sanitize(raw, {
      FORBID_TAGS: ['script', 'iframe', 'object', 'embed', 'base', 'link', 'meta', 'applet'],
      FORBID_ATTR: ['srcdoc', 'formaction', 'ping'],
      // Mail is a document, not an app: no custom elements, no SVG scripting.
      USE_PROFILES: { html: true },
      ALLOW_DATA_ATTR: true,
      // href is kept so the reader can see where a link points; nothing loads
      // until they click, and clicks are handled by the host.
      ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|cid|tel|data):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
    })
    return { html, blockedRemoteCount }
  } finally {
    DOMPurify.removeHook('afterSanitizeAttributes')
  }
}
```

- [ ] **Step 5: Testleri çalıştır ve geçtiklerini doğrula**

```bash
cd frontend && npm test
```

Beklenen: tüm testler PASS. Özellikle "leaves no http or https URL anywhere"
testi geçmeli — bu, altı vektörün hepsinin kapandığının tek satırlık kanıtı.

`preserves link hrefs` testi başarısız olursa `ALLOWED_URI_REGEXP` fazla
kısıtlayıcıdır; `blocks img src` başarısız olursa hook `afterSanitizeAttributes`
yerine yanlış aşamaya bağlanmıştır.

- [ ] **Step 6: Commit**

```bash
cd .. && git add frontend/src/lib frontend/package.json frontend/package-lock.json frontend/vitest.config.ts
git commit -m "feat(frontend): sanitise mail HTML and block tracker vectors

Six separate fetching vectors are tested: img/src, img/srcset, the
background attribute, inline style url(), url() inside a style element,
and video/poster. Blocking only img/src is the classic mistake — the
others still phone home, which is all a tracking pixel needs.

One assertion covers them together: no http or https URL survives in the
output. If no fetching attribute is ever emitted, the browser has no
opportunity to make a request.

Blocked URLs are kept in data-nexus-blocked-* attributes so 'load remote
images' can restore them without refetching the message."
```

---

### Task 14: Uygulama durumu ve üç sütunlu iskelet

**Files:**
- Create: `frontend/src/store/useMailStore.ts`, `frontend/src/store/useMailStore.test.ts`
- Create: `frontend/src/lib/events.ts`
- Create: `frontend/src/components/Layout.tsx`, `frontend/src/components/FolderList.tsx`
- Create: `frontend/src/components/ThemeToggle.tsx`
- Modify: `frontend/src/App.tsx`, `frontend/src/index.css`

**Interfaces:**
- Consumes: Task 12'nin ürettiği binding'ler (`MailService.ListAccounts` vb.)
- Produces:
  - `useMailStore` — Zustand store: `accounts`, `folders`, `messages`, `selectedFolderId`, `selectedMessageId`, `syncState`
  - Eylemler: `loadAccounts()`, `selectFolder(id)`, `selectMessage(id)`, `loadMoreMessages()`, `applySyncEvent(name, payload)`
  - `EVENTS` sabitleri — Go tarafındaki `internal/app/events.go` ile eşleşir

- [ ] **Step 1: Olay adlarını iki tarafta eşleştir**

`frontend/src/lib/events.ts`:

```ts
/**
 * Event names must match internal/app/events.go exactly. They are duplicated
 * rather than generated because there are three of them; if this list grows,
 * generate it from the Go constants instead.
 */
export const EVENTS = {
  syncStarted: 'sync:started',
  syncFinished: 'sync:finished',
  syncFailed: 'sync:failed',
} as const

export type SyncEventPayload = {
  accountId: number
  email: string
  error?: string
  class?: 'transient' | 'auth' | 'protocol' | 'permanent'
}
```

- [ ] **Step 2: Store için başarısız test yaz**

`frontend/src/store/useMailStore.test.ts`:

```ts
import { beforeEach, describe, expect, it } from 'vitest'
import { EVENTS } from '../lib/events'
import { useMailStore } from './useMailStore'

describe('sync state', () => {
  beforeEach(() => {
    useMailStore.setState(useMailStore.getInitialState())
  })

  it('starts idle', () => {
    expect(useMailStore.getState().syncState).toEqual({})
  })

  it('marks an account as syncing then idle', () => {
    const { applySyncEvent } = useMailStore.getState()

    applySyncEvent(EVENTS.syncStarted, { accountId: 1, email: 'a@x' })
    expect(useMailStore.getState().syncState[1]).toEqual({ status: 'syncing' })

    applySyncEvent(EVENTS.syncFinished, { accountId: 1, email: 'a@x' })
    expect(useMailStore.getState().syncState[1]).toEqual({ status: 'idle' })
  })

  it('records an auth failure distinctly, because it needs a sign-in prompt rather than a retry', () => {
    useMailStore.getState().applySyncEvent(EVENTS.syncFailed, {
      accountId: 2, email: 'b@x', error: 'authentication failed', class: 'auth',
    })
    expect(useMailStore.getState().syncState[2]).toEqual({
      status: 'error', errorClass: 'auth', error: 'authentication failed',
    })
  })

  it('keeps per-account state separate', () => {
    const { applySyncEvent } = useMailStore.getState()
    applySyncEvent(EVENTS.syncStarted, { accountId: 1, email: 'a@x' })
    applySyncEvent(EVENTS.syncFailed, { accountId: 2, email: 'b@x', error: 'boom', class: 'transient' })

    const state = useMailStore.getState().syncState
    expect(state[1].status).toBe('syncing')
    expect(state[2].status).toBe('error')
  })
})

describe('selection', () => {
  beforeEach(() => {
    useMailStore.setState(useMailStore.getInitialState())
  })

  it('clears the selected message when the folder changes', () => {
    useMailStore.setState({ selectedFolderId: 1, selectedMessageId: 42 })
    useMailStore.getState().setSelectedFolder(2)

    const state = useMailStore.getState()
    expect(state.selectedFolderId).toBe(2)
    // Leaving this set would render a message from the previous folder.
    expect(state.selectedMessageId).toBeNull()
  })

  it('resets pagination when the folder changes', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: [{ id: 1 } as never], hasMore: false })
    useMailStore.getState().setSelectedFolder(2)

    const state = useMailStore.getState()
    expect(state.messages).toEqual([])
    expect(state.hasMore).toBe(true)
  })
})
```

- [ ] **Step 3: Testi çalıştır, başarısız olduğunu gör**

```bash
cd frontend && npm test -- useMailStore
```

Beklenen: FAIL — `useMailStore` yok.

- [ ] **Step 4: Store'u yaz**

```bash
npm install --save zustand
```

`frontend/src/store/useMailStore.ts`:

```ts
import { create } from 'zustand'
import { EVENTS, type SyncEventPayload } from '../lib/events'

export type SyncStatus =
  | { status: 'idle' }
  | { status: 'syncing' }
  | { status: 'error'; error: string; errorClass: SyncEventPayload['class'] }

export interface Account {
  id: number
  email: string
  displayName: string
  provider: string
  authKind: string
}

export interface Folder {
  id: number
  accountId: number
  name: string
  path: string
  totalCount: number
  unreadCount: number
  isInbox: boolean
}

export interface Message {
  id: number
  folderId: number
  uid: number
  threadId: string
  subject: string
  fromName: string
  fromAddr: string
  snippet: string
  internalDateUnix: number
  isRead: boolean
  isStarred: boolean
  hasAttachments: boolean
  bodyFetched: boolean
}

interface MailState {
  accounts: Account[]
  folders: Folder[]
  messages: Message[]
  selectedFolderId: number | null
  selectedMessageId: number | null
  hasMore: boolean
  syncState: Record<number, SyncStatus>

  setAccounts: (accounts: Account[]) => void
  setFolders: (folders: Folder[]) => void
  appendMessages: (messages: Message[], hasMore: boolean) => void
  setSelectedFolder: (id: number | null) => void
  setSelectedMessage: (id: number | null) => void
  applySyncEvent: (name: string, payload: SyncEventPayload) => void
}

const initialState = {
  accounts: [],
  folders: [],
  messages: [],
  selectedFolderId: null,
  selectedMessageId: null,
  hasMore: true,
  syncState: {},
} satisfies Partial<MailState>

export const useMailStore = create<MailState>((set) => ({
  ...initialState,

  setAccounts: (accounts) => set({ accounts }),
  setFolders: (folders) => set({ folders }),

  appendMessages: (messages, hasMore) =>
    set((state) => ({ messages: [...state.messages, ...messages], hasMore })),

  // Changing folder clears the message list, the selection and pagination
  // together. Keeping any of them would render or fetch content belonging to
  // the folder the user just left.
  setSelectedFolder: (id) =>
    set({ selectedFolderId: id, selectedMessageId: null, messages: [], hasMore: true }),

  setSelectedMessage: (id) => set({ selectedMessageId: id }),

  applySyncEvent: (name, payload) =>
    set((state) => {
      let next: SyncStatus
      switch (name) {
        case EVENTS.syncStarted:
          next = { status: 'syncing' }
          break
        case EVENTS.syncFinished:
          next = { status: 'idle' }
          break
        case EVENTS.syncFailed:
          next = {
            status: 'error',
            error: payload.error ?? 'unknown error',
            errorClass: payload.class,
          }
          break
        default:
          return state
      }
      return { syncState: { ...state.syncState, [payload.accountId]: next } }
    }),
}))
```

- [ ] **Step 5: Testlerin geçtiğini doğrula**

```bash
npm test -- useMailStore
```

Beklenen: altı test PASS. `getInitialState` bulunamazsa Zustand sürümün onu
desteklemiyor; testlerde `useMailStore.setState(initialState, true)` kullan ve
`initialState`'i store dosyasından dışa aktar.

- [ ] **Step 6: Üç sütunlu iskeleti yaz**

```bash
npm install --save react-resizable-panels
```

`frontend/src/components/Layout.tsx`:

```tsx
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import type { ReactNode } from 'react'

interface LayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
}

/**
 * Three resizable columns: folders, message list, message body. Panel sizes
 * are persisted by react-resizable-panels under autoSaveId, so the layout a
 * user sets up survives a restart.
 */
export function Layout({ sidebar, list, reader }: LayoutProps) {
  return (
    <PanelGroup direction="horizontal" autoSaveId="nexus-mail-columns" className="h-screen">
      <Panel defaultSize={18} minSize={12} maxSize={30}>
        <aside className="h-full overflow-y-auto border-r border-neutral-200 bg-neutral-50 dark:border-neutral-800 dark:bg-neutral-900">
          {sidebar}
        </aside>
      </Panel>

      <PanelResizeHandle className="w-px bg-neutral-200 transition-colors hover:bg-blue-500 dark:bg-neutral-800" />

      <Panel defaultSize={32} minSize={22}>
        <section className="h-full overflow-hidden border-r border-neutral-200 dark:border-neutral-800">
          {list}
        </section>
      </Panel>

      <PanelResizeHandle className="w-px bg-neutral-200 transition-colors hover:bg-blue-500 dark:bg-neutral-800" />

      <Panel defaultSize={50} minSize={30}>
        <main className="h-full overflow-hidden bg-white dark:bg-neutral-950">{reader}</main>
      </Panel>
    </PanelGroup>
  )
}
```

`frontend/src/components/FolderList.tsx`:

```tsx
import { useMailStore } from '../store/useMailStore'

export function FolderList() {
  const accounts = useMailStore((s) => s.accounts)
  const folders = useMailStore((s) => s.folders)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const setSelectedFolder = useMailStore((s) => s.setSelectedFolder)
  const syncState = useMailStore((s) => s.syncState)

  return (
    <nav className="p-2 text-sm">
      {accounts.map((account) => {
        const state = syncState[account.id]
        return (
          <div key={account.id} className="mb-4">
            <div className="flex items-center justify-between px-2 py-1">
              <span className="truncate font-medium text-neutral-700 dark:text-neutral-300">
                {account.email}
              </span>
              {state?.status === 'syncing' && (
                <span className="text-xs text-neutral-500" aria-label="Syncing">
                  &#8635;
                </span>
              )}
              {state?.status === 'error' && (
                <span
                  className="text-xs text-red-600"
                  title={state.error}
                  aria-label={state.errorClass === 'auth' ? 'Sign-in required' : 'Sync error'}
                >
                  {state.errorClass === 'auth' ? '&#128274;' : '!'}
                </span>
              )}
            </div>

            <ul>
              {folders
                .filter((folder) => folder.accountId === account.id)
                .map((folder) => (
                  <li key={folder.id}>
                    <button
                      type="button"
                      onClick={() => setSelectedFolder(folder.id)}
                      aria-current={folder.id === selectedFolderId}
                      className={`flex w-full items-center justify-between rounded px-2 py-1 text-left hover:bg-neutral-200 dark:hover:bg-neutral-800 ${
                        folder.id === selectedFolderId
                          ? 'bg-neutral-200 font-medium dark:bg-neutral-800'
                          : ''
                      }`}
                    >
                      <span className="truncate">{folder.name}</span>
                      {folder.unreadCount > 0 && (
                        <span className="ml-2 shrink-0 text-xs text-neutral-500">
                          {folder.unreadCount}
                        </span>
                      )}
                    </button>
                  </li>
                ))}
            </ul>
          </div>
        )
      })}
    </nav>
  )
}
```

- [ ] **Step 7: Temayı kur**

`frontend/src/components/ThemeToggle.tsx`:

```tsx
import { useEffect, useState } from 'react'

type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'nexus-mail-theme'

function apply(theme: Theme) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const dark = theme === 'dark' || (theme === 'system' && prefersDark)
  document.documentElement.classList.toggle('dark', dark)
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem(STORAGE_KEY) as Theme) ?? 'system',
  )

  useEffect(() => {
    apply(theme)
    localStorage.setItem(STORAGE_KEY, theme)

    if (theme !== 'system') return
    // Follow the OS while set to system, so the app changes with it rather
    // than only at startup.
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => apply('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  return (
    <select
      value={theme}
      onChange={(e) => setTheme(e.target.value as Theme)}
      aria-label="Theme"
      className="rounded border border-neutral-300 bg-transparent px-2 py-1 text-xs dark:border-neutral-700"
    >
      <option value="system">System</option>
      <option value="light">Light</option>
      <option value="dark">Dark</option>
    </select>
  )
}
```

`frontend/tailwind.config.js` içinde karanlık modu sınıf tabanlı yap:

```js
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: { extend: {} },
  plugins: [],
}
```

- [ ] **Step 8: Testleri ve derlemeyi çalıştır**

```bash
npm test
npm run build
```

Beklenen: testler PASS, TypeScript derlemesi hatasız.

- [ ] **Step 9: Commit**

```bash
cd .. && git add frontend/src frontend/tailwind.config.js frontend/package.json frontend/package-lock.json
git commit -m "feat(frontend): add app state and the three-column shell

Changing folder clears the message list, the selection and pagination in
one action. Tests assert this: leaving the selection set would render a
message belonging to the folder the user just left.

Sync state is per account and distinguishes an auth failure from a
transient one, because the two need different UI — a sign-in prompt
versus a silent retry.

Column widths persist via autoSaveId, and the theme follows the OS while
set to system rather than only sampling it at startup."
```

---

### Task 15: Sanallaştırılmış mesaj listesi

**Files:**
- Create: `frontend/src/components/MessageList.tsx`, `frontend/src/components/MessageList.test.tsx`
- Create: `frontend/src/test/setup.ts`
- Modify: `frontend/vitest.config.ts`

**Interfaces:**
- Consumes: `useMailStore` (Task 14)
- Produces: `<MessageList onLoadMore={() => void} />`

- [ ] **Step 1: jsdom ölçüm kurulumunu yaz**

jsdom her elemanın yüksekliğini 0 döndürür; sanallaştırma buna bakarak "hiçbir şey
görünmüyor" sonucuna varır ve test anlamsızlaşır. Bu yüzden ölçümleri test
kurulumunda sabitliyoruz.

```bash
cd frontend && npm install --save @tanstack/react-virtual
npm install --save-dev @testing-library/react @testing-library/dom
```

`frontend/src/test/setup.ts`:

```ts
/**
 * jsdom reports every element as having zero size, which makes a virtualiser
 * conclude that nothing is visible. Fixing the reported viewport to 600px lets
 * the row-count assertions mean something.
 */
const VIEWPORT_HEIGHT = 600

Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
  configurable: true,
  get() {
    return VIEWPORT_HEIGHT
  },
})

Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
  configurable: true,
  get() {
    return VIEWPORT_HEIGHT
  },
})

HTMLElement.prototype.getBoundingClientRect = function () {
  return {
    width: 400,
    height: VIEWPORT_HEIGHT,
    top: 0,
    left: 0,
    right: 400,
    bottom: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect
}

global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as never
```

`frontend/vitest.config.ts`'i güncelle:

```ts
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
  },
})
```

- [ ] **Step 2: Başarısız testi yaz**

`frontend/src/components/MessageList.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMailStore, type Message } from '../store/useMailStore'
import { MessageList } from './MessageList'

function makeMessages(count: number): Message[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    folderId: 1,
    uid: i + 1,
    threadId: `<t${i}@x>`,
    subject: `Subject ${i + 1}`,
    fromName: `Sender ${i + 1}`,
    fromAddr: `s${i + 1}@example.com`,
    snippet: 'preview text',
    internalDateUnix: 1700000000 + i,
    isRead: i % 2 === 0,
    isStarred: false,
    hasAttachments: false,
    bodyFetched: false,
  }))
}

describe('virtualisation', () => {
  beforeEach(() => {
    useMailStore.setState(useMailStore.getInitialState())
  })

  // The headline claim of the project is that a 50,000-message mailbox scrolls
  // smoothly. That only holds if the DOM stays small, so assert the DOM size
  // rather than trusting the library.
  it('renders a bounded number of rows for 50,000 messages', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(50_000) })

    const { container } = render(<MessageList onLoadMore={() => {}} />)

    const rows = container.querySelectorAll('[data-testid="message-row"]')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(100)

    // Total node count matters too: a bounded row count with a huge wrapper
    // tree would still be slow.
    expect(container.querySelectorAll('*').length).toBeLessThan(1000)
  })

  it('shows an empty state rather than a blank pane', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: [] })
    render(<MessageList onLoadMore={() => {}} />)
    expect(screen.getByText(/no messages/i)).toBeTruthy()
  })

  it('prompts the user to pick a folder when none is selected', () => {
    useMailStore.setState({ selectedFolderId: null, messages: [] })
    render(<MessageList onLoadMore={() => {}} />)
    expect(screen.getByText(/select a folder/i)).toBeTruthy()
  })

  it('marks unread rows so they are visually distinct', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(4) })
    const { container } = render(<MessageList onLoadMore={() => {}} />)

    const unread = container.querySelectorAll('[data-unread="true"]')
    expect(unread.length).toBeGreaterThan(0)
  })

  it('selects a message when its row is clicked', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(5) })
    const { container } = render(<MessageList onLoadMore={() => {}} />)

    const firstRow = container.querySelector('[data-testid="message-row"]') as HTMLElement
    firstRow.click()

    expect(useMailStore.getState().selectedMessageId).not.toBeNull()
  })

  it('asks for more messages when the end of the list is reached', () => {
    const onLoadMore = vi.fn()
    // A short list means the last row is rendered immediately, which is what
    // triggers the request.
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(3), hasMore: true })
    render(<MessageList onLoadMore={onLoadMore} />)
    expect(onLoadMore).toHaveBeenCalled()
  })

  it('does not ask for more when there is nothing left', () => {
    const onLoadMore = vi.fn()
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(3), hasMore: false })
    render(<MessageList onLoadMore={onLoadMore} />)
    expect(onLoadMore).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 3: Testi çalıştır, başarısız olduğunu gör**

```bash
npm test -- MessageList
```

Beklenen: FAIL — `MessageList` yok.

- [ ] **Step 4: Listeyi yaz**

`frontend/src/components/MessageList.tsx`:

```tsx
import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useRef } from 'react'
import { useMailStore } from '../store/useMailStore'

const ROW_HEIGHT = 72

interface MessageListProps {
  onLoadMore: () => void
}

function formatDate(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000)
  const now = new Date()
  const sameDay = date.toDateString() === now.toDateString()
  return sameDay
    ? date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
    : date.toLocaleDateString(undefined, { day: '2-digit', month: 'short' })
}

export function MessageList({ onLoadMore }: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const messages = useMailStore((s) => s.messages)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
  const setSelectedMessage = useMailStore((s) => s.setSelectedMessage)
  const hasMore = useMailStore((s) => s.hasMore)

  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    // A small overscan keeps scrolling smooth without inflating the DOM.
    overscan: 8,
  })

  const items = virtualizer.getVirtualItems()
  const lastRenderedIndex = items.length > 0 ? items[items.length - 1].index : -1

  useEffect(() => {
    if (!hasMore || messages.length === 0) return
    if (lastRenderedIndex >= messages.length - 1) {
      onLoadMore()
    }
  }, [hasMore, lastRenderedIndex, messages.length, onLoadMore])

  if (selectedFolderId === null) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        Select a folder to see its messages.
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        No messages in this folder.
      </div>
    )
  }

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {items.map((item) => {
          const message = messages[item.index]
          const selected = message.id === selectedMessageId
          return (
            <button
              key={message.id}
              type="button"
              data-testid="message-row"
              data-unread={!message.isRead}
              onClick={() => setSelectedMessage(message.id)}
              aria-current={selected}
              className={`absolute left-0 flex w-full flex-col gap-0.5 border-b border-neutral-100 px-3 py-2 text-left dark:border-neutral-800 ${
                selected
                  ? 'bg-blue-50 dark:bg-blue-950'
                  : 'hover:bg-neutral-50 dark:hover:bg-neutral-900'
              }`}
              style={{ top: item.start, height: item.size }}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span
                  className={`truncate text-sm ${
                    message.isRead
                      ? 'text-neutral-600 dark:text-neutral-400'
                      : 'font-semibold text-neutral-900 dark:text-neutral-100'
                  }`}
                >
                  {message.fromName || message.fromAddr}
                </span>
                <span className="shrink-0 text-xs text-neutral-400">
                  {formatDate(message.internalDateUnix)}
                </span>
              </div>
              <span
                className={`truncate text-sm ${
                  message.isRead ? 'text-neutral-600 dark:text-neutral-400' : 'font-medium'
                }`}
              >
                {message.subject || '(no subject)'}
              </span>
              <span className="truncate text-xs text-neutral-400">{message.snippet}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Testlerin geçtiğini doğrula**

```bash
npm test -- MessageList
```

Beklenen: yedi test PASS. "renders a bounded number of rows" testi 50.000
satırın hepsini sayarsa sanallaştırma bağlanmamıştır — `getScrollElement`'in
gerçekten `scrollRef.current` döndürdüğünü kontrol et.

- [ ] **Step 6: Commit**

```bash
cd .. && git add frontend/src frontend/vitest.config.ts frontend/package.json frontend/package-lock.json
git commit -m "feat(frontend): virtualise the message list

The project's headline claim is that a 50,000-message mailbox scrolls
smoothly, which only holds if the DOM stays small. The test asserts the
DOM size directly — under 100 rows and under 1000 total nodes — rather
than trusting the library to behave.

jsdom reports every element as zero-sized, which would make the
virtualiser conclude nothing is visible, so the test setup fixes the
reported viewport at 600px."
```

---

### Task 16: İzole mail görüntüleyici ve hesap ekleme akışı

**Files:**
- Create: `frontend/src/components/MessageView.tsx`, `frontend/src/components/MessageView.test.tsx`
- Create: `frontend/src/components/AddAccount.tsx`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `sanitizeMessageHtml` (Task 13), `useMailStore` (Task 14), `MailService` binding'leri (Task 12)
- Produces: `<MessageView />`, `<AddAccount onDone={() => void} />`

- [ ] **Step 1: Görüntüleyici için başarısız test yaz**

`frontend/src/components/MessageView.test.tsx`:

```tsx
import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMailStore } from '../store/useMailStore'
import { MessageView } from './MessageView'

vi.mock('../lib/api', () => ({
  getBody: vi.fn(async () => ({
    html: '<p>Hello</p><img src="http://127.0.0.1:9999/tracker.png"><script>window.__xssFired = true</script>',
    text: 'Hello',
  })),
}))

describe('isolation', () => {
  beforeEach(() => {
    useMailStore.setState(useMailStore.getInitialState())
    delete (window as never as Record<string, unknown>).__xssFired
  })

  it('renders the body inside a sandboxed iframe', async () => {
    useMailStore.setState({ selectedMessageId: 1 })
    const { container } = render(<MessageView />)

    const iframe = await waitFor(() => {
      const el = container.querySelector('iframe')
      expect(el).toBeTruthy()
      return el as HTMLIFrameElement
    })

    const sandbox = iframe.getAttribute('sandbox')
    expect(sandbox).not.toBeNull()

    // allow-same-origin would give mail scripts access to our origin, which
    // defeats the whole sandbox. Its absence is the single most important
    // assertion in the frontend.
    expect(sandbox).not.toContain('allow-same-origin')
    expect(sandbox).not.toContain('allow-scripts')
  })

  it('passes the body through srcdoc rather than a URL', async () => {
    useMailStore.setState({ selectedMessageId: 1 })
    const { container } = render(<MessageView />)

    const iframe = await waitFor(() => container.querySelector('iframe') as HTMLIFrameElement)
    expect(iframe.getAttribute('srcdoc')).toBeTruthy()
    expect(iframe.getAttribute('src')).toBeNull()
  })

  it('strips scripts and remote images from the rendered body', async () => {
    useMailStore.setState({ selectedMessageId: 1 })
    const { container } = render(<MessageView />)

    const iframe = await waitFor(() => container.querySelector('iframe') as HTMLIFrameElement)
    const srcdoc = iframe.getAttribute('srcdoc') ?? ''

    expect(srcdoc).toContain('Hello')
    expect(srcdoc).not.toContain('__xssFired')
    expect(srcdoc).not.toContain('127.0.0.1:9999')
    expect((window as never as Record<string, unknown>).__xssFired).toBeUndefined()
  })

  it('embeds a content security policy in the document it renders', async () => {
    useMailStore.setState({ selectedMessageId: 1 })
    const { container } = render(<MessageView />)

    const iframe = await waitFor(() => container.querySelector('iframe') as HTMLIFrameElement)
    const srcdoc = iframe.getAttribute('srcdoc') ?? ''
    expect(srcdoc).toContain('Content-Security-Policy')
  })

  it('offers to load remote content when something was blocked', async () => {
    useMailStore.setState({ selectedMessageId: 1 })
    const { container } = render(<MessageView />)

    await waitFor(() => {
      const button = container.querySelector('[data-testid="load-remote"]')
      expect(button).toBeTruthy()
    })
  })

  it('shows a placeholder when no message is selected', () => {
    useMailStore.setState({ selectedMessageId: null })
    const { container } = render(<MessageView />)
    expect(container.querySelector('iframe')).toBeNull()
    expect(container.textContent).toMatch(/select a message/i)
  })
})
```

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu gör**

```bash
cd frontend && npm test -- MessageView
```

Beklenen: FAIL — `MessageView` ve `../lib/api` yok.

- [ ] **Step 3: API sarmalayıcısını yaz**

Binding'leri doğrudan bileşenlerden çağırmıyoruz: tek bir sarmalayıcı hem testte
mock'lamayı kolaylaştırıyor hem de Wails binding yolu beta sürümde değişirse
düzeltilecek tek yer oluyor.

`frontend/src/lib/api.ts`:

```ts
import { MailService } from './bindings/nexusmail/internal/app'

export interface BodyDTO {
  html: string
  text: string
}

export const listAccounts = () => MailService.ListAccounts()
export const listFolders = (accountId: number) => MailService.ListFolders(accountId)
export const listMessages = (folderId: number, limit: number, offset: number) =>
  MailService.ListMessages(folderId, limit, offset)
export const getBody = (messageId: number): Promise<BodyDTO> => MailService.GetBody(messageId)
export const syncAccount = (accountId: number) => MailService.SyncAccount(accountId)
export const addPasswordAccount = (
  email: string,
  displayName: string,
  imapHost: string,
  imapPort: number,
  smtpHost: string,
  smtpPort: number,
  password: string,
) =>
  MailService.AddPasswordAccount(
    email, displayName, imapHost, imapPort, smtpHost, smtpPort, password,
  )
export const addOAuthAccount = (email: string, displayName: string, provider: string) =>
  MailService.AddOAuthAccount(email, displayName, provider)
```

Binding import yolu `wails3 generate bindings` çıktısına göre değişebilir;
Task 12 Step 9'da listelediğin dizine göre düzelt.

- [ ] **Step 4: Görüntüleyiciyi yaz**

`frontend/src/components/MessageView.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react'
import { getBody } from '../lib/api'
import { sanitizeMessageHtml } from '../lib/sanitize'
import { useMailStore } from '../store/useMailStore'

/**
 * The sandbox attribute is intentionally minimal. Adding allow-same-origin
 * would give mail scripts access to our origin — including localStorage and the
 * Wails bridge — which defeats the isolation entirely. allow-scripts is
 * likewise absent: mail has no legitimate need to run code.
 */
const SANDBOX = 'allow-popups allow-popups-to-escape-sandbox'

const CSP = [
  "default-src 'none'",
  "img-src data: cid:",
  "style-src 'unsafe-inline'",
  "font-src data:",
].join('; ')

function wrapDocument(bodyHtml: string): string {
  return [
    '<!doctype html>',
    '<html><head><meta charset="utf-8">',
    `<meta http-equiv="Content-Security-Policy" content="${CSP}">`,
    '<style>',
    'html,body{margin:0;padding:16px;font:14px/1.5 system-ui,sans-serif;color:#111;background:#fff}',
    'img{max-width:100%;height:auto}',
    'table{max-width:100%}',
    'a{color:#1a56db}',
    '@media (prefers-color-scheme: dark){html,body{color:#e5e5e5;background:#0a0a0a}a{color:#7aa2f7}}',
    '</style>',
    '</head><body>',
    bodyHtml,
    '</body></html>',
  ].join('')
}

export function MessageView() {
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
  const [raw, setRaw] = useState('')
  const [allowRemote, setAllowRemote] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    // Consent to load remote content is per message, never sticky: carrying it
    // over would silently load trackers in the next mail the user opens.
    setAllowRemote(false)
    setRaw('')
    setError('')

    if (selectedMessageId === null) return

    let cancelled = false
    getBody(selectedMessageId)
      .then((body) => {
        if (!cancelled) setRaw(body.html || `<pre>${body.text}</pre>`)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })

    return () => {
      cancelled = true
    }
  }, [selectedMessageId])

  const { html, blockedRemoteCount } = useMemo(
    () => sanitizeMessageHtml(raw, { allowRemote }),
    [raw, allowRemote],
  )

  if (selectedMessageId === null) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        Select a message to read it.
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6 text-sm text-red-600">
        Could not load this message: {error}
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      {blockedRemoteCount > 0 && !allowRemote && (
        <div className="flex items-center justify-between gap-3 border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs dark:border-amber-900 dark:bg-amber-950">
          <span className="text-amber-900 dark:text-amber-200">
            {blockedRemoteCount} remote {blockedRemoteCount === 1 ? 'resource' : 'resources'} blocked
            to stop the sender learning you opened this message.
          </span>
          <button
            type="button"
            data-testid="load-remote"
            onClick={() => setAllowRemote(true)}
            className="shrink-0 rounded border border-amber-400 px-2 py-1 font-medium text-amber-900 hover:bg-amber-100 dark:text-amber-200 dark:hover:bg-amber-900"
          >
            Load remote content
          </button>
        </div>
      )}

      <iframe
        title="Message body"
        sandbox={SANDBOX}
        srcDoc={wrapDocument(html)}
        className="h-full w-full flex-1 border-0"
      />
    </div>
  )
}
```

- [ ] **Step 5: Testlerin geçtiğini doğrula**

```bash
npm test -- MessageView
```

Beklenen: altı test PASS. `srcDoc` React prop'unun DOM'a `srcdoc` olarak indiğini
test doğruluyor.

- [ ] **Step 6: Hesap ekleme akışını yaz**

`frontend/src/components/AddAccount.tsx`:

```tsx
import { useState } from 'react'
import { addOAuthAccount, addPasswordAccount, syncAccount } from '../lib/api'

type Mode = 'oauth-microsoft' | 'oauth-google' | 'password'

interface AddAccountProps {
  onDone: () => void
}

export function AddAccount({ onDone }: AddAccountProps) {
  const [mode, setMode] = useState<Mode>('oauth-microsoft')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [imapHost, setImapHost] = useState('')
  const [imapPort, setImapPort] = useState(993)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      const account =
        mode === 'password'
          ? await addPasswordAccount(email, displayName, imapHost, imapPort, '', 0, password)
          : await addOAuthAccount(
              email,
              displayName,
              mode === 'oauth-google' ? 'google' : 'microsoft',
            )
      // Clear the password from component state as soon as it is stored.
      setPassword('')
      await syncAccount(account.id)
      onDone()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-md p-6">
      <h2 className="mb-4 text-lg font-semibold">Add an account</h2>

      <div className="mb-4 flex flex-col gap-2">
        {(
          [
            ['oauth-microsoft', 'Microsoft 365 or Outlook.com'],
            ['oauth-google', 'Gmail or Google Workspace'],
            ['password', 'Other IMAP server (password or app password)'],
          ] as Array<[Mode, string]>
        ).map(([value, label]) => (
          <label key={value} className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name="mode"
              value={value}
              checked={mode === value}
              onChange={() => setMode(value)}
            />
            {label}
          </label>
        ))}
      </div>

      <div className="flex flex-col gap-3">
        <input
          type="email"
          placeholder="you@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        />
        <input
          type="text"
          placeholder="Display name"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        />

        {mode === 'password' && (
          <>
            <input
              type="text"
              placeholder="IMAP host (leave blank for known providers)"
              value={imapHost}
              onChange={(e) => setImapHost(e.target.value)}
              className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            />
            <input
              type="number"
              value={imapPort}
              onChange={(e) => setImapPort(Number(e.target.value))}
              className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            />
            <input
              type="password"
              placeholder="Password or app password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="off"
              className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            />
          </>
        )}

        {mode !== 'password' && (
          <p className="text-xs text-neutral-500">
            Your browser will open so you can sign in. Nexus Mail never sees your password.
            {mode === 'oauth-google' &&
              ' Gmail also needs an OAuth client ID of your own — see the setup guide in the README.'}
          </p>
        )}

        {error && <p className="text-sm text-red-600">{error}</p>}

        <button
          type="button"
          onClick={submit}
          disabled={busy || !email}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          {busy ? 'Connecting…' : 'Add account'}
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 7: `App.tsx`'i bağla**

`frontend/src/App.tsx`:

```tsx
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import { AddAccount } from './components/AddAccount'
import { FolderList } from './components/FolderList'
import { Layout } from './components/Layout'
import { MessageList } from './components/MessageList'
import { MessageView } from './components/MessageView'
import { ThemeToggle } from './components/ThemeToggle'
import { EVENTS, type SyncEventPayload } from './lib/events'
import { listAccounts, listFolders, listMessages } from './lib/api'
import { useMailStore } from './store/useMailStore'

const PAGE_SIZE = 100

export default function App() {
  const [adding, setAdding] = useState(false)
  const accounts = useMailStore((s) => s.accounts)
  const setAccounts = useMailStore((s) => s.setAccounts)
  const setFolders = useMailStore((s) => s.setFolders)
  const appendMessages = useMailStore((s) => s.appendMessages)
  const applySyncEvent = useMailStore((s) => s.applySyncEvent)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const messageCount = useMailStore((s) => s.messages.length)

  const refreshAccounts = useCallback(async () => {
    const list = await listAccounts()
    setAccounts(list)
    const folders = (await Promise.all(list.map((a) => listFolders(a.id)))).flat()
    setFolders(folders)
  }, [setAccounts, setFolders])

  useEffect(() => {
    refreshAccounts().catch(() => {
      /* surfaced through sync events */
    })

    const offs = Object.values(EVENTS).map((name) =>
      Events.On(name, (event: { data: SyncEventPayload }) => {
        applySyncEvent(name, event.data)
        if (name === EVENTS.syncFinished) {
          refreshAccounts().catch(() => {})
        }
      }),
    )
    return () => offs.forEach((off) => off())
  }, [applySyncEvent, refreshAccounts])

  // Load the first page whenever the selected folder changes.
  useEffect(() => {
    if (selectedFolderId === null) return
    listMessages(selectedFolderId, PAGE_SIZE, 0)
      .then((page) => appendMessages(page, page.length === PAGE_SIZE))
      .catch(() => {})
  }, [selectedFolderId, appendMessages])

  const loadMore = useCallback(() => {
    if (selectedFolderId === null) return
    listMessages(selectedFolderId, PAGE_SIZE, messageCount)
      .then((page) => appendMessages(page, page.length === PAGE_SIZE))
      .catch(() => {})
  }, [selectedFolderId, messageCount, appendMessages])

  if (adding || accounts.length === 0) {
    return (
      <AddAccount
        onDone={() => {
          setAdding(false)
          refreshAccounts().catch(() => {})
        }}
      />
    )
  }

  return (
    <Layout
      sidebar={
        <div className="flex h-full flex-col">
          <div className="flex items-center justify-between gap-2 border-b border-neutral-200 p-2 dark:border-neutral-800">
            <button
              type="button"
              onClick={() => setAdding(true)}
              className="rounded px-2 py-1 text-xs hover:bg-neutral-200 dark:hover:bg-neutral-800"
            >
              + Account
            </button>
            <ThemeToggle />
          </div>
          <div className="flex-1 overflow-y-auto">
            <FolderList />
          </div>
        </div>
      }
      list={<MessageList onLoadMore={loadMore} />}
      reader={<MessageView />}
    />
  )
}
```

- [ ] **Step 8: Tüm frontend testlerini ve derlemeyi çalıştır**

```bash
npm test
npm run build
```

Beklenen: tüm testler PASS, derleme hatasız. `@wailsio/runtime` bulunamazsa
`wails3 init` tarafından eklenmemiş demektir; `npm install --save @wailsio/runtime`
çalıştır.

- [ ] **Step 9: Commit**

```bash
cd .. && git add frontend/src
git commit -m "feat(frontend): render mail in a sandboxed iframe, add the account flow

The sandbox attribute deliberately omits allow-same-origin: granting it
would give mail scripts access to our origin, including localStorage and
the Wails bridge, which defeats the isolation completely. A test asserts
its absence, and another asserts the body arrives via srcdoc rather than
a URL.

Consent to load remote content is per message and never sticky. Carrying
it across messages would silently load trackers in the next mail the
user opens."
```

---

### Task 17: README, kurulum rehberi ve M1 kabul doğrulaması

Kod iki yerde "README'deki kurulum rehberi"ne atıf yapıyor (Google ve Microsoft
client ID'leri). O rehber bu görevde yazılıyor — aksi halde kullanıcı hata
mesajında var olmayan bir dokümana yönlendirilir.

**Files:**
- Create: `README.md`
- Create: `docs/oauth-setup.md`
- Create: `scripts/tracker-probe.go`

**Interfaces:**
- Consumes: her şey
- Produces: çalışan, doğrulanmış M1

- [ ] **Step 1: OAuth kurulum rehberini yaz**

`docs/oauth-setup.md`:

```markdown
# OAuth setup

Nexus Mail is open source, so it ships without OAuth client credentials of its
own. You register your own client once, which takes a few minutes and keeps
your mailbox access under your own control.

Client IDs go in `config.json` inside the data directory:

- Windows: `%APPDATA%\nexus-mail\config.json`
- macOS: `~/Library/Application Support/nexus-mail/config.json`
- Linux: `$XDG_DATA_HOME/nexus-mail/config.json` (or `~/.local/share/nexus-mail/config.json`)

```json
{
  "googleClientId": "…apps.googleusercontent.com",
  "microsoftClientId": "…"
}
```

A client ID is an identifier, not a secret: a desktop app is a public OAuth
client and cannot keep a secret on the user's machine. Refresh tokens are the
sensitive part, and those go to your OS keyring, never to this file.

## Microsoft 365 and Outlook.com

1. Open the [Microsoft Entra admin center](https://entra.microsoft.com) →
   **App registrations** → **New registration**.
2. Name it anything. Under **Supported account types** choose
   *Accounts in any organizational directory and personal Microsoft accounts*,
   so both work and Outlook.com addresses can sign in.
3. Under **Redirect URI** choose **Public client/native**, and enter
   `http://localhost`. Nexus Mail listens on a random loopback port; Entra
   treats any port on `http://localhost` as a match for native clients.
4. Copy the **Application (client) ID** into `microsoftClientId`.
5. Under **API permissions** → **Add a permission** →
   **APIs my organization uses** → *Office 365 Exchange Online* →
   **Delegated permissions**, add `IMAP.AccessAsUser.All` and `SMTP.Send`.

Basic authentication for SMTP client submission was retired on 30 April 2026,
so OAuth is the only way in — this setup is required, not optional.

## Gmail and Google Workspace

1. Open the [Google Cloud console](https://console.cloud.google.com), create a
   project, and enable the **Gmail API**.
2. Configure the **OAuth consent screen**. Keep it in *Testing* mode and add
   your own address under **Test users**: a testing-mode app allows up to 100
   users, which is plenty for your own accounts and avoids Google's
   verification process entirely.
3. Create credentials → **OAuth client ID** → **Desktop app**.
4. Copy the client ID into `googleClientId`.

The `https://mail.google.com/` scope Nexus Mail needs is a *restricted* scope.
Publishing an app that requests it requires an annual CASA Tier 2 security
assessment. Because you use your own client in testing mode, none of that
applies to you.

## Other providers

Pick *Other IMAP server* when adding the account and use a password or an app
password. No registration needed.
```

- [ ] **Step 2: README'yi yaz**

`README.md`:

```markdown
# Nexus Mail

A local-first, privacy-focused desktop mail client. Mail is stored in a local
SQLite database, so reading and filtering never wait for the network, and the
app keeps working offline.

Built with Go and Wails v3. Runs on Windows, macOS and Linux.

## Status

M1 (read-only client) — connect an account, sync folders and headers, read mail
in an isolated renderer. Sending, search, rules and live sync are on the way.

## What makes it different

- **No network wait.** Everything renders from the local database.
- **Trackers blocked by default.** Remote images and CSS references are stripped
  until you ask for them, and then they are proxied so your IP does not leak.
- **Real isolation.** Message HTML renders inside a sandboxed iframe with no
  same-origin access, behind a content security policy.
- **Credentials stay in your OS keyring.** Never in the database, never in a
  config file.

## Building from source

Requirements: Go 1.26+, Node 20+, and the Wails v3 CLI.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9
git clone <this repo>
cd nexus-mail
wails3 build
```

On Linux you also need `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`.

Windows and Linux builds use `CGO_ENABLED=0`. macOS needs cgo, because the
Wails WebView binding requires it.

## Adding an account

Microsoft and Google accounts need an OAuth client ID of your own — a one-time,
few-minute setup described in [docs/oauth-setup.md](docs/oauth-setup.md). Other
IMAP servers work with a password or app password and need no setup.

## Contributing

CI enforces the architecture, not just the tests:

- Layer boundaries are checked by `depguard`. `store`, `imapx` and `auth` may
  not import upward, and `sync` may not import go-imap directly — it goes
  through the `MailBackend` interface so it stays testable against an in-memory
  server.
- Windows and Linux builds run with `CGO_ENABLED=0` as required checks. This
  doubles as the guard keeping our dependencies pure Go: add a cgo-dependent
  library and those builds break.
- `go test -race ./...` plus a lock-contention stress test that asserts SQLite
  never reports a busy database.

Design documents live in `docs/superpowers/specs/`.

## Repository settings for maintainers

Mark the `build (windows-latest)` and `build (ubuntu-latest)` jobs as required
status checks in branch protection. The cgo guard depends on them.

## Licence

MIT
```

- [ ] **Step 3: İzleyici sondasını yaz**

Sanitizer birim testleri çıktıda uzak URL kalmadığını kanıtlıyor, ama gerçek
tarayıcıda hiçbir isteğin çıkmadığını yalnızca gerçek bir dinleyici gösterir.

`scripts/tracker-probe.go`:

```go
//go:build ignore

// tracker-probe serves a counter endpoint for the manual privacy check.
// Run it, open the test message in Nexus Mail, then watch this output.
//
//	go run scripts/tracker-probe.go
//
// Any line printed here means a tracker vector got through.
package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

func main() {
	var hits atomic.Int64

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		fmt.Printf("HIT %d: %s %s (referer=%q, ua=%q)\n",
			n, r.Method, r.URL.Path, r.Referer(), r.UserAgent())
		w.WriteHeader(http.StatusNoContent)
	})

	fmt.Println("listening on http://127.0.0.1:9999 — no output means nothing leaked")
	log.Fatal(http.ListenAndServe("127.0.0.1:9999", nil))
}
```

Test maili olarak kullanılacak içerik (kendine mail at):

```html
<p>Tracker probe</p>
<script>window.__xssFired = true</script>
<img src="http://127.0.0.1:9999/img-src.png">
<img srcset="http://127.0.0.1:9999/srcset.png 1x">
<div background="http://127.0.0.1:9999/attr.png">x</div>
<div style="background:url('http://127.0.0.1:9999/inline-css.png')">x</div>
<style>.p{background-image:url("http://127.0.0.1:9999/style-el.png")}</style>
<div class="p">x</div>
<video poster="http://127.0.0.1:9999/poster.png"></video>
```

- [ ] **Step 4: Tüm otomatik kontrolleri çalıştır**

```bash
gofmt -l .
golangci-lint run ./...
CGO_ENABLED=0 go build ./...
go test -race ./...
cd frontend && npm test && npm run build && cd ..
go mod tidy && git diff --exit-code go.mod go.sum
```

Beklenen: hepsi temiz. Bu, M1'in otomatik kapısı.

- [ ] **Step 5: M1 kabul listesini elle doğrula**

Doğrulama dokümanı §5'teki M1 listesi. Her maddeyi işaretle:

- [ ] `wails3 build` Windows'ta `CGO_ENABLED=0` ile başarılı.
- [ ] Uygulama açılıyor ve hesap yoksa hesap ekleme ekranı çıkıyor.
- [ ] **Microsoft yolu:** OAuth ile giriş yapıldı, sistem tarayıcısı açıldı,
      onay sonrası uygulamaya dönüldü, klasörler ve mailler geldi.
- [ ] **Google yolu:** aynısı çalıştı (kendi client ID'nle, testing modunda).
- [ ] **Parola yolu:** genel bir IMAP hesabı app password ile bağlandı.
- [ ] Token OS anahtarlığında: Windows'ta `cmdkey /list | findstr nexus-mail`
      çıktısında görünüyor. macOS'ta Keychain Access'te, Linux'ta `secret-tool`
      ile ya da `secrets.enc` dosyasının varlığıyla.
- [ ] **Veritabanı sır içermiyor:**

```bash
strings "$APPDATA/nexus-mail/mail.db" | grep -iE 'refresh|bearer|password' || echo "temiz"
```

      Beklenen çıktı: `temiz`. Bir eşleşme çıkarsa M1 kabul edilmez.

- [ ] **Local-first kanıtı:** Uygulamayı kapat, ağ bağlantısını kes, aç.
      Klasörler ve mail listesi anında geliyor; daha önce açılmış bir mailin
      gövdesi de açılıyor.
- [ ] **Gizlilik kanıtı:** `go run scripts/tracker-probe.go` çalıştır, Step 3'teki
      test mailini aç.
      - Uyarı çubuğu "6 remote resources blocked" diyor.
      - Sonda çıktısı **boş** — hiçbir HIT satırı yok.
      - "Load remote content" tıklanınca resimler geliyor ve sonda HIT görüyor;
        `referer` alanı boş.
      - Alert penceresi açılmıyor.
- [ ] **Sanallaştırma kanıtı:** Büyük bir klasör aç, WebView geliştirici
      araçlarında `document.querySelectorAll('[data-testid="message-row"]').length`
      100'ün altında.
- [ ] Karanlık ve aydınlık tema geçişi çalışıyor; mail gövdesi de temayı izliyor.
- [ ] Pencere 900px'e kadar daraltıldığında üç sütun kullanılabilir kalıyor.

- [ ] **Step 6: RAM ölçümünü kaydet**

Katı bir CI eşiği yok (runner varyansı yüksek), ama referans değer kaydedilir.
Windows'ta süreç ağacının toplamı:

```powershell
Get-Process | Where-Object { $_.ProcessName -like '*nexus*' -or $_.ProcessName -like '*msedgewebview*' } | Measure-Object -Property WorkingSet64 -Sum | Select-Object -ExpandProperty Sum
```

Sonucu MB'a çevirip `docs/superpowers/specs/` altındaki doğrulama dokümanının
§4.2 tablosuyla karşılaştır (boşta ≤ 250 MB, aktif ≤ 400 MB) ve gerçek sayıyı
commit mesajına yaz. Hedef tutulmuyorsa bu bir hata değil — ölçümü ve sebebini
kaydet, bütçeyi gerçeğe göre güncelle.

- [ ] **Step 7: Commit ve M1 etiketi**

```bash
git add README.md docs/oauth-setup.md scripts/tracker-probe.go
git commit -m "docs: add README, OAuth setup guide and the privacy probe

Two error messages in the app point users at a setup guide, so the guide
had to exist. Both providers are covered, including why a client ID is
not a secret and why testing mode keeps Gmail's CASA assessment out of
scope for self-hosted users.

The tracker probe covers what unit tests cannot: the sanitiser tests
prove no remote URL survives in the output, but only a real listener
proves the browser makes no request."

git tag -a m1 -m "M1: read-only client with three auth paths"
```

---

## Plan Öz-Denetimi

Planı yazdıktan sonra spec'e karşı kontrol ettim.

### 1. Spec kapsamı

| Tasarım dokümanı bölümü | Karşılayan görev |
|---|---|
| §3 Katmanlar ve bağımlılık yönü | Task 1 (depguard), Task 4 (model katmanı) |
| §4.1 Ortak CredentialProvider | Task 6 |
| §4.2 Loopback + PKCE, rastgele port | Task 7 |
| §4.3 Üç sağlayıcı | Task 6 (parola, ön ayarlar), Task 7 (Google, Microsoft) |
| §4.4 SecretStore + Linux yedeği | Task 5 |
| §5.0 Diskteki yerleşim | Task 2 (`paths`) |
| §5 Şema + FTS5 | Task 2 (migration), Task 4 (repository'ler) |
| §5.1 Dört kritik alan | Task 2 (şema), Task 4 (`ResetFolder`, UNIQUE), Task 10 (UIDVALIDITY mantığı) |
| §6.2 İlk senkron + tembel klasörler | Task 10 |
| §7.1 iframe izolasyonu | Task 16 |
| §7.2 Tracker engelleme | Task 13, Task 17 (sonda) |
| §8 Üç sütun, sanallaştırma, tema, thread | Task 14, 15; thread anahtarı Task 9 |
| §9 Hata sınıflandırma | Task 10 (`Classify`), Task 12 (olaylara taşınması), Task 14 (UI'da ayrışması) |
| §10 Sahte sunucu testleri | Task 8, 9 (`imapmemserver`), Task 10 (`fakeBackend`) |
| Doğrulama §1.1 cgo politikası | Task 1 (CI matrisi) |
| Doğrulama §1.2 depguard | Task 1, Task 4 |
| Doğrulama §1.3 sürüm eşleşmesi | Task 1 (CI adımı) |
| Doğrulama §2.1 kilit çekişmesi | Task 3 |
| Doğrulama §2.2 UIDVALIDITY kabul testi | Task 10 |
| Doğrulama §4.1 DOM bütçesi | Task 15 |
| Doğrulama §4.2 RAM ölçümü | Task 17 |
| Doğrulama §5 M1 listesi | Task 17 |

**Bulunan boşluklar ve kapatılması:**

- **§6.3 delta senkron, §6.4 IDLE, §6.5 işlem kuyruğu** — Bunlar M2 ve M3
  kapsamında; bu plan bilinçli olarak M1 ile sınırlı. `operations` tablosu
  şemada hazır, `highest_modseq` alanı doldurulmaya başlanmış durumda.
- **Doğrulama §2.3 ve §2.4** (kuyruk testleri, CONDSTORE'suz sunucu) — sırasıyla
  M3 ve M2 planlarına ait.
- **Doğrulama §3.1'in tam otomasyonu** — Task 13'te açıkça daraltıldı ve
  gerekçesi yazıldı: jsdom uzak kaynak indirmediği için iddia orada anlamlı
  test edilemez. İki katmanlı çözüm kondu (birim testleri + Task 17'de gerçek
  dinleyici), tam otomasyon M2'ye bırakıldı.
- **§8 konuşma (thread) görünümü** — `ThreadKey` üretiliyor ve saklanıyor
  (Task 9), ama M1 arayüzü düz liste. Gruplanmış görünüm M2'ye ait; veri hazır
  olduğu için yeniden indeksleme gerekmeyecek.

### 2. Placeholder taraması

"TBD", "TODO", "implement later" veya "uygun hata yönetimi ekle" türü ifade
yok. Her kod adımında çalıştırılabilir kod var. İki yerde kasıtlı doğrulama
adımı var (Task 8 Step 1, Task 12 Step 1): beta kütüphanelerin imzalarını
belgeye güvenmek yerine `go doc` ile teyit etmek. Bunlar belirsizlik değil,
somut komutu ve beklenen çıktısı olan adımlar.

Task 4 Step 9'da bilinçli bir tuzak var: `joinAttrs` fonksiyonu kullanılmıyor ve
adım metni bunu söyleyip silinmesini istiyor. Bunu bıraktım çünkü linter'ın ölü
kodu yakalamasını görmek, kuralın çalıştığının kanıtı.

### 3. Tip tutarlılığı

Kontrol ettim ve **iki uyumsuzluk buldum, düzelttim:**

- `sync.Store` arayüzü `SetMessageBody` istiyordu ama Task 4'te üretilenler
  listesinde yoktu. Task 10 Step 6'da `internal/store/bodies.go` olarak
  eklendi, `GetMessageBody` ile birlikte.
- `EnsureBody` cache okumak için `BodyReader`'a tip iddiası yapıyor
  (`e.store.(BodyReader)`). `*store.Store` bunu karşılıyor çünkü
  `GetMessageBody` aynı dosyada tanımlı.

Geri kalan imzalar tutarlı: `model.Message.HasFlag` (Task 4) → `messageToDTO`
(Task 12); `imapx.UIDRange` (Task 8) → `FetchHeaders` (Task 9) → `fakeBackend`
(Task 10); `auth.CredentialProvider.Kind()` üç implementasyonda da aynı;
`app.Emitter` (Task 12) → `Config.Emit` → `recorder.emit` (test).

Olay adları iki dilde çoğaltılıyor (`internal/app/events.go` ve
`frontend/src/lib/events.ts`). Üç sabit için kod üretmek aşırı; ikinci dosyanın
yorumu bu bağı ve büyürse ne yapılacağını söylüyor.

---

# Revizyon 1 — 2026-09-09 tasarım güncellemesi

Tasarım dokümanına altı ekleme yapıldı (§5.2, §6.6, §6.7, §7.3, §8.1, §9.1). Bu
plana etkileri aşağıda. **Yürütmeye başlamadan önce bu bölüm okunmalıdır**: iki
görevin yaklaşımı değişiyor, ikisinin kodu düzeltiliyor, iki yeni görev ekleniyor.

## R1.1 — Task 2: FTS tetikleyicileri (kod düzeltmesi)

`001_init.sql` sonundaki FTS bloğu tamamen değişiyor. Eski hali contentless bir
tabloydu ve `body` kolonu içeriyordu; yeni hali harici içerikli, üç kolonlu ve
tetikleyicili:

```sql
CREATE VIRTUAL TABLE fts_messages USING fts5(
  subject,
  from_addr,
  snippet,
  content='messages',
  content_rowid='id'
);

CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages BEGIN
  INSERT INTO fts_messages(rowid, subject, from_addr, snippet)
  VALUES (new.id, new.subject, new.from_addr, new.snippet);
END;

CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet)
  VALUES ('delete', old.id, old.subject, old.from_addr, old.snippet);
END;

CREATE TRIGGER messages_fts_update AFTER UPDATE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, subject, from_addr, snippet)
  VALUES ('delete', old.id, old.subject, old.from_addr, old.snippet);
  INSERT INTO fts_messages(rowid, subject, from_addr, snippet)
  VALUES (new.id, new.subject, new.from_addr, new.snippet);
END;
```

Ayrıca `operations` tablosuna iki alan eklenir (gerekçe R1.3):

```sql
  folder_id       INTEGER REFERENCES folders(id) ON DELETE CASCADE,
  uid_validity    INTEGER NOT NULL DEFAULT 0,
```

Task 2 Step 6'daki tablo listesi testi `fts_messages` satırını korur; ek olarak üç
tetikleyicinin varlığı da kontrol edilir:

```go
for _, trigger := range []string{"messages_fts_insert", "messages_fts_delete", "messages_fts_update"} {
	var n int
	err := s.Read().QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?", trigger).Scan(&n)
	if err != nil {
		t.Fatalf("sqlite_master query for %s: %v", trigger, err)
	}
	if n != 1 {
		t.Errorf("trigger %s: found %d, want 1", trigger, n)
	}
}
```

## R1.2 — Task 4 ve Task 10: uygulama kodundan FTS yazmalarını kaldır

Bu, öz-denetimin kaçırdığı gerçek bir hatayı düzeltiyor. `UpsertMessages` içindeki
FTS bloğu şöyleydi:

```go
if _, err := ftsStmt.ExecContext(ctx, id, m.Subject, m.From.Addr, m.Snippet); err != nil {
	continue    // <-- hatayı yutuyor: sessiz indeks kaybı
}
```

Bu satır hem gereksiz (tetikleyici işi yapıyor) hem hatalı (hatayı yutuyor).
**Yapılacak değişiklikler:**

- `internal/store/messages.go`: `ftsStmt` hazırlığı, `LastInsertId()` çağrısı ve
  yukarıdaki blok **tamamen silinir**. `stmt.ExecContext` sonucu artık
  kullanılmadığı için `res, err :=` yerine `_, err =` yazılır.
- `internal/store/folders.go` → `ResetFolder`: ilk `ExecContext` (FTS `'delete'`
  toplu silmesi) **silinir**. `DELETE FROM messages` zaten satır satır silme
  tetikleyicisini çalıştırır.
- `internal/store/bodies.go` → `SetMessageBody`: son iki `ExecContext` (FTS
  sil-ve-ekle) **silinir**. Gövde indeksi P3'e ait; bu fonksiyon yalnızca
  `message_bodies` yazar ve `body_fetched` bayrağını çevirir.

Karşılığında Task 4'e bir test eklenir (`internal/store/fts_test.go`):

```go
package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func ftsMatchCount(t *testing.T, s *Store, query string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow(
		`SELECT count(*) FROM fts_messages WHERE fts_messages MATCH ?`, query).Scan(&n); err != nil {
		t.Fatalf("FTS match %q: %v", query, err)
	}
	return n
}

func TestTriggersKeepTheSearchIndexInStep(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()
	acct := seedAccount(t, s)
	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX"},
		{Path: "Other", Name: "Other"},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, _ := s.ListFolders(ctx, acct)
	var inboxID, otherID int64
	for _, f := range folders {
		if f.Path == "INBOX" {
			inboxID = f.ID
		} else {
			otherID = f.ID
		}
	}

	msg := model.Message{
		AccountID: acct, FolderID: inboxID, UID: 1,
		Subject: "Quarterly invoice", From: model.Address{Addr: "billing@example.com"},
		Snippet: "attached please find", InternalDate: time.Unix(1, 0),
	}
	if err := s.UpsertMessages(ctx, inboxID, []model.Message{msg}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "invoice"); got != 1 {
		t.Errorf("after insert, matches for 'invoice' = %d, want 1", got)
	}

	// An upsert fires the UPDATE trigger, not INSERT. If that distinction is
	// missed the index accumulates stale rows and search returns both.
	msg.Subject = "Quarterly receipt"
	if err := s.UpsertMessages(ctx, inboxID, []model.Message{msg}); err != nil {
		t.Fatalf("second UpsertMessages() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "receipt"); got != 1 {
		t.Errorf("after upsert, matches for 'receipt' = %d, want 1", got)
	}
	if got := ftsMatchCount(t, s, "invoice"); got != 0 {
		t.Errorf("the old subject still matches %d times; the index kept a stale row", got)
	}

	// A sibling folder's index entries must survive a reset.
	sibling := msg
	sibling.FolderID, sibling.UID, sibling.Subject = otherID, 2, "Sibling notice"
	if err := s.UpsertMessages(ctx, otherID, []model.Message{sibling}); err != nil {
		t.Fatalf("UpsertMessages(sibling) error: %v", err)
	}
	if err := s.ResetFolder(ctx, inboxID, 42); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "receipt"); got != 0 {
		t.Errorf("reset folder still has %d index entries, want 0", got)
	}
	if got := ftsMatchCount(t, s, "notice"); got != 1 {
		t.Errorf("sibling folder lost its index entry; the reset leaked")
	}

	// The only real proof that 'delete' commands carried the right old values.
	// A corrupted index answers queries wrongly but silently; only this shouts.
	if _, err := s.Write().Exec(
		`INSERT INTO fts_messages(fts_messages) VALUES('integrity-check')`); err != nil {
		t.Errorf("FTS integrity check failed: %v", err)
	}
}
```

Ve CI'ya bir negatif kontrol eklenir (`.github/workflows/ci.yml`, lint job'ına):

```yaml
      - name: Application code must not write to the FTS table
        run: |
          if grep -rn "INSERT INTO fts_messages" --include='*.go' internal/ | grep -v '_test.go'; then
            echo "fts_messages is maintained by triggers; remove the direct write"
            exit 1
          fi
```

## R1.3 — Yeni Task 18: işlem kuyruğuna UIDVALIDITY damgası (M3'e hazırlık)

Tasarım §6.6 bu planın kapsamına M1'de yalnızca **şema** olarak giriyor —
`operations.folder_id` ve `operations.uid_validity` alanları R1.1'de eklendi.
Damganın yazılması ve boşaltmada kontrol edilmesi M3 planına ait, çünkü kuyruk
worker'ı orada yazılıyor.

M1'de yapılacak tek şey, alanların var olduğunu ve niyetin kayıtlı olduğunu
doğrulayan bir test:

```go
func TestOperationsTableCarriesUIDValidityStamp(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	// M3's queue worker compares this stamp against the folder's current
	// UIDVALIDITY before applying an operation. Without it, a mailbox the
	// server recreated while we were offline would have us act on UIDs that
	// now refer to entirely different messages.
	for _, column := range []string{"folder_id", "uid_validity"} {
		var n int
		err := s.Read().QueryRow(
			`SELECT count(*) FROM pragma_table_info('operations') WHERE name = ?`,
			column).Scan(&n)
		if err != nil {
			t.Fatalf("pragma_table_info query: %v", err)
		}
		if n != 1 {
			t.Errorf("operations.%s is missing; M3 cannot guard against a UIDVALIDITY change without it", column)
		}
	}
}
```

## R1.4 — Task 13 yerine: Go tarafında temizleme (`internal/mailhtml`)

Tasarım §7.3 gövde teslimini IPC'den özel protokole taşıdı; bunun sonucu olarak
temizleme JavaScript'ten Go'ya geçiyor. **Task 13 bu haliyle geçersizdir.**

Yerine geçen görev:

- `internal/mailhtml/sanitize.go` — `bluemonday` politikası (saf Go, cgo yok).
  `Sanitize(raw string, allowRemote bool) (html string, blocked int)`.
- `internal/mailhtml/sanitize_test.go` — Task 13'teki **altı tracker vektörünün
  ve dört XSS testinin aynısı**, Go'ya çevrilmiş. Test niyeti birebir korunur;
  yalnızca dil değişir.
- **Kazanç:** Doğrulama dokümanı §3.1'in "jsdom uzak kaynak indirmiyor" kısıtı
  ortadan kalkıyor. Go tarafında `httptest` sunucusu ve gerçek isabet sayacı
  kullanılabildiği için tam otomasyon artık M2'ye ertelenmiyor — P0'da mümkün.
  Task 13'teki daraltma geri alınır.
- `frontend/src/lib/sanitize.ts` ve testleri **silinir**; DOMPurify bağımlılığı
  kaldırılır.

## R1.5 — Task 16 yerine: özel protokol handler'ı ve küçülen MessageView

- `internal/app/bodyhandler.go` — `http.Handler`: `wails://mail-body/<id>` ve
  `wails://mail-asset/<id>/<url_hash>` yollarını karşılar. Gerçek
  `Content-Security-Policy` başlığı gönderir. Doğrulama dokümanı §3.3'teki beş
  iddia bu handler'ın testleridir; Wails'e ihtiyaç duymadan `httptest` ile
  çalıştırılabilir.
- `main.go` — handler Wails'in asset options'ına özel şema olarak kaydedilir.
- `MessageView.tsx` — sanitizasyon yapmaz. `srcDoc` yerine
  `src={`wails://mail-body/${id}`}` kullanır; `sandbox` özniteliği ve
  `allow-same-origin`'in yokluğu **aynen korunur** (test de korunur).
  Engellenen kaynak sayısı artık gövdeyle birlikte bir başlıkta
  (`X-Nexus-Blocked-Count`) gelir ve uyarı çubuğunu besler.

## R1.6 — Yeni Task 19: loglama, log rotasyonu ve dışa aktarma

Tasarım §9.1. Yeni dosyalar:

- `internal/logging/logging.go` — `log/slog` JSON handler + `lumberjack`
  rotasyonu (10 MB, 3 yedek, 28 gün), hedef `<veri dizini>/logs/nexus.log`.
- `internal/logging/redact.go` — yasaklı alanlar için `slog.Handler` sarmalayıcı.
  Konu, snippet, gövde, ek adı, e-posta adresleri, token ve parolalar loga
  ulaşamaz.
- `internal/logging/redact_test.go` — doğrulama dokümanı §3.4'ün kanarya testi.
  Dört kanarya değeriyle senkron çalıştırılır, log çıktısında hiçbirinin
  bulunmadığı iddia edilir; aynı iddia `DEBUG` seviyesinde de tekrarlanır.
- `MailService.ExportLogs() (string, error)` — log dosyalarını zip'e toplar,
  yolunu döndürür. UI kaydetmeden önce içeriği gösterir.

Bu, gizlilik odaklı bir uygulamada ihmal edilemez: denetlenmemiş bir "logları
dışa aktar" düğmesi, kullanıcının mail içeriğini bir GitHub issue'suna taşıyan
sızıntı yoluna dönüşür.

## R1.7 — Task 17: yaşam döngüsü kuralı ve genişleyen kabul listesi

Tasarım §8.1: **pencere kapatıldığında uygulama tamamen sonlanır.** Tepsi ve
işletim sistemi bildirimleri M4'e alındı. `main.go`'da özel bir "tepsiye küçült"
davranışı **yazılmaz**; kapanışta bekleyen `operations` satırları diskte kalır,
yarım senkron `context` iptaliyle sonlanır, WAL checkpoint alınır.

Task 17'nin kabul listesi doğrulama dokümanı §5'teki güncel haliyle değiştirilir;
sekiz madde eklendi (FTS sayısı ve bütünlük kontrolü, `wails://` üzerinden gövde
yükleme, 5 MB e-bültende donma olmaması, süreç gerçekten sonlanıyor mu, log
dosyası ve gizliliği, dışa aktarma düğmesi).

## R1.8 — Kapsam dışı kalan: saklama penceresi

Tasarım §6.7'deki saklama penceresi (12 ay / klasör başına 25.000 mesaj, yıldızlı
mailler muaf) **M1'de uygulanmaz.** Gerekçe: sınırsız büyüme ancak IDLE aylarca
çalıştığında ortaya çıkar, M1'de böyle bir yol yok. Politika tasarımda kayıtlı;
temizlik işi M2 planına ait.

## Revizyon sonrası görev sayısı

M1: **19 görev.** Task 13 ve 16 yeniden yazılıyor (yaklaşım değişti), Task 2, 4
ve 10 kod düzeltmesi alıyor, Task 18 ve 19 ekleniyor.

---

# Revizyon 2 — 2026-09-09, beş dayanıklılık maddesi

Tasarım dokümanına §4.2 (istemci türü ve yedek port), §4.2.1 (kilitli
anahtarlık), §6.2 (tarih ayrıştırma), §7.3 (handler iptali) ve §10.1
(`synctest`) eklendi. Plan etkileri:

## R2.1 — Task 5: `SecretStore` zaman aşımı ve kilit tespiti

`SecretStore` çağrıları bloke olabilir; kilitli bir anahtarlık işletim
sistemine parola penceresi açtırır ve o pencere açık kaldığı sürece çağrı geri
dönmez. Arka planda token yenileyen bir goroutine böylece süresiz askıda kalır.

`internal/auth/secretstore.go`'ya eklenir:

```go
// ErrKeyringLocked reports that the OS credential store did not answer in
// time, which almost always means it is locked and waiting for the user.
var ErrKeyringLocked = errors.New("auth: OS keyring is locked or not responding")

// keyringTimeout bounds every keyring call. A locked Secret Service or Keychain
// blocks until the user answers an OS prompt, and a background token refresh
// must never hang on that.
const keyringTimeout = 10 * time.Second
```

`internal/auth/timeout.go`:

```go
package auth

import (
	"sync/atomic"
	"time"
)

// pendingUnlock ensures at most one blocked keyring call is outstanding.
//
// A D-Bus or Keychain call cannot be cancelled from Go: on timeout the
// goroutine making it stays alive until the OS prompt is dismissed. That leak
// is acceptable once, but not once per sync cycle — so while one call is stuck,
// further calls fail fast instead of stacking up goroutines.
var pendingUnlock atomic.Bool

// withTimeout runs fn on its own goroutine and gives up after keyringTimeout.
func withTimeout[T any](fn func() (T, error)) (T, error) {
	var zero T

	if pendingUnlock.Load() {
		return zero, ErrKeyringLocked
	}

	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)

	pendingUnlock.Store(true)
	go func() {
		v, err := fn()
		done <- result{v, err}
		pendingUnlock.Store(false)
	}()

	select {
	case r := <-done:
		return r.value, r.err
	case <-time.After(keyringTimeout):
		// pendingUnlock stays true until the abandoned goroutine finishes.
		return zero, ErrKeyringLocked
	}
}
```

`keyring_store.go` içindeki üç metot ve `NewKeyringStore`'daki yoklama bu
sarmalayıcıdan geçirilir — yoklamanın kendisi de bloke olabilir, o yüzden en
kritik yer orası.

Test (`internal/auth/timeout_test.go`), zamanı gerçekten beklemeden:

```go
package auth

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestWithTimeoutGivesUpOnABlockedKeyring(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		t.Cleanup(func() { close(blocked) })

		_, err := withTimeout(func() (string, error) {
			<-blocked // never returns during the test
			return "", nil
		})
		if !errors.Is(err, ErrKeyringLocked) {
			t.Errorf("withTimeout() error = %v, want ErrKeyringLocked", err)
		}
	})
}

func TestWithTimeoutFailsFastWhileAnotherCallIsStuck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		t.Cleanup(func() { close(blocked) })

		go withTimeout(func() (string, error) {
			<-blocked
			return "", nil
		})
		// Let the first call register itself and then time out.
		time.Sleep(keyringTimeout + time.Second)

		start := time.Now()
		if _, err := withTimeout(func() (string, error) { return "x", nil }); !errors.Is(err, ErrKeyringLocked) {
			t.Errorf("second call error = %v, want ErrKeyringLocked", err)
		}
		// The point of failing fast: no second ten-second wait, and no second
		// leaked goroutine per sync cycle.
		if elapsed := time.Since(start); elapsed >= keyringTimeout {
			t.Errorf("second call waited %v; it should fail immediately", elapsed)
		}
	})
}

func TestWithTimeoutPassesThroughAFastCall(t *testing.T) {
	got, err := withTimeout(func() (string, error) { return "value", nil })
	if err != nil {
		t.Fatalf("withTimeout() error: %v", err)
	}
	if got != "value" {
		t.Errorf("withTimeout() = %q, want value", got)
	}
}
```

`app` katmanında `ErrKeyringLocked` ayrı bir hata sınıfı olarak UI'a taşınır ve
"işletim sistemi anahtarlığınız kilitli, lütfen kilidini açın" mesajı gösterilir.

## R2.2 — Task 9: tarih ayrıştırma yedeği

`internal/imapx/client.go` içindeki `FetchHeaders` döngüsünde envelope
atamasından sonra çalışır:

```go
m.Date = reconcileDate(env.Date, buf.InternalDate)
```

`internal/imapx/envelope.go`'ya eklenir:

```go
// maxDateSkew is how far ahead of INTERNALDATE a Date: header may legitimately
// sit. Real mail drifts by hours through timezone mistakes; more than two days
// ahead is either broken or deliberate, and in both cases the server's delivery
// time is the better answer.
const maxDateSkew = 48 * time.Hour

// reconcileDate picks a trustworthy timestamp. The RFC 2822 Date: header is
// attacker-controlled and frequently malformed — spam sends things like
// "Pzt, 99 Xyz 2026 29:99:99" — so INTERNALDATE wins whenever the header is
// unusable or implausible.
func reconcileDate(headerDate, internalDate time.Time) time.Time {
	if internalDate.IsZero() {
		// Nothing better to fall back to.
		return headerDate
	}
	if headerDate.IsZero() {
		return internalDate
	}
	if headerDate.After(internalDate.Add(maxDateSkew)) {
		return internalDate
	}
	return headerDate
}
```

Test (`internal/imapx/envelope_test.go`'ya eklenir):

```go
func TestReconcileDatePrefersInternalDateWhenTheHeaderIsUnusable(t *testing.T) {
	internal := time.Unix(1700000000, 0)

	cases := []struct {
		name   string
		header time.Time
		want   time.Time
	}{
		{"unparseable header leaves a zero time", time.Time{}, internal},
		{"absurd future date", internal.Add(365 * 24 * time.Hour), internal},
		{"just past the skew limit", internal.Add(49 * time.Hour), internal},
		{"plausible timezone drift is kept", internal.Add(6 * time.Hour), internal.Add(6 * time.Hour)},
		{"a date in the past is kept", internal.Add(-72 * time.Hour), internal.Add(-72 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcileDate(tc.header, internal); !got.Equal(tc.want) {
				t.Errorf("reconcileDate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconcileDateKeepsTheHeaderWhenThereIsNoInternalDate(t *testing.T) {
	header := time.Unix(1700000000, 0)
	if got := reconcileDate(header, time.Time{}); !got.Equal(header) {
		t.Errorf("reconcileDate() = %v, want the header date %v", got, header)
	}
}
```

## R2.3 — R1.5 (özel protokol handler'ı): iptal, önbellek, debounce

Tasarım §7.3'e eklenen üç kural handler görevinin kapsamına girer:

- `bodyhandler.go` her adımda `req.Context()`'i kontrol eder; `<-ctx.Done()`
  tetiklendiğinde veritabanı sorgusu ve temizleme derhal kesilir. Sorgular
  `QueryRowContext` kullanır.
- Temizlenmiş HTML mesaj kimliğine göre LRU önbelleğe alınır (~32 giriş).
- `MessageView.tsx` seçim değişimini ~120 ms debounce eder.

Handler testine iki iddia eklenir:

```go
func TestBodyHandlerStopsWorkWhenTheRequestIsCancelled(t *testing.T) {
	// Assert: with a cancelled context the handler returns without reading the
	// body from the database. Verified by pointing it at a store whose read
	// would fail the test if reached.
}

func TestBodyHandlerServesTheSecondRequestFromCache(t *testing.T) {
	// Assert: sanitising runs once for two requests for the same message.
	// Holding j or k changes the selection dozens of times a second, and every
	// change would otherwise re-sanitise the same HTML.
}
```

## R2.4 — Task 1: OAuth istemci türü uyarısı ve yedek port

`docs/oauth-setup.md` (Task 17) Google bölümüne **kalın bir uyarı** girer:

> Client type **must** be *Desktop app*. If you pick *Web application*, Google
> only accepts redirect URIs you registered in full, port included, and the
> random loopback port is rejected with `400 redirect_uri_mismatch`. Desktop
> app clients accept any loopback port.

`internal/app/config.go` içindeki `fileConfig`'e alan eklenir:

```go
	// OAuthRedirectPort forces a fixed loopback port. Zero, the default, picks
	// a random one — which is correct for both Google desktop clients and Entra
	// native clients. Set it only when security software blocks binding
	// arbitrary ports, and register the same port with the provider.
	OAuthRedirectPort int `json:"oauthRedirectPort"`
```

`auth.OAuthConfig`'e karşılık gelen `RedirectPort int` alanı eklenir;
`RunLoopbackFlow` dinleyiciyi `127.0.0.1:0` yerine bu portla açar (sıfırdan
farklıysa). Test:

```go
func TestRunLoopbackFlowHonoursAFixedPort(t *testing.T) {
	// Assert: with RedirectPort set, redirect_uri carries exactly that port.
	// Assert: with it zero, the port is neither 0 nor 3000.
}
```

## R2.5 — Task 3 ve Task 10: zaman bağımlı testlerde `synctest`

Kural (tasarım §10.1): üretim kodu `time` paketini normal kullanır; zaman
bağımlı testler `testing/synctest` bloğu içinde çalışır. `clockwork` gibi bir
saat enjeksiyon kütüphanesi eklenmeyecek — üretim kodunu test uğruna deforme
ediyor.

M1'de bu kural R2.1'deki zaman aşımı testlerinde uygulanır. Asıl faydası M2'de
ortaya çıkacak: 29 dakikalık IDLE yenilemesi ve 2 saniyeden 5 dakikaya üstel
geri çekilme, gerçek süre beklemeden test edilecek. `.golangci.yml`'ye bir
yasak eklenir, kuralın sonradan sessizce ihlal edilmemesi için:

```yaml
        no-clock-libraries:
          files: ["**/internal/**"]
          deny:
            - pkg: "github.com/jonboulle/clockwork"
              desc: "use testing/synctest; injected clocks deform production code"
            - pkg: "github.com/benbjohnson/clock"
              desc: "use testing/synctest instead"
```

## Revizyon 2 sonrası durum

Görev sayısı **19** olarak kalıyor; beş madde mevcut görevlerin içine giriyor.
Etkilenen görevler: 1 (config alanı, depguard kuralı), 3 (synctest kuralı),
5 (zaman aşımı — en büyük ekleme), 7 (sabit port), 9 (tarih yedeği), 17
(kurulum rehberi uyarısı), R1.5 (handler iptali ve önbellek).


---

# Yürütme Notları — Task 1 tamamlandı (2026-09-09)

Yürütme sırasında planın dört varsayımı yanlış çıktı. Sonraki görevler bu düzeltilmiş
gerçeklere göre okunmalıdır.

| Plan ne diyordu | Gerçek | Etkilenen görev |
|---|---|---|
| Şablon adı `react-ts` | `react` (React + TypeScript + Vite) | Task 1 (tamam) |
| Binding'ler `frontend/src/lib/bindings/` altında | `frontend/bindings/nexusmail/` altında, `.js` + `.d.ts` | **Task 12 Step 9, Task 16 Step 3** |
| Şablon doğrudan derlenir | `build/ios` ve `build/android` `package main`, ama `main` fonksiyonu yalnızca kendi build tag'leri altında — `go build ./...` kırılıyor. Masaüstü hedefli olduğumuz için ikisi de Taskfile include'larıyla birlikte silindi. | Task 1 (tamam) |
| Go testleri tek başına çalışır | `main.go` `frontend/dist`'i embed ediyor; frontend bir kez derlenmeden **hiçbir** Go paketi derlenmiyor. Yerel geliştirmede ve CI'da sıra: `wails3 generate bindings` → `npm ci && npm run build` → Go. | Tüm Go görevleri |

`internal/app/service.go` içindeki api sarmalayıcısı ve `frontend/src/lib/api.ts`
import satırı şu olacaktır:

```ts
import { MailService } from '../../bindings/nexusmail'
```

Doğrulanan araç sürümleri: Go 1.26.3, Node 25.9.0, npm 11.17.0,
wails3 v3.0.0-beta.9 (kütüphaneyle eşleşiyor), golangci-lint 2.13.2.

Depguard kuralları iki kasıtlı ihlalle test edildi ve ikisi de yakalandı:
`store → sync` ve `mailhtml → net/http`. `sync → go-imap` kuralı go-imap
bağımlılık ağacına girmediği için henüz test edilemedi; **Task 8'de test
edilecek.**

## Yürütme Notları — Task 2 ve 3 (2026-09-09)

**Planda çelişki vardı: `CGO_ENABLED=0 go test -race` imkânsız.** `-race` cgo
gerektiriyor. Bu bir çelişki değil, iki ayrı endişenin karıştırılmasıydı:

- **cgo yasağı** dağıtılan binary'nin neye bağlandığıyla ilgilidir → build
  matrisi doğrular (`CGO_ENABLED=0 go build`).
- **`-race`** bir test aracıdır → test job'ında cgo açık çalışır.

CI artık testleri **iki kez** çalıştırıyor: `-race` ile (cgo açık) ve
`CGO_ENABLED=0` ile (saf Go yolu). İkincisi olmasa, `modernc.org/sqlite`'ın saf
Go yolundaki bir gerileme yalnızca derleme anında görünür, testlerde görünmezdi.

**Bu makinede `-race` çalıştırılamıyor:** gcc kurulu değil. Yerel doğrulama
`-race` olmadan yapılır; race detector yalnızca CI'da çalışır. Planın her
görevin sonundaki "`go test -race` yeşil olmadan commit yapılmaz" kuralı bu
makinede "`go test` yeşil olmadan" şeklinde okunur.

**Go 1.25'in `WaitGroup.Go` metodu kullanıldı** — `wg.Add(1)` + `go func(){
defer wg.Done() }` üçlüsü yerine. Kod kısalıyor ve `Add`/`Done` dengesizliği
ihtimali ortadan kalkıyor.

**Doğrulanan kritik varsayım:** FTS5, `modernc.org/sqlite` altında
`CGO_ENABLED=0` ile çalışıyor. Sanal tablo ve üç tetikleyici oluşuyor. Ayrıca
eşzamanlılık testi tetikleyicinin yük altında geride kalmadığını da doğruluyor:
50 okuyucu ve 3 saniyede 6968 yazma, sıfır kilit çekişmesi, FTS satır sayısı
`messages` ile birebir aynı.

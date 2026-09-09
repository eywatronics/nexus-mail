# Nexus Mail — P0 Tasarım Dokümanı

- **Tarih:** 2026-09-09
- **Durum:** Onaylandı
- **Kapsam:** P0 (Çekirdek + hesap bağlama + çift yönlü senkron)

---

## 1. Ürün Tanımı

Nexus Mail; yerel-önce (local-first), gizlilik odaklı, çapraz platform bir masaüstü
e-posta istemcisidir. Veri önce yerel SQLite veritabanına iner; okuma, filtreleme ve
(P3'ten itibaren) arama ağ beklemeden çalışır. İnternet yokken uygulama tam
işlevseldir; yapılan değişiklikler kuyruğa alınır ve bağlantı geldiğinde uygulanır.

**Dağıtım modeli:** Açık kaynak. Windows, macOS ve Linux.

---

## 2. Onaylanmış Kararlar

| Karar | Seçim | Gerekçe |
|---|---|---|
| Dağıtım | Açık kaynak | Gmail CASA Tier 2 denetim yükünü (yıllık ~540-1000 USD, 4-12 hafta) projeden kaldırır; kullanıcı kendi OAuth client ID'sini sağlar. |
| Platform | Windows + macOS + Linux | Kullanıcı gereksinimi. cgo'yu fiilen yasaklar. |
| Masaüstü çatı | Wails v3-beta, sürüm sabitlenmiş | v2 tek pencere mimarisi; e-posta istemcisi ayrı compose/okuma pencereleri ve sistem tepsisi gerektirir. v3'ün masaüstü API'si stabil ilan edildi. |
| cgo | Bağımlılıklarımızda kullanılmayacak | Katkıcı kurulum eşiğini düşük tutmak; C derleme zinciri gerektirmemek. **Not:** macOS'ta Wails'in WebView bağlaması cgo gerektirir, bu kaçınılamaz. Kural bizim bağımlılık ağacımız için geçerlidir; Windows/Linux `CGO_ENABLED=0` ile derlenerek zorlanır. |
| Veritabanı | modernc.org/sqlite (saf Go) + FTS5 | cgo yasağının doğal sonucu. FTS5 mevcut. |
| IMAP | emersion/go-imap v2 (beta) | IMAP4rev2, CONDSTORE/QRESYNC, API'si dondurulmuş. v1 eski nesil. |
| Şifreleme | Yalnızca kimlik bilgileri (OS anahtarlığı) | SQLCipher cgo gerektirir → çapraz platform ve saf Go hedefiyle çelişir. Thunderbird/Apple Mail de bu modeli kullanır. |
| Senkron mimarisi | Yerel-önce + giden işlem kuyruğu | Çevrimdışı çalışma ayrı bir özellik değil, mimarinin doğal sonucu olur. |
| JMAP | Uygulanmayacak | 2026'da Gmail/Outlook/Yahoo desteklemiyor; yalnızca Fastmail/Cyrus/Stalwart. Gelecekte eklenebilmesi için `sync` motoru `imapx`'e doğrudan değil, `MailBackend` arayüzü üzerinden bağlanır. |

### Reddedilen alternatifler

- **Write-through senkron:** Her UI eylemi ağ gecikmesi kadar bekler, çevrimdışıyken
  uygulama salt-okunur olur. "Sıfır gecikme" vaadiyle çelişir; Outbox'ı sonradan ayrı
  bir mekanizma olarak yeniden icat etmeyi gerektirir.
- **CRDT / tam çift yönlü merge:** IMAP sunucuyu zaten tek gerçek kaynak olarak
  tanımlar. Çözdüğü problem bu projede yok.
- **Tauri:** Go IMAP ekosistemi (go-imap, go-message, go-smtp) bu projenin en değerli
  bağımlılık yığını; Rust'a geçmek onu kaybettirir.
- **Shadow DOM ile mail izolasyonu:** CSS'i izole eder, JavaScript'i etmez. Güvenlik
  sınırı değildir.

---

## 3. Mimari

### 3.1 Modüller ve bağımlılık yönü

```
frontend/          React 19 + TS + Tailwind + shadcn/ui
      |
internal/app       Wails servisleri: frontend'e açılan tek yüzey, olay yayını
      |
internal/sync      SyncEngine: ilk senkron, delta, IDLE, işlem kuyruğu boşaltma
      |
      +--> internal/imapx    go-imap v2 sarmalayıcı: dial, SASL, capability, backoff
      +--> internal/store    SQLite: şema, migration, repository'ler
      +--> internal/auth     CredentialProvider + SecretStore
```

Bağımlılık tek yönlüdür: hiçbir alt katman üstündekini bilmez. `sync` motoru
`imapx`'i somut tip olarak değil, `MailBackend` arayüzü üzerinden kullanır. Bunun iki
faydası var: motor gerçek ağ olmadan test edilebilir, ve ileride JMAP gibi ikinci bir
protokol aynı arayüzü uygulayarak eklenebilir.

### 3.2 Modül sorumlulukları

| Modül | Ne yapar | Neye bağımlı |
|---|---|---|
| `internal/store` | Şema, migration, repository'ler. Tek yazıcı goroutine + okuma havuzu. | — |
| `internal/auth` | `CredentialProvider` (3 impl: msoauth, googleoauth, password), `SecretStore`. | — |
| `internal/imapx` | Bağlantı kurma, SASL, capability tespiti, yeniden bağlanma. | auth |
| `internal/sync` | Senkron durum makinesi, IDLE, delta, kuyruk boşaltma. | imapx, store |
| `internal/app` | Wails servis metotları, olay yayını, hesap yaşam döngüsü. | sync, store, auth |

---

## 4. Kimlik Doğrulama

### 4.1 Ortak arayüz

Üç sağlayıcı tek arayüzün arkasındadır; IMAP kodu hangi sağlayıcıyla konuştuğunu
bilmez:

```go
type CredentialProvider interface {
    SASLClient(ctx context.Context) (sasl.Client, error)
    Refresh(ctx context.Context) error
    AccountKind() AuthKind
}
```

### 4.2 OAuth akışı

RFC 8252 uyarınca **loopback yönlendirme + PKCE**:

1. `127.0.0.1` üzerinde **rastgele** portta geçici HTTP dinleyici açılır.
   (Sabit port kullanılmayacak — çakışma girişi kırar.)
2. Sistem tarayıcısında yetkilendirme sayfası açılır.
3. Yetki kodu yakalanır, token ile takas edilir, dinleyici kapatılır.
4. Refresh token `SecretStore`'a yazılır. Access token yalnızca bellekte tutulur.

**Sağlayıcı kaydında istemci türü kritiktir.** Rastgele loopback portu yalnızca
istemci "yerel/masaüstü" olarak kaydedildiğinde çalışır:

- **Google Cloud:** İstemci türü **kesinlikle "Desktop app"** olmalıdır. "Web
  application" seçilirse Google yalnızca kaydedilmiş tam URL'lere (port dahil)
  yönlendirme yapar ve rastgele port `400 redirect_uri_mismatch` ile reddedilir.
  Desktop app türünde ise Google her loopback portunu kabul eder.
- **Microsoft Entra:** Yönlendirme URI'si **"Public client/native"** altına
  `http://localhost` olarak girilir. Entra yerel istemcilerde port farkını
  yok sayar.

**Yedek sabit port.** Rastgele port doğru varsayılan olsa da bazı kurumsal
ortamlarda güvenlik yazılımı süreçlerin rastgele port bağlamasını engelliyor.
Bu yüzden `config.json` içinde isteğe bağlı bir `oauthRedirectPort` alanı
bulunur; `0` (varsayılan) rastgele port demektir, sıfırdan farklı bir değer o
portu zorlar. Sağlayıcı tarafında da aynı port kaydedilmek zorundadır — bu
yüzden varsayılan değil, çıkış kapısıdır.

### 4.2.1 Anahtarlık kilitliyse ne olur

`SecretStore` çağrıları **bloke olabilir** ve bu, uygulamayı donduran en sinsi
yoldur. Linux'ta Secret Service, macOS'ta Keychain açılış sonrası kilitli
olabilir; kilitliyken okuma denemesi işletim sistemine parola penceresi
açtırır. O pencere kullanıcı yanıtlayana kadar açık kalır — yani arka planda
sessizce token yenilemeye çalışan bir goroutine süresiz askıda kalır.

**Kural: her `SecretStore` çağrısı zaman aşımlıdır (10 saniye).** Zaman aşımı
`ErrKeyringLocked` döndürür ve UI "işletim sistemi anahtarlığınız kilitli,
lütfen kilidini açın" uyarısı gösterir — uygulama donmaz.

Uygulama detayı, dürüstçe: D-Bus çağrısı Go tarafından iptal edilemez. Zaman
aşımında çağrıyı yapan goroutine, işletim sistemi penceresi kapanana kadar
yaşamaya devam eder. Bu kabul edilebilir bir sızıntıdır, ancak **sınırsız
olmamalıdır**: aynı anda en fazla bir kilit açma denemesi çalışır, ve bir
deneme askıdayken yeni çağrılar beklemeden `ErrKeyringLocked` döner. Aksi halde
her senkron döngüsü bir goroutine daha biriktirir.

Bu, `NewKeyringStore`'daki yoklama (probe) çağrısını da kapsar — yoklamanın
kendisi bloke olabilir.

### 4.3 Sağlayıcıya özgü ayrıntılar

**Microsoft (M365 + Outlook.com)**

- Entra "public client" kaydı, authority `common`.
- Scope'lar: `https://outlook.office.com/IMAP.AccessAsUser.All`,
  `https://outlook.office.com/SMTP.Send`, `offline_access`.
- SASL mekanizması: `XOAUTH2`.
- Not: Basic auth 30 Nisan 2026'da tamamen kapandı; OAuth zorunludur.

**Google (Gmail + Workspace)**

- Scope: `https://mail.google.com/`.
- Bu bir *restricted scope*'tur. Açık kaynak modeli gereği **client ID kullanıcı
  tarafından** kendi Google Cloud projesinden sağlanır; kurulum adımları README'de
  belgelenir. Böylece CASA denetimi proje yükü olmaktan çıkar.
- Client ID `config.json` içinde saklanır (public client'ta gizli değildir);
  `SecretStore`'a yalnızca refresh token yazılır.
- SASL mekanizması: `XOAUTH2`.

**Parola / app password**

- Genel IMAP sunucuları, Yandex, Zoho, Zimbra, kendi sunucun.
- Yaygın sağlayıcılar için host/port ön ayarları gömülü; gerisi manuel giriş.
- SASL mekanizması: `PLAIN` (yalnızca TLS üzerinde).

### 4.4 Sır saklama

`SecretStore` bir arayüzdür. Birincil uygulama saf Go anahtarlık
(`zalando/go-keyring`): Windows Credential Manager, macOS Keychain, Linux Secret
Service. **Yedek yol zorunludur:** Secret Service her Linux masaüstünde bulunmaz;
bu durumda ana parolayla türetilen anahtar (Argon2id) ile AES-GCM şifreli dosya
kullanılır.

Windows Credential Manager'ın kimlik başına ~2.5 KB sınırı vardır; refresh token'lar
sığar, ancak `SecretStore` uzun değerlerde anlamlı hata döndürmelidir.

---

## 5. Veri Modeli

### 5.0 Diskteki yerleşim

Tüm kullanıcı verisi platformun standart uygulama veri dizini altında, `nexus-mail`
klasöründe tutulur:

| Platform | Yol |
|---|---|
| Windows | `%APPDATA%\nexus-mail\` |
| macOS | `~/Library/Application Support/nexus-mail/` |
| Linux | `$XDG_DATA_HOME/nexus-mail/` (yoksa `~/.local/share/nexus-mail/`) |

İçerik: `mail.db` (SQLite + WAL yan dosyaları), `attachments/` (P1+), `config.json`
(sır içermez), `logs/`. Sırlar hiçbir zaman bu dizine yazılmaz — yalnızca
`SecretStore` üzerinden OS anahtarlığına gider.

Şema `internal/store/migrations/` altında sıralı SQL dosyaları olarak tutulur.
Veritabanı WAL modunda açılır; `busy_timeout` ayarlanır.

```sql
CREATE TABLE accounts (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  display_name  TEXT NOT NULL DEFAULT '',
  provider      TEXT NOT NULL,            -- microsoft | google | generic
  auth_kind     TEXT NOT NULL,            -- oauth | password
  imap_host     TEXT NOT NULL,
  imap_port     INTEGER NOT NULL,
  smtp_host     TEXT NOT NULL DEFAULT '',
  smtp_port     INTEGER NOT NULL DEFAULT 0,
  secret_ref    TEXT NOT NULL,            -- SecretStore anahtarı; sır burada TUTULMAZ
  created_at    INTEGER NOT NULL
);

CREATE TABLE folders (
  id              INTEGER PRIMARY KEY,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  path            TEXT NOT NULL,
  delimiter       TEXT NOT NULL DEFAULT '/',
  attributes      TEXT NOT NULL DEFAULT '',  -- \Sent, \Trash, \Junk ...
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
  to_addrs        TEXT NOT NULL DEFAULT '',   -- JSON
  cc_addrs        TEXT NOT NULL DEFAULT '',   -- JSON
  date            INTEGER NOT NULL DEFAULT 0,
  internal_date   INTEGER NOT NULL DEFAULT 0,
  size            INTEGER NOT NULL DEFAULT 0,
  snippet         TEXT NOT NULL DEFAULT '',
  flags           TEXT NOT NULL DEFAULT '',   -- JSON dizi
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
  kind            TEXT NOT NULL,   -- flag_add | flag_remove | move | delete
  payload         TEXT NOT NULL,   -- JSON
  state           TEXT NOT NULL,   -- pending | running | failed | done
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT NOT NULL DEFAULT '',
  created_at      INTEGER NOT NULL,
  next_attempt_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_operations_queue ON operations(account_id, state, next_attempt_at);
```

Ek olarak, `operations` tablosuna bir alan daha giriyor — gerekçesi §6.6'da:

```sql
ALTER TABLE operations ADD COLUMN folder_id INTEGER
  REFERENCES folders(id) ON DELETE CASCADE;
ALTER TABLE operations ADD COLUMN uid_validity INTEGER NOT NULL DEFAULT 0;
```

(001_init.sql içinde doğrudan tablo tanımına yazılır; yukarıdaki `ALTER` biçimi
yalnızca hangi alanların eklendiğini göstermek için.)

### 5.2 Arama indeksi ve zorunlu tetikleyiciler

```sql
CREATE VIRTUAL TABLE fts_messages USING fts5(
  subject,
  from_addr,
  snippet,
  content='messages',
  content_rowid='id'
);
```

FTS5 sanal tabloları ana tabloyla **kendiliğinden senkronize olmaz.** Bunu
uygulama kodunda yapmak da yanlış: her yazma yolunun FTS'i hatırlaması gerekir ve
tek bir eksik yol indeksi sessizce eksik bırakır. Doğru yer tetikleyicilerdir —
veritabanı seviyesinde, atlanması imkânsız:

```sql
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

Üç incelik, üçü de atlanırsa arama sessizce yanlış sonuç verir:

1. **FTS5'te `DELETE` sıradan bir silme değil.** Satır, `'delete'` komutu ve
   **eskiden indekslenmiş değerlerin birebir aynısı** verilerek kaldırılır.
   Yanlış değer verilirse indeks bozulur. `old.*` kullanmak bunu garanti eder —
   ama yalnızca **her yazma tetikleyiciden geçtiği sürece.** Bu yüzden kural
   şudur: uygulama kodu `fts_messages`'a asla doğrudan yazmaz.
2. **`UPDATE` sil-ve-ekle olarak yazılır**; FTS5'te yerinde güncelleme yoktur.
   Bu, `messages` üzerindeki `ON CONFLICT DO UPDATE` (upsert) yolunu da kapsar:
   upsert INSERT değil UPDATE tetikleyicisini çalıştırır.
3. **`ResetFolder` FTS'i elle temizlemez.** `DELETE FROM messages` zaten silme
   tetikleyicisini satır satır çalıştırır.

**Gövde indekslemesi P3'e ertelendi.** Önceki karar gövdeyi de P0'da
indekslemekti; gerekçe "sonradan tam yeniden indeksleme yapmamak"tı. Bu gerekçe
gövdeler için geçerli değil: gövdeler talep üzerine indiğinden P0'da indeks
zaten eksik olacaktı. P3'te `message_bodies` üzerinde ayrı bir FTS tablosu ve
tek seferlik backfill yapılacak; o an yerelde ne varsa onun üzerinde çalışacağı
için ucuz. Başlık, gönderen ve snippet indeksi ise P0'da eksiksiz dolar, çünkü
bu üçü her mesajla birlikte gelir.

### 5.3 Şemadaki kritik alanlar

Bunlar orijinal plandan eksikti; yoklukları senkronu sessizce bozar:

1. **`folders.uid_validity`** — Sunucu bu değeri değiştirdiğinde klasördeki tüm
   UID'ler geçersizleşir. Tespit edilmezse yanlış mesaj silinir/güncellenir.
   Kural: değişim algılandığında klasörün yerel içeriği silinir ve yeniden çekilir.
2. **`folders.highest_modseq`** — CONDSTORE tabanlı delta senkronun dayanağı.
3. **`UNIQUE(account_id, folder_id, uid)`** — Yeniden bağlanmalarda mükerrer kayıt
   olmamasını uygulama mantığı değil, veritabanı kısıtı garanti eder.
4. **`operations`** — Çevrimdışı desteğin tamamı bu tablodan gelir.

---

## 6. Senkronizasyon Motoru

### 6.1 Bağlantı modeli

Hesap başına **iki IMAP bağlantısı**:

- **IDLE bağlantısı** — Seçili klasörde bekler, sunucu itmelerini dinler.
- **Komut bağlantısı** — LIST, STATUS, FETCH, STORE, MOVE için.

Gerekçe protokoldür: IDLE durumundaki bir bağlantı komut kabul edemez. Tek bağlantı
kullanan tasarımlar, kullanıcı bir mail açtığında IDLE'ı kırar ve bildirim kaçırır.

Bağlantı sayısı hesap başına 2-3 ile sınırlanır: Gmail eşzamanlı IMAP bağlantılarını
sınırlar ve aşımda hesabı geçici olarak kilitler.

### 6.2 İlk senkron

1. `LIST` ile klasör ağacı; özel klasörler (`\Sent`, `\Trash`, `\Junk`) öznitelikten
   tespit edilir.
2. Her klasör için `STATUS` (UIDVALIDITY, UIDNEXT, MESSAGES, UNSEEN).
3. Gelen Kutusu ve özel klasörlerde son N (varsayılan 1000) UID için başlık `FETCH`:
   ENVELOPE, FLAGS, BODYSTRUCTURE, RFC822.SIZE, INTERNALDATE.
4. **Diğer klasörler yalnızca listelenir**; başlıkları kullanıcı o klasörü ilk kez
   açtığında çekilir (tembel yükleme). Böylece 80 klasörlü kurumsal hesaplarda ilk
   senkron dakikalarca sürmez.
5. Gövdeler çekilmez — kullanıcı maile tıkladığında talep üzerine indirilir.
6. `BODYSTRUCTURE` çözümlenerek ek dosya **meta verisi** (`attachments` tablosu)
   yazılır; dosyaların kendisi indirilmez (P0 dışı).

**Tarih ayrıştırma kuralı.** RFC 2822 `Date:` başlığı güvenilmezdir: spam
gönderenler ve bozuk sunucular `Date: Pzt, 99 Xyz 2026 29:99:99` gibi
ayrıştırılamaz ya da 2077 yılını gösteren değerler yollar. (`time.Parse`
paniklemez, hata döndürür — ama sonuç aynı: elde geçersiz bir tarih kalır.)

Kural: **`INTERNALDATE` kesin doğru kabul edilir.** `Date:` başlığı
ayrıştırılamazsa, sıfır dönerse, ya da `INTERNALDATE`'den **48 saatten fazla
ileride** ise `messages.date` alanına `internal_date` yazılır. Eşiğin sebebi:
gerçek maillerde saat dilimi hataları birkaç saatlik sapma üretir, bu normaldir;
iki günü aşan ileri tarih ya bozuk ya kasıtlıdır ve her iki durumda da sunucunun
teslim zamanı daha güvenilir bilgidir.

Bu kural görüntülemeyi kurtarmıyor — liste sıralaması ve arayüzdeki tarih baştan
`internal_date` üzerinden çalışıyor, yani o taraf zaten korunuyordu. Kuralın
amacı `date` kolonunun çöp taşımaması: P3'teki `before:`/`after:` arama
operatörleri ve P4'teki kurallar motoru o kolonu okuyacak.

### 6.3 Delta senkron — iki yollu

- **CONDSTORE/QRESYNC varsa:** `SELECT` sırasında QRESYNC parametresi, ardından
  `FETCH ... (CHANGEDSINCE <modseq>)`. Yalnızca değişenler gelir.
- **Yoksa (zorunlu yedek yol):** UIDNEXT karşılaştırmasıyla yeni mesajlar, mevcut
  UID aralığında `FETCH FLAGS` ile bayrak farkı, eksik UID'lerden silinenler.

Yedek yol opsiyonel değildir: bazı kurumsal Zimbra ve eski Dovecot kurulumları
CONDSTORE desteklemez.

### 6.4 IDLE döngüsü

- IDLE en geç **29 dakikada bir** yeniden başlatılır (RFC önerisi; NAT ve sunucu
  zaman aşımları bunu gerektirir).
- Kopmada üstel geri çekilme ile yeniden bağlanma (taban 2 sn, tavan 5 dk, jitter'lı).
- Yeniden bağlanmanın ardından **her zaman** delta senkron çalıştırılır — IDLE
  kopukken gelen değişiklikler ancak böyle yakalanır.

### 6.5 Giden işlem kuyruğu

Kullanıcı bir eylem yaptığında (okundu, yıldız, taşı, sil):

1. SQLite **anında** güncellenir; UI iyimser olarak yansıtır.
2. `operations` tablosuna niyet yazılır.
3. Worker kuyruğu boşaltır: `UID STORE`, `UID MOVE`, `UID EXPUNGE`.
4. Başarıda `done`; başarısızlıkta `attempts++` ve `next_attempt_at` geri çekilmeyle
   ileri atılır.

**Kurallar:**

- Her işlem **idempotent** olmalıdır (aynı bayrağı iki kez eklemek zararsızdır).
- Çakışmada **sunucu kazanır**: delta senkron yerel durumu ezer, bekleyen işlemler
  yeniden uygulanır.
- **Tek bir bozuk işlem kuyruğu bloke etmez.** Kuyruk mesaj bazında ilerler; kalıcı
  hataya düşen işlem `failed` olarak işaretlenip kullanıcıya bildirilir, diğerleri akar.

### 6.6 UIDVALIDITY değişimi ile bekleyen işlemlerin çakışması

Bu, tasarımın en sinsi veri kaybı yolu ve iki ayrı kuralın kesişiminde duruyor:
§5.3 "UIDVALIDITY değişirse klasörü sıfırla" diyor, §6.5 "işlemler UID üzerinden
kuyruğa alınır" diyor. İkisi tek başına doğru, birlikte tehlikeli.

**Senaryo:** Kullanıcı çevrimdışıyken `UID 55`'i silmek üzere işlem kuyruğa
girer. Bağlantı geldiğinde sunucu klasörü yeniden yaratmış ve `UIDVALIDITY`
değişmiştir. Artık sunucudaki `UID 55` **başka bir maildir.** Kuyruk boşaltılırsa
yanlış mail silinir — ve kullanıcı bunu asla fark etmez.

**Uygulanmayacak çözüm: işlemleri `Message-ID` üzerinden kuyruğa almak.** IMAP'te
"şu Message-ID'li mesajı sil" diye bir komut yoktur; her işlem için
`SEARCH HEADER MESSAGE-ID` çalıştırmak gerekir. Bu yavaştır, her sunucu o başlığı
indekslemez, ve Message-ID ne benzersizliği ne varlığı garanti edilen bir alandır.
Protokol UID üzerine kuruludur; ondan kaçmak yerine UID'yi geçerliliğiyle birlikte
saklamak gerekir.

**Uygulanacak çözüm: işleme UIDVALIDITY damgası.** Her `operations` satırı
kuyruğa alınırken ait olduğu `folder_id` ve o andaki `uid_validity` ile birlikte
yazılır. Boşaltma anında worker şunu yapar:

```
op.uid_validity != folder.uid_validity  →  op.state = 'dropped'
```

Bu, "sıfırlama sırasında kuyruğu temizle" yaklaşımından üstündür: sıfırlamanın
başka bir bağlantıda ya da başka bir oturumda olduğu durumu da yakalar, çünkü
kontrol sıfırlama anında değil **işlemin uygulanacağı anda** yapılır.

`dropped` işlemler sessizce yok sayılmaz. Kullanıcıya "sunucu bu klasörü yeniden
oluşturduğu için N bekleyen değişiklik uygulanamadı" bildirimi gösterilir; veri
kaybı yerine görünür bir bilgi kaybı olur. Bu, doğru olan takas.

### 6.7 Saklama penceresi (retention)

`InitialHeaderCount` ilk çekimi 1000 mesajla sınırlıyor, ama bu büyümeyi
sınırlamıyor: IDLE aylarca çalıştıkça veritabanı sürekli şişer. 50.000 maillik
bir hesapta günde 50 mail, yılda ~18.000 yeni satır demektir.

**Politika:** Ayarlanabilir bir saklama penceresi tutulur, varsayılanı
**son 12 ay veya klasör başına 25.000 mesaj** — hangisi önce dolarsa. Pencerenin
dışına düşen satırlar `messages` tablosundan silinir; `message_bodies`,
`attachments` ve FTS satırları `ON DELETE CASCADE` ve tetikleyicilerle birlikte
gider.

Üç kural:

1. **Silme yereldir, sunucuya dokunmaz.** Yerel temizlik ile IMAP silme işlemi
   birbirine karıştırılmamalıdır; temizlik `operations` tablosuna hiçbir şey
   yazmaz.
2. **Yıldızlı ve bayraklı mesajlar pencereden muaftır.** Kullanıcının işaretlediği
   bir mail yaşına göre silinmez.
3. **Kullanıcı pencereyi kapatabilir** ("her şeyi tut"). Diski kullanıcının
   kararına bırakmak, yerel-önce bir uygulamada doğru varsayılan.

Uygulama zamanı: politika P0'da belgelenir, temizlik işi **IDLE ile birlikte
M2'de** gelir. Gerekçe: sınırsız büyüme ancak canlı senkron çalıştığında ortaya
çıkan bir sorundur, M1'de mevcut değildir.

---

## 7. HTML Render ve Gizlilik

### 7.1 İzolasyon

Mail içeriği `<iframe sandbox>` içinde, **`allow-same-origin` olmadan**, `srcdoc`
ile beslenerek render edilir; üstüne kısıtlayıcı CSP eklenir. DOMPurify ile
sanitizasyon buna **ek** katmandır, alternatifi değil.

Shadow DOM bu iş için yeterli değildir: CSS'i izole eder, JavaScript'i etmez.

### 7.2 Tracker engelleme

Uzak kaynaklar varsayılan olarak engellenir. Engelleme yalnızca `<img src>` ile
sınırlı değildir; `background` öznitelikleri ve CSS `url()` çağrıları da yeniden
yazılır. Kullanıcı "resimleri göster" dediğinde istekler Go tarafından proxy'lenir;
böylece IP adresi ve referrer sızmaz.

### 7.3 Gövde teslimi: IPC değil, özel protokol

**Bu, önceki tasarım kararını değiştiriyor.** Önceki plan gövdeyi Wails
binding'i üzerinden JSON string olarak frontend'e geçirip `<iframe srcdoc>` ile
render etmekti. Bu yaklaşım iki yerde kırılıyor:

- **Büyük gövdeler UI'ı dondurur.** Inline base64 resimlerle dolu bir e-bülten
  8 MB'ı aşabilir. Bu boyutta bir string'i JSON'a serileştirip IPC'den geçirip
  JavaScript'te parse etmek ana iş parçacığını gözle görülür süre bloke eder —
  ve "sıfır gecikme" vaadinin en görünür ihlali tam burada olur.
- **`srcdoc` gerçek CSP başlığı taşıyamaz.** `<meta http-equiv>` ile konan CSP,
  gerçek bir HTTP başlığından daha zayıftır ve bazı direktifleri hiç
  desteklemez.

**Yeni karar:** Gövdeler Wails'in özel şema (custom scheme) handler'ı üzerinden
servis edilir. Frontend iframe'i şuna bakar:

```
<iframe sandbox="..." src="wails://mail-body/<message_id>">
```

Go tarafındaki handler gövdeyi veritabanından okur, **temizler**, ve gerçek
`Content-Security-Policy` başlığıyla birlikte döndürür. IPC'den geçen tek şey
mesaj kimliğidir; içerik hiç serileştirilmez.

Bunun üç sonucu var, ikisi kazanç:

1. **Temizleme Go tarafına taşınır.** DOMPurify yerine `bluemonday` (saf Go,
   cgo yok) kullanılır. Bu, M1'in 13. ve 16. görevlerini değiştirir — aşağıya
   bakınız.
2. **Uzak kaynak proxy'si kendiliğinden çözülür.** Tasarım §7.2 "istekler Go
   üzerinden proxy'lenir" diyordu ama `srcdoc` ile bunu yapmanın temiz bir yolu
   yoktu. Özel protokolle onaylanmış uzak resimler
   `wails://mail-asset/<message_id>/<url_hash>` altında servis edilir; Go isteği
   kendisi yapar, `Referer` göndermez, sonucu önbelleğe alır. Kullanıcının IP'si
   hiç ortaya çıkmaz.
3. **İzolasyon aynı kalır, hatta güçlenir.** `sandbox` özniteliği `src` ile de
   geçerlidir ve `allow-same-origin` yine verilmez — iframe opak kaynağa (opaque
   origin) sahip olmayı sürdürür. Üstüne artık gerçek bir CSP başlığı biner.

**Aynı kural ekler için de geçerlidir** (P1): ek dosyalar IPC'den geçmez,
`wails://mail-attachment/<id>` üzerinden servis edilir.

**Handler iptali zorunludur.** Kullanıcı `j`/`k` ya da ok tuşunu basılı
tuttuğunda seçim saniyede onlarca kez değişir ve her değişim bir gövde isteği
tetikler. İstek iptal edilse bile handler veritabanı okumaya ve HTML temizlemeye
devam ederse CPU boşa yanar; ekler ve inline resimler geldiğinde (P1) bu bir
istek dalgasına dönüşür.

Üç kural:

1. Handler `req.Context()` kullanır ve `<-ctx.Done()` tetiklendiğinde
   veritabanı sorgusunu ve temizleme işini **derhal keser**. Sorgular
   `QueryRowContext` ile, temizleme ise parça parça yapılıp aralarda `ctx`
   kontrol edilerek çalışır.
2. Temizlenmiş HTML mesaj kimliğine göre önbelleğe alınır (LRU, ~32 giriş).
   Aynı maile geri dönmek yeniden temizleme yapmaz.
3. Frontend seçim değişimini ~120 ms debounce eder. Kullanıcı listede hızla
   geçerken ara mesajlar hiç istenmez.

**M1 plan etkisi:** Task 13 (DOMPurify sanitizer) ve Task 16 (srcdoc'lu
MessageView) bu karara göre yeniden yazılmalıdır. Yeni haliyle Task 13 Go
tarafında `internal/mailhtml` paketi ve bluemonday politikası olur; testleri de
Go tarafına taşınır — altı tracker vektörü ve XSS altın dosya testleri aynı
kalır, yalnızca dili değişir. Task 16'da `MessageView` küçülür: sanitizasyon
yapmaz, yalnızca doğru `src`'yi ve uyarı çubuğunu yönetir.

---

## 8. Kullanıcı Arayüzü

- Üç sütun (Klasörler / Liste / İçerik), `react-resizable-panels` ile boyutlandırılabilir.
- Mesaj listesi `TanStack Virtual` ile sanallaştırılır.
- Global durum `Zustand`; Go tarafındaki değişiklikler Wails olaylarıyla yayılır.
- Konuşma gruplama: basitleştirilmiş JWZ algoritması — `References` ve `In-Reply-To`
  zinciri birincil, normalize edilmiş konu başlığı yedek.
- Karanlık/aydınlık tema.

### 8.1 Arka plan yaşam döngüsü — P0 için net kural

Wails v3 sistem tepsisi ve çoklu pencere destekliyor; bu, sürüm seçiminin de
gerekçesiydi. Ama **P0'da tepside çalışma yok.**

**Kural: pencere kapatıldığında uygulama tamamen sonlanır.** IMAP bağlantıları
kapanır, IDLE döngüleri durur, senkron motoru iner. Uygulama açıkken senkron
eder, kapalıyken etmez.

Bu, ilk bakışta bir e-posta istemcisi için tuhaf görünüyor — Outlook tepside
bekler. Bilinçli bir erteleme, çünkü tepside sessiz çalışma tek bir özellik
değil, bir küme: tepsi ikonu, okunmamış sayacı, işletim sistemine özgü bildirim
(Windows Toast, macOS Notification Center, Linux'ta libnotify), "kapanışta
tepsiye küçült" ayarı, oturum açılışında otomatik başlatma, ve bildirime
tıklandığında doğru maili açma. Bunların her biri üç platformda ayrı ayrı
davranıyor.

Bu kümeyi senkron motorunun kendisi güvenilir çalışmadan önce ele almak, iki
zor problemi birbirine karıştırmak olur. **Tepsi ve bildirimler M4'e** (P1
kapsamına) alınmıştır; o noktada delta senkron ve işlem kuyruğu çalışıyor
olacak ve bildirimin ne zaman atılacağı sorusu net bir cevabı olan bir soru
haline gelecek.

Kapanışta yapılacaklar açıkça tanımlıdır: bekleyen `operations` satırları diskte
kalır (bir sonraki açılışta boşaltılır), yarım kalan senkron `context` iptaliyle
sonlanır, SQLite WAL checkpoint'i alınır.

---

## 9. Hata Yönetimi

Hatalar dört sınıfa ayrılır ve her sınıfın davranışı farklıdır:

| Sınıf | Davranış |
|---|---|
| Geçici ağ | Sessiz yeniden deneme, üstel geri çekilme. |
| Kimlik doğrulama | Önce token yenile; başarısızsa hesabı "yeniden giriş gerekli" durumuna al ve kullanıcıya bildir. |
| Protokol | Logla, ilgili klasörü karantinaya al, diğer klasörler çalışmaya devam etsin. |
| Kalıcı | Kullanıcıya bildir, otomatik yeniden deneme yapma. |

Her hesabın senkron durumu (bağlı / senkronize ediliyor / çevrimdışı / hata) UI'da
görünür olacaktır.

### 9.1 Loglama ve gözlemlenebilirlik

Arka planda IDLE döngüleri, delta senkron ve kuyruk worker'ı dönüyor. Kullanıcı
"maillerim senkronize olmuyor" diye issue açtığında elimizde ne olduğu şimdiden
belirlenmelidir, yoksa cevap "bilmiyorum" olur.

**Yapı:** `log/slog` ile yapılandırılmış (structured) loglama, JSON handler.
Dosya döndürme `natefinch/lumberjack` ile (saf Go): 10 MB'da yeni dosya, en fazla
3 yedek, 28 gün. Hedef `<veri dizini>/logs/nexus.log`. Varsayılan seviye `INFO`;
`--debug` bayrağı ve ayarlardan açılabilen bir anahtar `DEBUG`'a çıkarır.

Her log satırı en az şu alanları taşır: `account_id`, `folder_path`, `operation`,
ve hata varsa §9'daki `error_class`. Bu dört alan, bir senkron sorununu okumak
için gereken minimumdur.

**Gizlilik kuralları — bu bölümün en önemli kısmı.** Gizlilik odaklı bir
uygulamada "logları dışa aktar" düğmesi, dikkat edilmezse kullanıcının mail
içeriğini bir GitHub issue'suna taşıyan bir sızıntı yoluna dönüşür. Bu yüzden
loglanması **yasak** olan alanlar açıkça sayılır:

- Mesaj konusu, snippet, gövde, ek dosya adı
- Gönderen ve alıcı adresleri — hesabın kendi adresi dahil
- Parola, access token, refresh token, `Authorization` başlığı
- Ham IMAP protokol trafiği (`DEBUG` seviyesinde bile: kimlik doğrulama satırı
  token içerir)

Loglanması **gereken** şeyler bunların yerine geçer: hesap kimliği (e-posta
değil, veritabanı id'si), klasör yolu, UID aralıkları, mesaj **sayıları**,
süreler, sunucu capability listesi, hata sınıfı ve sunucunun döndürdüğü hata
kodu.

Bu ayrım bir CI testiyle korunur (doğrulama dokümanı §6'ya eklendi): bilinen
hassas değerlerle bir senkron çalıştırılır ve log çıktısında hiçbirinin
görünmediği iddia edilir.

**"Logları dışa aktar" düğmesi P0 kapsamındadır.** Ayarlar ekranında, log
dosyalarını tek bir zip'e toplayıp kullanıcının seçtiği yere kaydeder. Kaydetmeden
önce içeriği kullanıcıya gösterir — ne paylaştığını görmeden paylaşmasını
istemiyoruz.

---

## 10. Test Stratejisi

**Bellek içi sahte IMAP sunucusu.** go-imap v2 kendi `imapserver` paketini içerir.
Bununla senkron motorunun tamamı gerçek ağ olmadan test edilir. Kapsanacak senaryolar:

- UIDVALIDITY değişimi → klasörün doğru şekilde sıfırlanması
- Senkron ortasında bağlantı kopması → yeniden bağlanma ve durum bütünlüğü
- CONDSTORE desteklemeyen sunucu → yedek delta yolunun devreye girmesi
- Çakışan bayraklar → "sunucu kazanır" kuralının uygulanması
- Kuyrukta kalıcı hata → diğer işlemlerin bloke olmaması

### 10.1 Zaman bağımlı mantığın testi

Motor zamana bağlı kararlar veriyor: 29 dakikada bir IDLE yenileme, 2 saniyeden
5 dakikaya üstel geri çekilme, kuyruk yeniden deneme aralıkları. Bunları
`time.Sleep` ile test etmek iki şekilde başarısız olur — CI saatlerce sürer ya
da testler flaky olur.

**Kural: `testing/synctest` kullanılır.** Go 1.25'te stabil hale geldi ve
1.26'da mevcut. Üretim kodu normal `time.After`, `time.Sleep` ve `time.Now`
kullanmaya devam eder; `synctest.Test` bloğu içinde bu çağrılar sahte bir saate
bağlanır ve tüm goroutine'ler beklemeye girdiğinde saat anında ilerler. 29
dakikalık IDLE yenilemesi mikrosaniyelerde test edilir.

`clockwork` gibi bir saat sahteleme kütüphanesi **kullanılmayacak.** Bu tür
kütüphaneler üretim kodunun `time` paketini bırakıp enjekte edilen bir `Clock`
arayüzünü kullanmasını gerektirir; yani üretim kodu test uğruna deforme olur ve
her yeni geliştirici bu deformasyonu öğrenmek zorunda kalır. `synctest` aynı
sonucu bağımlılık eklemeden ve kodu bozmadan veriyor.

Ek testler:

- **Sanitizer:** XSS vektör dosyalarıyla altın dosya (golden file) testleri.
- **Store:** migration ileri/geri, eşzamanlı okuma altında yazma.
- **Frontend:** Vitest + Testing Library.

Test senaryolarının tam listesi, kabul kriterleri, CI yapılandırması ve mimari
kuralların otomatik zorlanması ayrı bir dokümanda:
[P0 Doğrulama ve Denetim Stratejisi](2026-09-09-nexus-mail-p0-verification.md).

---

## 11. P0 Kapsam Dışı (bilinçli YAGNI)

Gönderme ve SMTP, composer/editör, arama arayüzü, akıllı kurallar, snooze, etiketler,
AI entegrasyonu, CalDAV takvim, CardDAV kişiler, ek dosya indirme, çoklu pencere,
otomatik güncelleme, kod imzalama.

Ayrıca §8.1 uyarınca **sistem tepsisi, arka planda çalışma ve işletim sistemi
bildirimleri** — bunlar M4'e (P1) alınmıştır.

FTS5 indeksi P0'da oluşturulur ve başlık/gönderen/snippet için **eksiksiz dolar**;
yalnızca arama arayüzü ertelenir. Gövde indeksi §5.2 uyarınca P3'e aittir.

Saklama penceresi (§6.7) politikası P0'da belgelenir, temizlik işi M2'de gelir.

---

## 12. İç Kilometre Taşları

Geniş P0 kapsamı seçildiği için üç doğrulama noktasına bölünür. Her nokta kendi
başına çalışır durumda kalır:

- **M1 — Salt okunur istemci.** Hesap bağlama (3 auth yolu), klasör ve başlık çekme,
  üç sütunlu UI, izole HTML render, karanlık tema.
- **M2 — Canlı senkron.** IDLE döngüsü, delta senkron (her iki yol), yeniden bağlanma.
- **M3 — Durum yazma.** İşlem kuyruğu: okundu/okunmadı, yıldız, klasöre taşı, sil;
  çevrimdışı dayanıklı.

---

## 13. Riskler

| Risk | Etki | Azaltma |
|---|---|---|
| Wails v3 beta'da kırıcı değişiklik | Orta | Sürüm sabitlenir; yükseltme kontrollü ve ayrı bir iş olarak yapılır. |
| go-imap v2 beta API değişimi | Düşük | API dondurulmuş sayılıyor; `imapx` sarmalayıcısı temas yüzeyini tek yerde tutar. |
| Sunucu davranış farklılıkları (Gmail / Exchange / Zimbra) | **Yüksek** | Sahte sunucu testleri + üç gerçek hesapla erken elle doğrulama. |
| Gmail bağlantı limiti aşımı → geçici kilit | Orta | Hesap başına en fazla 2-3 bağlantı; geri çekilmeli yeniden bağlanma. |
| Linux'ta Secret Service yokluğu | Orta | Ana parolayla korunan şifreli dosya yedek yolu (Argon2id + AES-GCM). |
| Geniş P0 kapsamının uzaması | Orta | M1/M2/M3 kilometre taşları; her biri teslim edilebilir halde kalır. |

---

## 14. Kaynaklar

- [Wails v3 beta durumu](https://v3.wails.io/blog/wails-v3-beta/) ve
  [GA release tracker](https://github.com/wailsapp/wails/issues/5844)
- [Wails v2 → v3 geçiş rehberi](https://v3.wails.io/migration/v2-to-v3/)
- [emersion/go-imap v2](https://pkg.go.dev/github.com/emersion/go-imap/v2)
- [Microsoft: IMAP/POP/SMTP OAuth kimlik doğrulama](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth)
- [Google restricted scope doğrulaması](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification)
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)
- [zalando/go-keyring](https://github.com/zalando/go-keyring)

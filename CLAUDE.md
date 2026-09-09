# Nexus Mail — proje kuralları

Bu dosya depoda tutulur ve `.gitignore`'a **eklenmez**: kurallar makineye değil
projeye aittir.

## Commit kuralları

- Commit mesajlarının sonuna **yalnızca** şu satır eklenir:

  ```
  Co-authored-by: ismet kabatepe <>
  ```

- Commit mesajlarında, açıklamalarında veya trailer'larında **hiçbir yapay zekâ
  aracının adı geçmez.** `Co-Authored-By: Claude`, `Generated with ...` ve
  benzeri satırlar eklenmez.
- Mesaj gövdesi *ne* yapıldığını değil, **neden** yapıldığını anlatır. Kararın
  gerekçesi ve reddedilen alternatif, koda bakılarak anlaşılamayacak tek şeydir.
- Konu satırı 72 karakteri geçmez; gövde satırları 72 karakterde sarılır.
- Conventional Commits öneki kullanılır: `feat`, `fix`, `chore`, `docs`,
  `test`, `refactor`, `perf`, `ci`.

## Dal ve PR akışı

- `main` korumalıdır; doğrudan push yapılmaz.
- Her iş kendi dalında yapılır, PR ile birleştirilir.
- PR açıklamalarında da yapay zekâ aracı adı veya imzası bulunmaz.

## Mimari kurallar

Bunlar CI'da `depguard` ile zorlanır, yorum düzeyinde kalmaz:

- Bağımlılık yönü tek yönlüdür: `app → sync → {imapx, store, auth}`.
  Alt katman üst katmanı import etmez.
- `internal/model` bir yaprak pakettir; projeden hiçbir şey import etmez.
- `internal/sync`, `github.com/emersion/go-imap/v2`'yi **doğrudan import
  etmez**. Motor `imapx.MailBackend` arayüzü üzerinden çalışır; sahte sunucuya
  karşı test edilebilirliği buna bağlıdır.
- `internal/mailhtml` saf bir dönüşümdür, `net/http` import edemez.
- Uygulama kodu `fts_messages` tablosuna **doğrudan yazmaz**. İndeks
  tetikleyicilerle korunur; CI bunu kontrol eder.
- Zamana bağlı mantık `testing/synctest` ile test edilir. `clockwork` gibi saat
  enjeksiyon kütüphaneleri kullanılmaz.

## cgo politikası

- Windows ve Linux derlemeleri `CGO_ENABLED=0` ile yapılır ve CI'da zorunlu
  kontroldür. Bu, bağımlılıklarımızın saf Go kaldığının kanıtıdır.
- macOS'ta cgo açıktır; Wails'in WebView bağlaması bunu gerektirir.
- Bağımlılık ağacına cgo gerektiren paket eklenmez.

## Gizlilik kuralları

- Parola, access token ve refresh token **hiçbir koşulda** veritabanına,
  `config.json`'a veya loglara yazılmaz. Yalnızca `SecretStore` üzerinden OS
  anahtarlığına gider.
- Loglara mail konusu, snippet, gövde, ek dosya adı veya e-posta adresi
  yazılmaz.

## Test disiplini

- Her değişiklik testle başlar.
- `go test ./...` yeşil olmadan commit yapılmaz.
- `-race` cgo gerektirdiği için yalnızca CI'da çalışır.

## Dokümanlar

- Mimari kararlar: `docs/design/`
- Uygulama planları: `docs/plans/`

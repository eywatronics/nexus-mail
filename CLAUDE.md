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

## Arayüz tasarımı

**Her yeni ekran ve her arayüz değişikliğinde `design-taste-frontend` skill'i
kullanılır.** Bu bir tercih değil, proje kuralı.

Skill'in kendi kapsam kuralı (§13) geçerlidir: skill landing page ve portfolyo
içindir, yoğun ürün arayüzü için değil. Nexus Mail'in üç sütunlu ana ekranı
yoğun ürün arayüzüdür. Dolayısıyla:

- **Uygulanmaz:** hero disiplini, eyebrow sayımı, bento ritmi, marquee limiti,
  zigzag capı, scroll-hijack kalıpları. Bu projede karşılığı olan ekran yok.
  İleride bir tanıtım sayfası yapılırsa devreye girer.
- **Uygulanır:** AI Tells (§9), tipografi disiplini (§4.1), Color ve Shape
  Consistency Lock (§4.2, §4.4), kontrast kontrolleri, ikon politikası (§3.C),
  emoji yasağı (§3.D), dark mode protokolü (§8), reduced motion (§6.B).
- **Tam uygulanır:** hesap ekleme gibi tam pencere kaplayan, ikna ve kompozisyon
  işi olan ekranlar.

### Bu projenin dial değerleri

Landing baseline'ı değil, ürün arayüzü değerleri:

| Dial | Değer | Gerekçe |
|---|---|---|
| `DESIGN_VARIANCE` | 3 | Mail istemcisinde asimetrik düzen kullanıcıya düşmanca. Öngörülebilirlik özellik. |
| `MOTION_INTENSITY` | 2 | Anında hissettirmeli. Hover ve basma geri bildirimi var, giriş animasyonu yok. |
| `VISUAL_DENSITY` | 7 | Mail istemcisi yoğundur. §4.4 gereği kart yerine 1px çizgi, sayılarda `font-mono`. |

### Tasarım token'ları

Tek kaynak: `frontend/src/index.css` (`@theme` bloğu) ve
`frontend/src/lib/ui.ts` (paylaşılan sınıf dizileri).

- **Tek vurgu rengi:** `--color-accent` (teal). Seçim, birincil eylem ve odak
  halkası. Bileşen içinde ham `bg-blue-600` gibi bir sınıf yazılmaz.
- **Durum renkleri** (`--color-danger`, `--color-warn`) dekorasyon değildir;
  yalnızca o anlamı taşıdıkları yerde kullanılır. Vurgu için kullanılan bir
  durum rengi, ikinci bir accent'tir.
- **Tek yarıçap:** `--radius-ui` (6px), tüm etkileşimli öğe ve konteynerlerde.
  Paneller, ayraçlar ve pencere kenarı köşesizdir.
- **Tipografi:** Geist (arayüz), Geist Mono (sayı ve tarih). Kendi paketimizde
  barındırılır; gizlilik odaklı bir istemci her açılışta üçüncü tarafa haber
  vermez.
- **Kontrast:** açık zeminde `text-neutral-400` kullanılmaz (~2.8:1, AA'yı
  geçmez). Açık taraf 500'ün altına inmez; koyu tarafta aynı değer 7:1 üzeridir,
  bu yüzden iki taraf simetrik değildir.

### İkonlar

`@phosphor-icons/react`, tek aile, tek boyut ve ağırlık (`ui.ts` içindeki
`ICON`). Elle SVG path çizilmez. Arayüzde, kodda ve görünen metinde emoji
kullanılmaz.

## Mimari kurallar

Bunlar CI'da `depguard` ile zorlanır, yorum düzeyinde kalmaz:

- Bağımlılık yönü tek yönlüdür: `app → sync → {imapx, store, auth}`.
  Alt katman üst katmanı import etmez.
- `internal/model` bir yaprak pakettir; projeden hiçbir şey import etmez.
- `internal/sync`, `github.com/emersion/go-imap/v2`'yi **doğrudan import
  etmez**. Motor `imapx.MailBackend` arayüzü üzerinden çalışır; sahte sunucuya
  karşı test edilebilirliği buna bağlıdır.
- **Yeni protokol arka uçları arayüz üzerinden bağlanır.** `imapx.MailBackend`
  deseni takvim ve kişiler için de geçerlidir: motor somut protokolü değil
  arayüzü görür, protokol kütüphanesi yalnızca kendi paketinde import edilir.
  Yeni bir katman eklenince `.golangci.yml` içindeki `depguard` kuralı da
  eklenir — kural yazılmamışsa mimari kural yok demektir.

  | Katman | Sorumluluk | Arayüz |
  |---|---|---|
  | `internal/imapx` | IMAP | `MailBackend` |
  | `internal/carddavx` | CardDAV, LDAP (M7) | `ContactsBackend` |
  | `internal/caldavx` | CalDAV, ICS (M9b) | `CalendarBackend` |
  | `internal/graphx` | Microsoft Graph (M10) | yukarıdakilerin ikinci uygulaması |

- **Her yeni protokol kendi sahte sunucusuyla gelir.** IMAP için
  `imapmemserver` kullanılıyor; SMTP için `go-smtp`'nin sunucu tarafı, diğerleri
  için eşdeğeri. Gerçek hesaba karşı elle deneme test yerine geçmez.
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

# Nexus Mail yol haritası

Bu doküman M1'den sonrasını tanımlar. Her kilometre taşının kapsamı ve kabul
kriteri burada; hangi özelliğin neden kapsamda olduğu
[özellik envanterinde](../design/feature-inventory.md).

**Kural:** her kilometre taşı kendi başına çalışır durumda kalır. Yarım kalmış
bir taşın üzerine bir sonraki başlamaz.

---

## Durum

| KT | Kapsam | Durum |
|---|---|---|
| **M1** | Salt okunur istemci: hesap bağlama (3 yol), klasör ve başlık senkronu, izole okuma, arama, klavye navigasyonu | Bitti |
| **M2** | Canlı senkron: IDLE, delta senkron, yeniden bağlanma, saklama penceresi | Bitti |
| **M3** | Durum yazma: işlem kuyruğu (okundu, yıldız, taşı, sil), çevrimdışı dayanıklı | Bitti |
| **M4** | Tepsi, bildirimler, arka plan yaşam döngüsü | Kısmen bitti |
| **M5** | Okuma deneyimini tamamla | Sürüyor |
| **M6** | Gönderme | Yeni |
| **M7** | Kişiler | Yeni |
| **M8** | Organizasyon, arama olgunluğu, otomasyon | Yeni |
| **M9** | Uçtan uca şifreleme + takvim | Yeni |
| **M10** | Microsoft Graph / Exchange | Yeni |
| **M11** | Göç (içe/dışa aktarma) | Yeni |
| **M12** | Yerelleştirme ve erişilebilirlik | Yeni |
| **M13** | Sohbet | Yeni |
| **M14** | Ürünleşme | Yeni |

Thunderbird 25 yıllık bir ürün. Bu liste çok yıllık bir yük; değeri
sıralamada ve neyin **bilerek dışarıda** bırakıldığında.

---

## M4 — Tepsi, bildirimler, arka plan yaşam döngüsü

**Bitti:**

- Pencere kapanınca uygulama kapanmıyor, tepside kalıyor; izleyiciler
  çalışmaya devam ediyor
- Tepsi menüsü: aç, şimdi senkronla, çık
- Okunmamış sayısı tepsi ipucunda
- Yeni mail geldiğinde işletim sistemi bildirimi, önizleme ayarlı
  (`notificationPreview`, varsayılan açık)

**Kalan:**

- Varsayılan posta istemcisi olarak ayarla (`mail/components/shell`)
- Görev çubuğu ilerlemesi, Jump List, macOS dock rozeti
- Açılışta başlat

**Doğrulanmayan:** toast'ın gerçekten teslim edildiği. Bildirim servisi çalışan
bir Wails uygulaması gerektirdiği için tek başına denenemiyor; gerçek bir
hesaba mail gelerek doğrulanmalı. Pencere kapatmanın süreci öldürmediği ise
çalışan sürece `WM_CLOSE` gönderilerek doğrulandı.

---

## M5 — Okuma deneyimini tamamla

M1'in yarım bıraktığı yer. Küçük, birbirinden bağımsız parçalar; her biri tek
başına teslim edilebilir.

- ~~Ek indirme, açma, kaydetme~~ — **bitti**
- ~~Kaynağı görüntüle, `.eml` kaydet~~ — **bitti**
- ~~Kodlamayı onar: yanlış çözülmüş mesaj için charset seçici~~ — **bitti**
- ~~Konuşma gruplama~~ — **bitti**
- ~~Gövde kipi: özgün HTML / sade HTML / düz metin~~ — **bitti**
- ~~SPECIAL-USE ile klasör rolleri~~ — **bitti**
- ~~STARTTLS ve kimlik doğrulama mekanizması seçimi~~ — **bitti** (aşağıya bakın)
- Mesaj gövdesinde karanlık mod (`bodyhandler.go` zaten `prefers-color-scheme` yazıyor)
- ~~Silmek çöp kutusuna taşısın~~ — **bitti** (aşağıya bakın)
- ~~Geri al~~ — **bitti** (tek adım, yıkıcı işlemler için)
- Mesajda bul, okundu işaretleme davranışı, yinele

### UTF8=ACCEPT neden burada değil

Planda "Türkçe klasör adları için doğrudan ilgili" yazıyordu. Ölçünce öyle
çıkmadı: go-imap gelen kutu adlarını modified UTF-7'den zaten çözüyor ve giden
komutlarda zaten kodluyor, dolayısıyla "Gönderilmiş Öğeler" IMAP4rev1 bir
sunucuda bugün çalışıyor. Test yazıldı (Türkçe, Rusça, Japonca ve Yunanca
adlar listeleniyor, seçiliyor ve içinden mesaj çekiliyor).

UTF8=ACCEPT'in gerçekten fark yarattığı iki yer var: **APPEND** ile UTF-8
gövde göndermek (M6) ve **sunucu tarafı SEARCH**'te ASCII olmayan ölçüt
kullanmak (M8). İkisinden önce etkinleştirmek, hiçbir şeyin kullanmadığı bir
yetenek pazarlığı olurdu — bu projede tekrar tekrar bulunan desen.

### Geri alma penceresi

Kuyruğun `next_attempt_at` alanı zaten vardı ve `ClaimOperations` zamanı
gelmemiş işlemleri zaten atlıyordu; beş saniye ileri koymak, durum makinesine
hiç dokunmadan geri alma penceresi açtı.

İptal **"hâlâ bekliyor"** yerine **"zamanı hâlâ gelecekte"** koşuluyla
yapılıyor. İşçi bir işlemi ancak zamanı geçmişse okur; silme ancak zamanı
gelecekteyse eşleşir. Zaman yalnızca ileri aktığı için iki pencere inşaat
gereği ayrık — yarış yok.

Pencere `undoWindowSeconds` ile ayarlanabilir; `0` kapatır.

**Kalan:** yinele (redo) yok, ve geri alma tek adım.

### Silme artık yok etmiyor

Delete, UID EXPUNGE demekti: mesaj sunucudan gidiyor, bulunacağı bir çöp
kutusu olmadan. Artık çöp kutusuna taşıyor; çöp kutusunun kendisinde silmek
yok ediyor, yoksa kutu hiç boşaltılamazdı.

Çöp kutusu **role göre** bulunuyor, ada göre değil — SPECIAL-USE işinin ilk
gerçek karşılığı.

**Kalan:** çöp kutusundan kalıcı silmede onay yok.

### Şirket içi Exchange

Planda olmayan ama gerçek kullanımın dayattığı iş. Bağlantı güvenliği kodda
993/örtük TLS olarak sabitti; şirket içi Exchange böyle yayınlanmıyor. IMAP4
servisinin varsayılan `LoginType` değeri `SecureLogin`'dir — bağlantı
yükseltilmeden parola kabul edilmez — ve pek çok kurulum 993'ü hiç açmaz.

Eklenen: hesap başına STARTTLS/örtük TLS seçimi (migration 003), zorunlu
yükseltme, düz metnin loopback dışında reddi, port ve sertifika hatalarının
adlandırılması, ve PLAIN → SASL LOGIN → LOGIN komutu sırası.

**Kalan tek boşluk NTLM/GSSAPI.** Temel kimlik doğrulamayı kapatmış bir
kurumda tek yol budur ve `internal/auth`'ta karşılığı yok. Hata mesajı
sunucunun sunduğu mekanizmaları adlandırdığı için bu duruma düşüldüğü
anlaşılıyor; uygulaması M10'da Graph işiyle birlikte.

### Yazdırma neden burada değil

Envanterde "kaynağı görüntüle, `.eml` kaydet, **yazdır**" tek satırdı; ilk
ikisi yapıldı, üçüncüsü bilerek yapılmadı.

Okuma paneli `allow-same-origin` taşımayan bir iframe. Bu, frontend'deki tek en
önemli satır: mail'e bu uygulamanın origin'ini vermemek. Ama bunun bedeli,
ana pencerenin `iframe.contentWindow` üzerine hiç uzanamaması — yani
`contentWindow.print()` çağrılamaz. Yazdırmayı çalıştırmanın yolu ya sandbox'ı
gevşetmek ya da gövdeyi kendi origin'imizde render etmek; ikisi de panelin
varlık sebebini iptal ediyor.

Şimdilik `.eml` kaydetme bu ihtiyacın pratik karşılığı: dosya diskte, istenen
her şeyle açılabiliyor. Gerçek çözüm muhtemelen ayrı bir yazdırma penceresi
(Wails çoklu pencere) ve orada aynı sanitize edilmiş gövdeyi kendi belgesi
olarak render etmek; M6'da compose penceresi için zaten kurulacak altyapıyla
birlikte ele alınacak.

**Kabul:** gerçek bir hesapta ekli bir mesaj açılıp eki diske kaydedilebiliyor;
ISO-8859-9 kodlaması bozuk gelen bir mesaj elle düzeltilebiliyor; konuşma
zinciri listede gruplanıyor.

---

## M6 — Gönderme

Salt okunur olmaktan çıkmak. Tek en büyük boşluk.

- SMTP (`emersion/go-smtp`): STARTTLS, 8BITMIME, SIZE, SMTPUTF8, XOAUTH2
- **Hesap başına çoklu kimlik** — şema değişikliği; bugün `accounts.display_name` tek kimlik varsayıyor
- İmzalar (kimlik başına metin/HTML/dosya)
- Compose penceresi: yanıtla / tümünü / listeye / ilet / yönlendir / yeni olarak düzenle
- Zengin metin editörü (`contenteditable`) ve düz metin kipi
- Alıntılama ve yanıt konumu
- Alıcı "pill" arayüzü + otomatik tamamlama (toplanan adreslerden başlar)
- Ek ekleme, gömülü resim (`cid:`), **ek hatırlatıcı**
- Taslak otomatik kaydetme
- **Outbox**: kuyruğa al, bağlantı gelince gönder
- Fcc — gönderilen kopyayı Gönderilenler'e yazma (UIDPLUS ile UID öğrenme)
- Otomatik yapılandırma (ISPDB, DNS MX/SRV, tahmin) — hesap eklemeyi üç adımdan bire indirir
- `mailto:` işleyicisi

**Yazım denetimi:** hunspell cgo gerektirir. Composer `contenteditable` üzerine
kurulur ve WebView'in yerleşik denetimi kullanılır; sözlük yönetimi işletim
sistemine kalır.

**Kabul:** gerçek bir hesaptan mesaj gönderiliyor, Gönderilenler'de görünüyor;
uçak modunda yazılan mesaj outbox'ta bekliyor ve bağlantı gelince gidiyor;
uygulama kapatılıp açıldığında bekleyen mesaj kaybolmuyor.

---

## M7 — Kişiler

Compose'un otomatik tamamlaması buna dayanıyor; CardDAV/LDAP kurumsal
kullanımın şartı.

- Yerel adres defteri (aynı SQLite dosyası)
- vCard 4.0 kişi modeli ve düzenleyici (`emersion/go-vcard`)
- CardDAV senkronu (`emersion/go-webdav`) — keşif, ctag/etag
- LDAP dizini (`go-ldap/ldap/v3`) ve **çevrimdışı kopyası** — yerel-önce ilkesinin kişilere uygulanmış hali
- Posta listeleri (dağıtım grupları), compose'da açılma
- Mesaj başlığından kişi ekle/düzenle
- Kişi avatarları — **yalnızca yerel/vCard fotoğrafı**; Gravatar gibi uzak servis yok

**Kabul:** kurumsal bir LDAP dizininden kişi aranıp compose'a alıcı olarak
eklenebiliyor; CardDAV sunucusunda yapılan değişiklik iki yönlü eşitleniyor;
ağ kapalıyken kişi listesi çalışıyor.

---

## M8 — Organizasyon, arama olgunluğu, otomasyon

Envanterde 28 madde çıktı; tek kilometre taşı olarak planlamak aylarca "M8
üzerinde çalışıyoruz" demek olur. Üç teslim edilebilir parçaya bölünür.

### M8a — Elle organizasyon

- Etiketler (renkli, 1–9 kısayolu) — yeni tablo
- Arşivle (tarih bazlı klasörler)
- Klasör paneli modları: birleşik / okunmamış / favori / son / etiket
- **Birleşik gelen kutusu** — çoklu hesabın asıl değeri
- Konuşmayı yoksay / izle
- Hesap rengi
- Etkinlik yöneticisi: senkron, gönderme, filtre işlemlerinin görünür günlüğü

### M8b — Arama olgunluğu

- **Gövde indeksi** — P0'da P3'e bırakılmıştı; geri dolduran iş gerekiyor
- **Kök bulma kararı**: Thunderbird FTS3 + Porter stemmer kullanıyor. Porter
  İngilizce için yazılmış; Türkçe sondan eklemeli bir dil ve aynı yaklaşım
  çalışmaz. Bu ayrı bir karar olarak ele alınır, sessizce Porter takılmaz.
- Gelişmiş arama: 30+ nitelik (rastgele başlık dahil), operatörler, kapsam
- Sanal klasörler (kayıtlı aramalar)
- Hızlı filtre çubuğu (okunmamış/yıldızlı/ekli/etiket + metin, yapışkan)
- Sunucu tarafı IMAP SEARCH — yerel indekste olmayan eski mail için; saklama
  penceresinin doğal tamamlayıcısı
- Hesaplar arası küresel arama

### M8c — Otomasyon

- Kural motoru: **çalışma anları** (gelen, elle, junk sonrası, giden sonrası,
  arşiv öncesi, periyodik) — "gelen mailde çalışır" tek başına yetmiyor
- Eylemler: taşı/kopyala, öncelik, sil, okundu, yıldız, etiket, konuşmayı
  yoksay, şablonla yanıtla, ilet, **yürütmeyi durdur**, junk skoru
- **Filtre günlüğü** — "kural neden çalışmadı" sorusunun tek cevabı
- Junk: Bayes sınıflandırıcı (`jbrukh/bayesian`), **eğitim yerelde kalır**
- Junk: adres defteri beyaz listesi (M7'ye bağlı), sunucu başlıklarına güven,
  eğitim verisini sıfırla/dışa aktar
- MDN (okundu bilgisi) isteği ve yanıt politikası; DSN; öncelik
- Kimlik avı tespiti; `List-Unsubscribe` ile tek tık abonelikten çıkma
- IMAP NAMESPACE, ACL, QUOTA; COMPRESS=DEFLATE; Gmail etiketleri

**Kabul (her parça için ayrı):** M8a — iki hesabın gelen kutusu tek listede
görünüyor. M8b — gövdede geçen bir kelime aranıp bulunuyor, sonuç sanal klasör
olarak kaydediliyor. M8c — bir kural gelen mesajı doğru klasöre taşıyor ve
günlükte neden taşıdığı yazıyor.

---

## M9 — Uçtan uca şifreleme ve takvim

İki bağımsız iş; paralel ilerleyebilirler.

### M9a — Uçtan uca şifreleme

- OpenPGP (`ProtonMail/go-crypto`): şifrele, imzala, çöz, doğrula
- Anahtar yöneticisi; anahtarlar **OS anahtarlığında** (mevcut `SecretStore`)
- WKD anahtar keşfi. Anahtar sunucusu opsiyonel — meta veri sızdırır
- Anahtar asistanı: compose'da alıcı başına anahtar durumu
- S/MIME (`go.mozilla.org/pkcs7` + `crypto/x509`)
- Konu satırını şifreleme
- **DKIM / SPF / ARC doğrulama göstergesi** (`emersion/go-msgauth`) —
  Thunderbird'ün çekirdekte yapmadığı bir artı

### M9b — Takvim ve görevler

- Yerel takvim deposu (aynı SQLite dosyası)
- CalDAV iki yönlü senkron (`emersion/go-webdav`), ctag/etag
- ICS abonelik / içe / dışa aktarma (`emersion/go-ical`)
- Etkinlik (VEVENT) ve görev (VTODO)
- **Yinelenme**: RRULE + RDATE + EXDATE + tekil istisnalar — en zor parça
- Hatırlatıcılar ve erteleme — M4 bildirim altyapısını kullanır
- Zaman dilimleri (Go `time/tzdata` gömülebilir)
- Kategoriler, görünümler (gün/hafta/çok hafta/ay), görev listesi, bugün paneli

### M9c — Davetler

M9b'den sonra, M6 gönderme altyapısına dayanır.

- iTIP işleme (REQUEST/REPLY/CANCEL/REFRESH/COUNTER)
- iMIP e-posta taşıması
- **Postada davet çubuğu** (Kabul / Belki / Reddet) — kurumsal kullanımda en
  görünür özellik
- Katılımcılar, RSVP, PARTSTAT
- Yinelenen öğede "bu / tümü" sorusu, çakışma çözümü

**Kabul:** M9a — şifreli bir mesaj gönderilip karşı tarafta Thunderbird ile
açılabiliyor. M9b — CalDAV sunucusundaki yinelenen bir etkinlik doğru
tekrarlarla görünüyor, hatırlatıcı zamanında çalıyor. M9c — gelen bir Outlook
daveti kabul edilip organizatöre yanıt gidiyor.

---

## M10 — Microsoft Graph / Exchange

"Outlook killer" iddiasının kurumsal ayağı. Posta zaten IMAP + OAuth ile
çalışıyor; kazanç takvim ve kişilerde.

- Microsoft Graph istemcisi: klasör hiyerarşisi, artımlı senkron, öğe işlemleri
- M365 takvimi ve kişileri
- GSSAPI / Kerberos, NTLM kimlik doğrulama
- Serbest/meşgul sorgusu
- Takvim yazdırma

**Kütüphane kararı:** `microsoftgraph/msgraph-sdk-go` v1.102.0 mevcut ama
devasa; ihtiyacımız olan uç nokta sayısı azken elle yazılmış bir REST
istemcisi daha muhtemel. Bu kilometre taşının planında karara bağlanır.
(Thunderbird bu işi C++ ve Rust karışımıyla yapıyor —
`mailnews/protocols/exchange/` + `rust/graph_xpcom/` — bizim için hazır bir
örnek değil.)

**Kabul:** bir M365 hesabının takvimi ve kişileri görünüyor; toplantı daveti
kabul edilebiliyor.

---

## M11 — Göç

Kullanıcı kazanmanın önündeki tek gerçek engel.

- Thunderbird / SeaMonkey profili: hesap, posta, kişi, ayar
- Outlook içe aktarma (Windows; MAPI + RTF gövde çözümü ayrı bir iş)
- Apple Mail (`.emlx`)
- mbox ve maildir okuma (`emersion/go-mbox`, `go-maildir`) — **yalnızca kaynak**;
  kendi depomuz SQLite kalır
- Kişi dosyaları: CSV (alan eşleme arayüzüyle), LDIF, vCard
- Filtre ve ayar içe aktarma
- **Profil dışa aktarma** — kilitlenme yok, veri çıkabiliyor

**Kabul:** gerçek bir Thunderbird profili içe aktarılıyor; mesaj sayısı,
klasör yapısı ve okundu durumları kaynakla birebir eşleşiyor.

---

## M12 — Yerelleştirme ve erişilebilirlik

**Bugün hiç i18n yok; tüm arayüz dizeleri İngilizce olarak koda gömülü.**
Hedef kitlenin bir kısmı Türkçe konuşurken bu sürdürülebilir değil.

- i18n altyapısı, tüm dizelerin dışarı çıkarılması
- Türkçe ve İngilizce; çalışırken dil değiştirme
- Bölgesel tarih ve sayı biçimleri
- Özelleştirilebilir klavye kısayolları
- Arayüz yoğunluğu (sıkışık/varsayılan/geniş), font boyutu
- Erişilebilirlik denetimi (axe), klavyeyle tam gezinme
- Oturum geri yükleme; düzen modları (klasik/geniş/dikey); kart görünümü

**Kabul:** arayüz Türkçeye çevrildiğinde hiçbir dize İngilizce kalmıyor;
uygulama yalnızca klavyeyle kullanılabiliyor; axe denetimi temiz.

---

## M13 — Sohbet

Bağımsız; istenirse ertelenebilir.

- Matrix (uçtan uca şifreli) — gizlilik konumlandırmasıyla en uyumlu protokol
- XMPP (SCRAM-SHA-256) + OTR
- Cihaz/oturum doğrulama — şifreleme varsa doğrulama zorunlu
- Yerel konuşma günlüğü
- Kişi listesi, gruplar, durum, bildirimler

IRC kapsam dışı: eski yük, şifreleme yok.

---

## M14 — Ürünleşme

- Otomatik güncelleme
- Kod imzalama (Windows, macOS)
- Kurulum paketleri, dağıtım

---

## Mimari sonuçlar

### Yeni arka uç arayüzleri

`imapx.MailBackend` deseni tekrarlanır: `carddavx.ContactsBackend` ve
`caldavx.CalendarBackend`. Graph/EWS (M10) bu arayüzlerin ikinci uygulaması
olarak girer — takvim ve kişi motorları da somut protokole değil arayüze
bağlanır. `depguard` kurallarına yeni katmanlar eklenir.

Bu, Thunderbird'ün de vardığı yer: `IExchangeClient.idl` arayüzünü hem EWS hem
Graph uyguluyor.

### Şema genişlemeleri

`internal/store/migrations/` altına:

| Tablo | Kilometre taşı |
|---|---|
| `identities` — hesap başına çoklu kimlik | M6 |
| `contacts`, `contact_emails`, `address_books` | M7 |
| `tags`, `message_tags` | M8a |
| `filters`, `filter_actions` | M8c |
| `calendars`, `calendar_items`, `attendees`, `alarms` | M9b |

**Giden posta:** ham MIME büyük olduğu için `operations.payload` içine değil,
diske (`outbox/`) yazılır; kuyrukta yalnızca referans durur.

**Zaten hazır olanlar:** `accounts.smtp_host/smtp_port`, `attachments` tablosu
(`local_path`, `downloaded`), `operations` kuyruğu ve `uid_validity` damgası,
`message_bodies`. M6'nın outbox'ı sıfırdan mekanizma icat etmiyor.

### Bağımlılıklar

cgo yasağı korunur. Hepsi saf Go ve varlığı doğrulandı:

| İş | Paket | Sürüm | KT |
|---|---|---|---|
| SMTP | `emersion/go-smtp` | v0.25.0 | M6 |
| CalDAV / CardDAV | `emersion/go-webdav` | v0.7.0 | M7, M9b |
| vCard | `emersion/go-vcard` | v0.1.0 | M7 |
| iCalendar | `emersion/go-ical` | 2025-06 | M9b |
| LDAP | `go-ldap/ldap/v3` | v3.4.14 | M7 |
| Bayes junk | `jbrukh/bayesian` | v1.1.0 | M8c |
| DKIM/SPF/ARC | `emersion/go-msgauth` | v0.7.0 | M9a |
| OpenPGP | `ProtonMail/go-crypto` | v1.4.1 | M9a |
| S/MIME (PKCS#7) | `go.mozilla.org/pkcs7` | v0.10.0 | M9a |
| mbox / maildir | `emersion/go-mbox`, `go-maildir` | v1.0.4 / v0.6.0 | M11 |

### Test altyapısı

Thunderbird IMAP, POP3, SMTP, NNTP, LDAP, EWS ve Graph için betiklenebilir
sahte sunucular taşıyor (`mailnews/test/fakeserver/`). Bizde `imapmemserver`
aynı işi görüyor; M6'da SMTP için `go-smtp`'nin sunucu tarafıyla aynısı
kurulur. Her yeni protokol kendi sahte sunucusuyla birlikte gelir.

---

## Bilerek kapsam dışı

Gerekçeleriyle birlikte [envanterde](../design/feature-inventory.md) yazılı.
Özet:

| Ne | Neden |
|---|---|
| POP3 | Sunucu tarafı durum yok; yerel-önce modelimizle çelişiyor |
| NNTP / haber grupları | Eski yük |
| RSS / Atom besleme hesapları | Tutarlı ama e-posta değil |
| IRC | Eski yük; şifreleme yok |
| Bulut ek (FileLink) | Üçüncü tarafa yükleme; gizlilik iddiasıyla çelişir |
| İşletim sistemi arama entegrasyonu | Postayı OS indeksine verir |
| Mozilla hesap senkronu | Ayarları sunucuya taşır; yerel-önce ile çelişir |
| **Telemetri** | Gizlilik odaklı bir istemci ölçüm göndermez |
| WebExtension eklenti API'si | Çok uzun vadeli; şimdi planlamak erken |
| Uygulama içi ürün bildirimleri | Pazarlama kanalı |

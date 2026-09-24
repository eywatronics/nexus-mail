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
| **M4** | Tepsi, bildirimler, arka plan yaşam döngüsü | Bitti |
| **M5** | Okuma deneyimini tamamla | Bitti |
| **M6** | Gönderme | Sürüyor |
| **M7** | Kişiler | Yeni |
| **M8** | Organizasyon, arama olgunluğu, otomasyon | Yeni |
| **M9** | Uçtan uca şifreleme + takvim | Yeni |
| **M10** | Microsoft Graph / Exchange | Yeni |
| **M11** | Göç (içe/dışa aktarma) | Yeni |
| **M12** | Yerelleştirme ve erişilebilirlik | Yeni |
| **M13** | Yerel-önce RAG ve posta zekâsı | Yeni |
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
- Görev çubuğu / dock rozetinde okunmamış sayısı
- Açılışta başlat (ayarlar ekranından)

**M4 bitti.** Son madde (`mailto:`) M6'ya taşındı — gerekçe aşağıda.

### `mailto:` neden M6'da

Varsayılan posta istemcisi olarak kaydolmak M4'ün son maddesiydi. Bugün
yapılsaydı **zararlı** olurdu: gönderme M6'da, yani bir `mailto:` bağlantısına
tıklayan kişi Nexus Mail'i açar ve hiçbir şey olmaz. Outlook'u varsayılan
bırakmaktan kötü bir sonuç.

Bir de işin Windows tarafı sanıldığı gibi değil: Windows 10'dan beri bir
uygulama kendini programatik olarak varsayılan **yapamıyor**. Yapılabilecek
olan, uygulamayı aday olarak kaydedip Ayarlar sayfasını açmak. Yani madde
"bir kayıt defteri yazımı" değil, "aday olarak görün + kullanıcıyı doğru
sayfaya götür + gelen `mailto:` URL'sini compose penceresine bağla" — ve
sonuncusu M6 olmadan yok.

### Açılışta başlat, işletim sisteminin ayarıdır

Diğerlerinden farklı olarak `config.json`'a **yazılmıyor**. Kayıt defteri
değeri, launch agent ya da desktop dosyası — nerede duruyorsa gerçek orası. Bir
kopyasını dosyaya yazmak, kullanıcı Görev Yöneticisi'nden kapattığı anda
dosyanın gerçekle çelişmesi ve bir sonraki açılışta dosyanın kazanması
demekti.

İki sonucu var: ayar **okunarak** gösteriliyor, ve kaydederken yalnızca
değiştiyse yazılıyor — aksi halde kullanıcının dışarıdan yaptığı değişiklik,
ilgisiz bir ayarı kaydettiğinde sessizce geri alınırdı.

Okunamadığında anahtar **kapalı değil, kullanılamaz** gösteriliyor. "Kapalı" ile
"bilmiyoruz" farklı cevaplar, ve ilkini göstermek ayar ekranının yalan
söylemesi olurdu.

### Rozet ipucunun yerine değil, yanına

`tray.go` şunu yazıyordu: *"Windows'ta tepsi ikonunun dock gibi bir rozeti
yok — bu yüzden sayının dürüst yeri metin."* Tepsi için hâlâ doğru, ama **görev
çubuğu düğmesinin** rozeti var; Wails bunu macOS dock'uyla aynı API üzerinden
sunuyor. İkisi tek fonksiyondan besleniyor, yoksa aynı ekranda iki farklı sayı
görünebilirdi.

99'da duruyor: on altı piksellik bir dairede üç hane okunmuyor, ve dört yüz
okunmamışta "kaç tane" sorusunun cevabı zaten "şu an okuyacağından fazla".
Sıfırda rozet çizilmiyor — sıfır, fark edilecek bir şey olmadığını söyleyen bir
işaret olurdu.

### Görev çubuğu ilerlemesi ve Jump List neden yapılmadı

Envanterde "görev çubuğu ilerlemesi, Jump List, dock rozeti" tek satırdı.
Üçüncüsü yapıldı, ilk ikisi **bilerek yapılmadı**.

İlerleme çubuğu uzun bir işlem ister. Buradaki tek aday ilk senkron, ve onun
ilerlemesi zaten pencerede görünüyor; çubuk, pencere kapalıyken görülmeyen bir
işlemin göstergesi olurdu. Jump List ise henüz olmayan eylemleri listeler —
"yeni mesaj" M6'da gelecek. İkisi de Thunderbird'de olduğu için değil, burada
karşılığı olduğu için yapılmalı; bugün yok.

**Doğrulanmayan:** toast'ın gerçekten teslim edildiği, rozetin gerçekten
çizildiği ve açılışta başlatmanın gerçekten çalıştığı. Üçü de çalışan bir Wails
uygulaması gerektiriyor; mantık testli, işletim sistemi tarafı değil. Pencere
kapatmanın süreci öldürmediği ise çalışan sürece `WM_CLOSE` gönderilerek
doğrulandı.

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
- ~~Mesaj gövdesinde karanlık mod~~ — **bitti**; tema çerçeveye URL'de söyleniyor,
  çünkü sandbox'lı belge ana sayfadaki sınıfı göremiyor
- ~~Silmek çöp kutusuna taşısın~~ — **bitti** (aşağıya bakın)
- ~~Geri al~~ — **bitti** (tek adım, yıkıcı işlemler için)
- ~~Okundu işaretleme davranışı~~ — **bitti** (açınca / birkaç saniye sonra / hiç)
- ~~Mesajda bul~~ — **bitti** (aşağıya bakın)
- ~~Yinele (redo)~~ — **bitti** (aşağıya bakın)

**M5 bitti.**

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

Geri alma tek adım, ve öyle kalıyor: eskiye uzanan bir yığın, sunucunun aradaki
girdilere yetişmiş olmasıyla baş etmek zorunda kalırdı.

### Yinele

Yinele, iptal edilen işlemi tekrar oynatmıyor; eylemi baştan yapıyor. Yinelenen
bir silme `DeleteMessages`'tan geçiyor, dolayısıyla kendi sırası geldiğinde
geri alınabiliyor, kendi kuyruk kaydını alıyor, ve hesabın artık bir çöp
kutusu olup olmadığını yeniden kendisi buluyor. İşlemi oynatmak üçünü de
atlardı.

**Bir tuzak vardı:** geri alma mesajı eski satırına koymuyor. `id` sıradan bir
`INTEGER PRIMARY KEY`, yani SQLite `max(rowid)+1` veriyor ve tablodaki en yeni
satır olmayan bir mesaj başka bir numarayla geri geliyor. Eski numarayı taşımak
kaybetmekten kötü olurdu: numara geçersiz olmuyor, **boşa çıkıyor** ve sonraki
gelen mesaja verilebiliyor. Bu yüzden `RestoreMessages` artık geri koyduğu
satırların kimliklerini döndürüyor. (Tek mesajlı bir testte kimlik korunuyormuş
gibi görünüyor — rowid yeniden kullanılıyor — o yüzden test ikinci bir mesajla
kuruluyor.)

Yinele, geri almayla **aynı pencerede** sönüyor. Bu bir mekanizma değil karar:
geri almanın süresi kuyruğun değişikliği aldığı an, yinelemenin kendine ait bir
süresi yok. Ama ikisi de bir tereddüt anı, ve on dakika sonra hâlâ canlı bir
yinele, okuyucunun çoktan tutmaya karar verdiği postayı sessizce silen bir tuş
olurdu.

Şerit geri alma alınınca kaybolmuyor, tersine dönüyor: refleksle geri alıp
sonra fikir değiştiren okuyucu, bu şeridin var olduğu okuyucunun bir adım
sonrası. Ctrl+Shift+Z ve Ctrl+Y ikisi de yinele, çünkü platformlar anlaşamıyor
ve insanlar ilk öğrendikleri alışkanlığı taşıyor.

### Silme artık yok etmiyor

Delete, UID EXPUNGE demekti: mesaj sunucudan gidiyor, bulunacağı bir çöp
kutusu olmadan. Artık çöp kutusuna taşıyor; çöp kutusunun kendisinde silmek
yok ediyor, yoksa kutu hiç boşaltılamazdı.

Çöp kutusu **role göre** bulunuyor, ada göre değil — SPECIAL-USE işinin ilk
gerçek karşılığı.

Kalıcı silmede onay soruluyor — yalnızca çöp kutusunun içinde, çünkü her yerde
sormak insanlara soruyu okumadan kapatmayı öğretir.

**Çöp kutusunu boşaltma** ayrı bir işlem türü (`empty_folder`) olarak yapıldı.
UID adlandırmıyor, ve adlandırmaması doğru olan şey: yerel satırlar klasörün
yalnızca indirilmiş kısmı, dolayısıyla bizim listemizden kurulan bir boşaltma
sekiz bin mesajlık bir kutuyu yüz mesaj silerek "boşaltılmış" gösterirdi.
İstemcideki tek posta kutusu çapında EXPUNGE budur.

Bilerek geri alınamaz: tam senkronlanmamış bir klasörün anlık görüntüsü orada
olanın bir kesrini geri koyardı.

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

### Mesajda bul, tarayıcının değil bizim

Ctrl+F tarayıcının kendi aramasını açmıyor, açamaz da: okuma paneli
`allow-scripts` ve `allow-same-origin` taşımayan bir iframe, yani içine
girecek betik yok ve dışarıdan tutamak da yok. Tarayıcının aramasını olduğu
gibi bırakmak, okunan mesajda hiçbir zaman eşleşme bulmayan bir Ctrl+F demek
olurdu — yazdırmayla aynı duvar.

Bu yüzden arama markup'a geliyor: sorgu gövde URL'sinde gidiyor,
`mailhtml.Highlight` eşleşmeleri belge sunulmadan önce `<mark>` ile sarıyor,
ve geçerli eşleşmeye `#nx-find-current` parçasıyla kaydırılıyor. Parça, betiği
olmayan bir belgeyi kaydırmanın tek yolu.

Üç sonucu var, üçü de bilerek:

- **"Hepsini vurgula" anahtarı yok.** Her eşleşme her zaman işaretli, çünkü
  işaretlemek aramanın kendisi.
- **Eşleşme öğe sınırını aşmıyor.** Ortasından `<b>` geçen bir kelime
  bulunmuyor. Tarayıcının kendi araması bunu yapıyor; ona ulaşmanın bedeli
  çerçeveye betik vermek, ki bu işin var oluş sebebini iptal eder.
- **Sayım olaydan geliyor, ikinci bir çağrıdan değil.** Sayı, belgeyi kuran
  aynı geçişte düşüyor; ayrıca sormak mesajı iki kez render etmek ve ikisinin
  ayrışma ihtimalini kabul etmek olurdu.

Vurgu sarı değil, uygulamanın kendi accent'i: bu bir seçim ve accent tam da
bunun için var. Sarı, pencereyi kaplayan tek panelde ikinci bir accent olurdu.

Büyük/küçük harf ayrımı yok. Türkçe'de basit kat sıralamanın sınırı görünüyor:
`I` ve `İ` ikisi de `i`'ye katlanıyor, `ı` ise kendisine — yani "istanbul"
araması "İstanbul"u da buluyor, "ışık" yalnızca "ışık" ile bulunuyor. Doğrusu
için okuma panelinin sahip olmadığı bir yerel ayar gerekiyor; davranış testle
sabitlendi.

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

- ~~SMTP (`internal/smtpx`)~~ — **bitti**: STARTTLS zorunlu, 8BITMIME, SIZE,
  SMTPUTF8, PLAIN/LOGIN. XOAUTH2 hesap bağlamayla birlikte gelecek
- ~~Giden mesajı kurma (`internal/mailmime`)~~ — **bitti**: yapı, başlıklar,
  ekler, quoted-printable
- ~~Hesap başına çoklu kimlik~~ — **bitti** (migration 004, `identities`)
- **Varsayılan posta istemcisi (`mailto:`)** — M4'ten taşındı; compose penceresi
  olmadan kaydolmak, tıklayana hiçbir şey yapmayan bir uygulama vaat etmek olurdu
- ~~İmzalar (kimlik başına metin/HTML)~~ — **bitti**; dosyadan imza yok
- ~~`SendMessage` servisi~~ — **bitti**: adres ayrıştırma, imza, kur, kuyruğa al
- Compose penceresi: yanıtla / tümünü / listeye / ilet / yönlendir / yeni olarak düzenle
- Zengin metin editörü (`contenteditable`) ve düz metin kipi
- Alıntılama ve yanıt konumu
- Alıcı "pill" arayüzü + otomatik tamamlama (toplanan adreslerden başlar)
- Ek ekleme, gömülü resim (`cid:`), **ek hatırlatıcı**
- Taslak otomatik kaydetme
- ~~**Outbox**~~ — **bitti**: ham MIME diske, kuyrukta referans, SMTP ile
  boşaltma, kalıcı/geçici hata ayrımı
- Fcc — gönderilen kopyayı Gönderilenler'e yazma (UIDPLUS ile UID öğrenme)
- Otomatik yapılandırma (ISPDB, DNS MX/SRV, tahmin) — hesap eklemeyi üç adımdan bire indirir
- `mailto:` işleyicisi

### Outbox neden motoru değiştiriyor

Kuyruk işçisi (`applyOperation`) bugün yalnızca `imapx.MailBackend` alıyor,
çünkü bugüne kadar her işlem IMAP işlemiydi. Gönderme öyle değil: SMTP
bağlantısı istiyor, klasöre bağlı değil, ve `uid_validity` damgası taşımıyor —
üstelik gönderdikten sonra kopyayı Gönderilenler'e yazmak için **yine IMAP**
gerekiyor. Yani `send` işlemi tek başına iki protokole dokunuyor.

Motora ikinci bir bağlayıcı (`SetSender`) ve `send` için ayrı bir boşaltma yolu
eklendi. Ham MIME `operations.payload` içine değil diske yazılıyor; kuyrukta
yalnızca referans duruyor, çünkü yirmi megabaytlık bir ek bir metin sütununa
konacak şey değil.

Kuyruk **bağlantı açılmadan** ikiye ayrılıyor: yalnızca gönderme bekleyen bir
hesap posta kutusu açmıyor, yalnızca bayrak değişikliği bekleyen bir hesap da
gönderim bağlantısı açmıyor.

**Kalıcı/geçici ayrımı `smtpx`'te**, çünkü bilgi orada: SMTP yanıt kodu bunu
söylüyor (5xx red, 4xx "şimdi değil") ve paketin kendi ön redleri `ErrRefused`
sarıyor. Tanınmayan her şey geçici sayılıyor — sunucunun aslında reddetmediği
bir mesajdan vazgeçmek, iki hatanın kötüsü: kullanıcı gönderildiğini sanır.

**Kapatılamayan bir pencere var ve yazıldı:** mesaj gidip "done" yazımı
başarısız olursa kuyruk hâlâ "bekliyor" der ve bir sonraki geçiş aynı mesajı
tekrar gönderir. Düzgün kapatmak, göndermeyle kaydın birlikte commit olmasını
gerektirir; SMTP bunu sunmuyor. Dürüst hafifletme, boşaltmayı orada
durdurmak — kalan mesajları aynı arızanın içine sürmemek.

### Gönderme kuyruğa alır, beklemez

`SendMessage` mesajı kurar, diske yazar, kuyruğa koyar ve döner. Pencere bir
dosya yazma süresinde cevap alıyor; mesaj bağlantı izin verince gidiyor.
Senkron göndermek, bir TLS el sıkışması ve bir yükleme boyunca donan bir
composer demek olurdu — ve kullanıcı kapatırsa kaybolan bir mesaj.

**Dosya önce, kuyruk satırı sonra.** Ters sıra, var olmayan bir dosyayı
adlandıran bir satır bırakırdı ve işçinin bunu çözmenin tek yolu mesajı kalıcı
olarak başarısız saymak olurdu. Satırsız bir dosya ise sonradan süpürülüyor ve
disk dışında bir maliyeti yok.

**Ayrıştırılamayan adres reddediliyor, atlanmıyor.** Dört alıcının üçüne
sessizce göndermek, dördüncü kişi neden dışarıda bırakıldığını sorana kadar
kimsenin fark etmediği türden bir arıza.

**OAuth ile gönderme henüz yok.** XOAUTH2 gerekiyor; go-sasl bunu sunmuyor ve
`imapx` IMAP için elle uygulamış. O uygulamayı paylaşmak kendi başına bir iş ve
buraya sıkıştırılmak yerine M6'nın geri kalanıyla ele alınacak. Hata mesajı
bunu adlandırıyor.

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

## M13 — Yerel-önce RAG ve posta zekâsı

Uzun zincirleri özetleyen, yazışmalardan doğal dille bilgi çıkaran, yanıt
taslağı üreten bir katman. İki kip: makinede çalışan yerel model (Ollama) ya
da kullanıcının kendi anahtarıyla bulut sağlayıcı (BYOK).

Ayrıntılı plan: [m13-intelligence-rag.md](m13-intelligence-rag.md).

- Sağlayıcı katmanı (`internal/ai`), anahtarlar OS anahtarlığında
- Zincir özeti, akan yanıt
- `mail_chunks` + saf Go vektör arama, FTS5 ile hibrit (RRF)
- Gelen kutusu soru-cevap, kaynak mesaj kartlarıyla
- Taslak asistanı (M6'ya bağlı tek parça)

**Varsayılan kapalı**, ve yerel kipte tek bayt makineden çıkmaz.

### Bu, bir kuralın bilinçli istisnası

Aşağıdaki "bilerek kapsam dışı" tablosunda **işletim sistemi arama
entegrasyonu** "postayı OS indeksine verir" diye reddedilmişti. Bulut kipi
daha ileri gider: gövdenin tamamı üçüncü tarafa iner.

Fark rızanın olup olmaması değil, **görünür ve dönülebilir olması**: veriyi
veren kullanıcının kendisi, kendi anahtarıyla, hangi mesaj için olduğunu
görerek. OS entegrasyonunda veriyi veren uygulamaydı ve kullanıcı bunu
göremiyordu.

Bunu tabloya yazmadan geçmek, projenin kendi gerekçeleriyle çelişmek olurdu.

### Sohbet nereye gitti

M13 daha önce Sohbet'ti (Matrix + XMPP). Kapsamdan çıkarıldı; aşağıdaki
"bilerek kapsam dışı" tablosuna geçti.

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
| **Sohbet (Matrix, XMPP, IRC)** | E-posta istemcisi olmanın asgari şartı değil ve dört iddiadan hiçbirini desteklemiyor. Önce M13, sonra M15 olarak planlandı; ikisinde de kendinden önceki posta işlerinin arkasında duruyordu — bu "hiçbir zaman" demenin uzun yolu. Kısa yolu burası |
| Bulut ek (FileLink) | Üçüncü tarafa yükleme; gizlilik iddiasıyla çelişir |
| İşletim sistemi arama entegrasyonu | Postayı OS indeksine verir |
| Mozilla hesap senkronu | Ayarları sunucuya taşır; yerel-önce ile çelişir |
| **Telemetri** | Gizlilik odaklı bir istemci ölçüm göndermez |
| WebExtension eklenti API'si | Çok uzun vadeli; şimdi planlamak erken |
| Uygulama içi ürün bildirimleri | Pazarlama kanalı |

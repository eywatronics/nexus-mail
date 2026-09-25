# Konumlandırma: bu istemci kimin için

Bu belge bir pazarlama metni değil. Dışarıdan gelen sert bir rakip
değerlendirmesinin kayda geçmiş hâli ve ona verilen cevap: hangi boşluk
**bilerek** açık, hangisi kapanmak zorunda, ve hangisi kapanana kadar bu
yazılım kimin eline verilmemeli.

Değerlendirmede ölçülmüş şeyle iddia edilen şey karışmış hâlde geliyordu. Burada
ayrıştırılıyor.

---

## Ölçülmüş olan

Yalnızca şu sayılar bu depoda üretildi ve tekrar üretilebilir:

| | |
|---|---|
| Arama, 25.000 mesajlık kutuda, isabet | 22 ms |
| Aynı kutuda ıska | 5 ms |
| Terimin 25.000 mesajın hepsinde geçtiği patolojik durum | 48 ms |
| Liste ilk sayfa | 0,8 ms |
| Senkron 500'lük grupları yazarken en yavaş okuma turu | 55 ms |

Kaynak: `internal/store/search_bench_test.go`, `internal/store/concurrency_test.go`.
Tek makinede, sıcak sayfa önbelleğiyle. Soğuk disk daha yavaş olur.

## Ölçülmemiş olan

Değerlendirmedeki puan tablosu (`Outlook arama 3/10`, `Nexus gizlilik 10/10`
gibi) kanaat. Rakiplerin hiçbiri bu depoda ölçülmedi. Kendi tarafımızdaki
"gizlilik 10/10" da bir iddia: doğru olan, **hangi kuralın kod tarafından
zorlandığı** — parolaların yalnızca OS anahtarlığına gitmesi, logların mail
içeriği taşımaması, `internal/ai` içinde yalnızca `provider` alt paketinin ağa
çıkabilmesi, hepsi `depguard` ve testlerle tutuluyor. Puan değil, kural.

Bu ayrım önemli: kendi lehimize olan bir tabloyu ölçülmüş gibi sunmak,
eleştirinin kendisini işe yaramaz hâle getirir.

---

## Bilerek açık bırakılan boşluklar

Değerlendirmenin sonucu, kendi eleştirilerinin bir kısmıyla çelişiyor: hem
"takvim ve Exchange olmadan kurumsal dünyada ölü doğmuştur" diyor, hem de
"İsviçre çakısı olmaya çalışma, Japon kılıcı ol" diyor. İkisi aynı anda doğru
olamaz — **birincisi ancak kurumsal dünyayı hedeflersen bir kusur.**

Karar: hedef kitle kurumsal masaüstü değil. Dolayısıyla aşağıdakiler eksik
değil, **kapsam dışı** — ve bu kapsam dışılığın bir bedeli var, yazılı olması
gereken de o bedel.

### Ortak posta kutusu (`info@`, `destek@`)

Beş kişinin aynı gelen kutusunu, bayraklarını ve kategorilerini eşzamanlı
yönetmesi Outlook'un en olgun özelliklerinden biri. Burada bir cevabı yok ve
yakın planda olmayacak.

**Bedeli:** bir ekip aracı olarak kullanılamaz. Bu yazılım bir kişinin kendi
postasını okuduğu yer.

**Bunu ne değiştirir:** IMAP ACL ve paylaşımlı klasör desteği tek başına
yetmez; asıl iş bayrak çakışmalarının uzlaştırılması. Tek kullanıcı varsayan
işlem kuyruğu (`internal/store` operations) bunun için yeniden düşünülmeyi
gerektirir. Kuyruk yeniden yazılmadan bu özelliği eklemek, sessizce yanlış
davranan bir istemci üretir — hiç olmamasından kötü.

### Derin Exchange entegrasyonu

M10'da Microsoft Graph var ve o gerçek. Ama şirket içi Exchange'in NTLM'i,
kendi imzaladığı sertifikaları ve MAPI'si kapsam dışı.

Bir düzeltme: değerlendirmedeki *"NTLM olmadan Exchange'e giremiyorum"*
şikâyeti bugünün Exchange Online'ı için geçerli değil — Microsoft orada temel
kimlik doğrulamayı kapattı, geçerli yol OAuth ve o bizde var (`smtpx` ve
`imapx`, XOAUTH2). Şikâyet şirket **içi** Exchange kurulumları için doğru.

---

## Kapanmak zorunda olan boşluklar

Bunlar kapsam dışı değil; hedef kitlenin kendisini de engelliyorlar.

### 1. BYOK OAuth duvarı

Bugün `config.json` içinde bir `googleClientId` yoksa Google hesabı
eklenemiyor, ve gömülü bir varsayılan yok (`internal/app/service.go`,
`oauthConfigFor`). Kullanıcıdan Google Cloud Console'da proje açması isteniyor.

Bu, gizlilik meraklısı bir geliştirici için kabul edilebilir; **Linux kullanan
ama geliştirici olmayan** biri için aşılmaz. Ve hedef kitle onları da
kapsıyor.

Üç yol var ve üçü birbirini dışlamıyor:

1. **GNOME Online Accounts (Linux).** Ayrı başlık, aşağıda.
2. **Uygulama parolası.** Google ve Microsoft hâlâ, iki adımlı doğrulama açık
   hesaplarda IMAP için uygulama parolası üretmeye izin veriyor. Tek tık
   değil ama Cloud Console'dan çok daha kısa bir yol, ve zaten desteklenen
   parola yoluna düşüyor. Ekranda anlatılması gereken bir şey, kod değil.
3. **Doğrulanmış bir OAuth istemcisi.** Gerçek çözüm bu. Bedeli bürokrasi ve
   masraf: Google'ın kısıtlı kapsam (`restricted scope`) doğrulaması, yıllık
   güvenlik denetimi ve bir tüzel kişilik istiyor. M14'ün (ürünleşme) parçası.

**Sıralama kararı:** 2 önce, çünkü bugün yazılabilir ve kimseye bağlı değil.
Sonra 1. 3 en sona, çünkü tek kod dışı iş o.

### 2. İmzasız derlemeler

Windows SmartScreen'in kırmızı ekranı ve macOS'un "geliştirici doğrulanamadı"
reddi, ortalama kullanıcı için virüs uyarısıyla eşdeğer. README bunu dürüstçe
söylüyor ama dürüstlük indirmeyi geri getirmiyor.

Bu da M14. Kod imzalama sertifikası ve Apple geliştirici hesabı, yine tüzel
kişilik ve para. Ama Linux tarafında karşılığı **bugün** var: `.rpm` ve `.deb`
paketleri imzalanabilir ve bir depo (repo) yayınlanabilir, ikisi de kimseden
onay istemez.

### 3. Protokol uç durumları

Değerlendirmenin en yerinde maddesi bu, çünkü tek panzehiri zaman:
Thunderbird'ün 25 yılda yamadığı bozuk sunucular. Temiz bir IMAP standardı
varsaymak, ilk bin kullanıcıda GitHub'ı alevlendirir.

Yapılabilecek olan, kusurları önceden tahmin etmek değil — **arızanın
kullanıcıdan bize okunabilir biçimde ulaşmasını sağlamak.** Bugün eksik olan:

- Parolasız, içerik taşımayan bir protokol günlüğü (kullanıcı açar, IMAP
  komutlarını ve sunucu cevaplarını görür, dosyayı issue'ya ekler). Gizlilik
  kuralı gereği konu, gövde ve adres **girmeyecek** biçimde tasarlanmalı;
  bunun kendisi bir iş.
- Sunucu davranışlarının kayda geçtiği bir uyumluluk tablosu.

Bu, M14'ten önce gelmeli: hata raporu alamadan uç durum yamalanamaz.

### 4. Takvim

M9'da. Değerlendirmenin "ölü doğmuştur" hükmü kurumsal kullanıcı için doğru,
hedef kitle için abartılı — ama `.ics` davetinin **okunabilir görünmesi** ayrı
bir şey ve daha ucuz: bugün bir toplantı daveti, okuyucuda anlamsız bir ek
olarak duruyor. Tam CalDAV senkronu M9'da kalsın; davetin ne olduğunu gösteren
bir okuma görünümü M8'e alınabilir. Kabul/ret yanıtı göndermek zaten bir
`METHOD:REPLY` maili — gönderme M6'da bitti.

---

## GNOME Online Accounts: Linux'ta BYOK duvarını atlamak

Öneri şu: GNOME kullanıcısı Google veya Microsoft hesabına zaten sistem
Ayarlar'ından girmiş durumda. `org.gnome.OnlineAccounts` D-Bus servisine bağlanıp
o hesabın erişim token'ını istemek, kullanıcıdan hiçbir şey istemeden Gmail'e
bağlanmak demek.

**Doğru ve bu projeye özellikle iyi oturuyor.** Ama dört kayıt düşülmesi
gerekiyor, çünkü öneri olduğu gibi alınırsa yanlış beklenti kurar.

**Bir: saf Go kalıyor — ölçüldü.** `github.com/godbus/dbus/v5` D-Bus tel
protokolünü kendi uygulayan saf Go bir kütüphane. Boş bir modülde deneyip
`CGO_ENABLED=0 GOOS=linux` ile çapraz derledim: derleniyor, ve bağımlılık
ağacının tamamında tek bir cgo dosyası yok (yalnızca `golang.org/x/sys`
ekleniyor). Yani zorunlu cgo yasağıyla çakışmıyor. Bu kontrol önemliydi:
çakışsaydı öneri en baştan ölürdü.

**İki: "Linux" değil, "GNOME".** KDE'nin karşılığı KAccounts, ayrı bir servis.
Ne GNOME ne KDE çalıştıran bir kullanıcı için yol yine BYOK. Özelliğin adı ve
arayüzdeki metni bunu söylemeli; "Linux'ta tek tık" demek, Xfce kullanıcısına
yalan olur.

**Üç: Flatpak.** Sandbox içinde rastgele D-Bus adlarına erişim yok; ya izin
verilmesi ya da bir portal gerekiyor. Dağıtım biçimi Flatpak olacaksa bu
manifest meselesi, ve önce çözülmesi gerekir.

**Dört: kapsam.** GOA hesap başına "Mail" anahtarı tutuyor. Kapalıysa basılan
token IMAP/SMTP kapsamını taşımaz ve bağlantı, sebebi anlaşılmaz biçimde
reddedilir. Kullanıcıya "GNOME Ayarları'nda bu hesap için Posta'yı açın"
diyebilmek, özelliğin kendisi kadar iş.

### Mimari

Proje kuralı net: yeni bir arka uç arayüz üzerinden bağlanır ve `depguard`
kuralı yazılır — *kural yazılmamışsa mimari kural yok demektir.*

Buranın şansı, arayüzün **zaten var olması**: `auth.CredentialProvider`. GOA,
üçüncü bir sağlayıcı — parola ve OAuth'un yanına. `SASLClient` çağrıldığında
D-Bus üzerinden token istiyor, `Refresh` yenilemeyi GNOME'a bırakıyor. `imapx`
ve `smtpx` hiçbir şey öğrenmiyor; ikisi de bu turda aynı dikişe getirildi.

| Katman | Sorumluluk | Arayüz |
|---|---|---|
| `internal/auth/goa` | GNOME Online Accounts, D-Bus | `auth.CredentialProvider` |

Yazılması gereken `depguard` kuralı: `godbus` yalnızca bu pakette import
edilebilir. Gerekçesi mimari değil taşınabilirlik — D-Bus çağrısı Linux dışında
anlamsız ve derleme etiketiyle ayrılmış tek bir dizinde durması, Windows
derlemesinde ne olduğu sorusunun tek dizinlik bir cevabı olması demek.

Kural **şimdi yazılmadı**, bilerek. Bu depoda her `depguard` kuralının
gerçekten tetiklendiği geçici bir dosyayla kanıtlanıyor; `godbus` henüz
`go.mod`'da olmadığı için kanıtlanamayacak bir kural yazmak, kural yazmış gibi
görünüp hiçbir şey tutmamak olurdu. Paketle birlikte gelir ve o zaman
kanıtlanır.

---

## Özet karar

Bu yazılım Outlook'un yerine geçmeye çalışmıyor; çalışsaydı takvim, ortak kutu
ve Exchange olmadan çalışamazdı. Hedef, hızlı ve gizliliği suistimal etmeyen
bir **tek kişilik** istemci.

Bu hedefi bugün engelleyen şey teknik değil: BYOK duvarı, imzasız derlemeler ve
hata raporu alamamak. Üçü de M14'e yığılmış durumdaydı; ilk ikisinin bir
bölümü ve üçüncüsü tamamen öne çekiliyor.

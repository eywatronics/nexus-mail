<div align="center">

# Nexus Mail

**Yerel-önce, gizlilik odaklı, gecikmesiz masaüstü e-posta istemcisi.**

Go · Wails v3 · React · SQLite

[![CI](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml/badge.svg)](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml)
[![Release](https://github.com/eywatronics/nexus-mail/actions/workflows/release.yml/badge.svg)](https://github.com/eywatronics/nexus-mail/actions/workflows/release.yml)
[![License: GPL v3](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8.svg)](https://go.dev)

</div>

---

## Neden

Modern e-posta istemcilerinin çoğu tarayıcıyı masaüstüne paketliyor. Sonuç
tanıdık: yarım gigabaytlık bellek kullanımı, her tıklamada ağ beklemesi ve
internet kesildiğinde işe yaramaz hale gelen bir uygulama.

Nexus Mail farklı bir kural üzerine kurulu: **veri önce diskte, sonra ağda.**
Mailleriniz yerel bir SQLite veritabanında durur. Okumak, listelemek ve
filtrelemek hiçbir zaman sunucuyu beklemez. İnternet yokken uygulama tam
çalışır; yaptığınız değişiklikler kuyruğa alınır ve bağlantı geri geldiğinde
uygulanır. Çevrimdışı çalışma sonradan eklenmiş bir özellik değil, mimarinin
doğal sonucu.

## Ayırt eden dört şey

**Ağ beklemesi yok.** Arayüz her şeyi yerel veritabanından çizer. Klasör
değiştirmek, listeyi kaydırmak ve daha önce açılmış bir maili okumak
çevrimdışıyken de anında çalışır.

**İzleyiciler varsayılan olarak engelli.** Uzak resimler ve CSS kaynakları,
siz istemedikçe yüklenmez — ve yalnızca `<img src>` değil, `background`
öznitelikleri, `srcset` ve CSS `url()` çağrıları da. İzin verdiğinizde
istekler uygulama üzerinden geçer, böylece IP adresiniz ve `Referer` başlığınız
gönderene ulaşmaz.

**Gerçek izolasyon.** Mail içeriği, `allow-same-origin` verilmemiş bir
`<iframe sandbox>` içinde ve kısıtlayıcı bir içerik güvenlik politikası
altında render edilir. Temizleme (sanitization) buna ek katmandır, alternatifi
değil.

**Kimlik bilgileri işletim sisteminin anahtarlığında.** Parolalar ve OAuth
token'ları veritabanına ya da yapılandırma dosyasına asla yazılmaz; Windows
Credential Manager, macOS Keychain veya Linux Secret Service üzerinde durur.

## İndir

Her `main` birleşmesi üç platform için kurulum dosyası üretir. En son yapı:
**[Releases](https://github.com/eywatronics/nexus-mail/releases)**.

| Platform | Dosya |
|---|---|
| Windows | `*-installer.exe` kurulum, `nexus-mail-windows-amd64.exe` taşınabilir |
| macOS | `nexus-mail-macos-arm64-unsigned.zip` (Apple silicon), `...-amd64-...` (Intel) |
| Linux | `*.AppImage` her dağıtımda, `*.rpm` Fedora/RHEL, `*.deb` Debian/Ubuntu |

**macOS ve Windows yapıları imzasız.** macOS, Sistem Ayarları → Gizlilik ve
Güvenlik'ten izin verene kadar açmayı reddeder; Windows SmartScreen bilinmeyen
yayıncı uyarısı gösterir. Kod imzalama M14'te; o zamana kadar imzasız bir
yapının dürüst hâli bu.

Linux paketleri GTK 4 ve WebKitGTK 6.0 gerektiriyor; `.rpm` ve `.deb` bunu
bağımlılık olarak bildiriyor, AppImage paketlemiyor.

## Durum

Geliştirme aşamasında, ama artık günlük kullanılabilir bir okuma istemcisi.

### Bugün ne yapıyor

**Hesaplar.** Parola veya uygulama parolasıyla herhangi bir IMAP sunucusu;
Google ve Microsoft için OAuth 2.0 (XOAUTH2); şirket içi Exchange için
143/STARTTLS. Bağlantı güvenliği hesap başına seçilir ve şifresiz seçenek
yoktur. Sunucunun sunduğu kimlik doğrulama mekanizması pazarlıkla seçilir
(PLAIN → SASL LOGIN → LOGIN), ve hiçbiri tutmazsa hata mesajı sunucunun ne
sunduğunu adlandırır.

**Senkron.** İlk senkron, IMAP IDLE ile canlı güncelleme, CONDSTORE destekleyen
sunucularda delta senkron, ve veritabanının sınırsız büyümesini durduran bir
saklama penceresi (klasör başına 365 gün / 25.000 mesaj; yıldızlılar muaf,
silme yalnızca yerelde).

**Okuma.** Üç sütunlu sanallaştırılmış liste, konuşma gruplama, FTS5 araması
(Türkçe'nin noktasız ı'sı dahil), klavye navigasyonu, ekleri listeleme ve
indirme, kaynağı görüntüleme, `.eml` kaydetme, yanlış beyan edilmiş kodlamayı
onarma, ve üç gövde görüntüleme kipi (özgün HTML / sade HTML / düz metin).

**Yazma.** Okundu, yıldız, taşı ve sil anında görünür, kuyruğa alınır ve
bağlantı geldiğinde sunucuya gider. Silmek çöp kutusuna taşır; çöp kutusunun
içinde sorar ve yok eder. Çöp kutusunu boşaltma, senkronlanmamış mesajları da
kapsayan ayrı bir sunucu işlemidir. Son yıkıcı işlem beş saniye boyunca geri
alınabilir (Ctrl+Z).

**Arka plan.** Pencere kapanınca uygulama tepside kalır ve senkron sürer; yeni
mail geldiğinde işletim sistemi bildirimi gönderir, içeriği isteğe bağlı
(kilit ekranı için).

**Ayarlar.** Tema, gövde kipi, okundu işaretleme davranışı, konuşma gruplama,
bildirim önizlemesi, geri alma penceresi, saklama limitleri ve OAuth client
ID'leri uygulama içinden.

### Henüz yok

**Gönderme** (M6) — okuma istemcisidir, yanıt yazılamaz. **NTLM/GSSAPI** (M10)
— temel kimlik doğrulamayı kapatmış kurumsal sunucular bağlanamaz.
**Yazdırma**, sandbox'lı okuma paneli yüzünden ayrı bir pencere gerektiriyor ve
M6'ya ertelendi. **Kod imzalama** (M14).

| Kilometre taşı | Kapsam | Durum |
|---|---|---|
| **M1** | Hesap bağlama (3 yol), klasör ve başlık senkronu, izole okuma, arama, klavye navigasyonu | Bitti |
| **M2** | IMAP IDLE ile canlı senkron, delta senkron, saklama penceresi | Bitti |
| **M3** | Çevrimdışı dayanıklı durum yazma (okundu, yıldız, taşı, sil) | Bitti |
| **M4** | Sistem tepsisi, bildirimler, arka planda çalışma | Kısmen bitti |
| **M5** | Ekler, konuşma gruplama, kaynak/kaydet, kodlama onarımı, gövde kipleri | Büyük ölçüde bitti |
| **M6** | Gönderme: SMTP, çoklu kimlik, imza, taslak, outbox, composer | Planlandı |
| **M7** | Kişiler: yerel defter, vCard, CardDAV, LDAP | Planlandı |
| **M8** | Etiket, arşiv, birleşik gelen kutusu, gövde araması, kural motoru, junk | Planlandı |
| **M9** | OpenPGP/S-MIME; CalDAV takvim ve toplantı davetleri | Planlandı |
| **M10** | Microsoft Graph: M365 takvim ve kişileri | Planlandı |
| **M11** | Thunderbird / Outlook / Apple Mail'den içe aktarma | Planlandı |
| **M12** | Türkçe arayüz, erişilebilirlik, özelleştirilebilir kısayollar | Planlandı |
| **M13** | Sohbet: Matrix ve XMPP, uçtan uca şifreli | Planlandı |
| **M14** | Otomatik güncelleme, kod imzalama, kurulum paketleri | Planlandı |

Yol haritasının nasıl çıkarıldığı, hangi özelliğin neden kapsamda olduğu ve
neyin **bilerek dışarıda** bırakıldığı:
[docs/plans/roadmap.md](docs/plans/roadmap.md) ve
[docs/design/feature-inventory.md](docs/design/feature-inventory.md).

## Desteklenen hesaplar

| Sağlayıcı | Kimlik doğrulama | Not |
|---|---|---|
| Microsoft 365 / Outlook.com | OAuth 2.0 (XOAUTH2) | Kendi Entra uygulama kaydınız gerekir |
| Gmail / Google Workspace | OAuth 2.0 (XOAUTH2) | Kendi Google Cloud client ID'niz gerekir |
| Şirket içi Exchange | Parola | IMAP üzerinden; 143/STARTTLS veya 993/TLS |
| Genel IMAP | Parola veya uygulama parolası | Kayıt gerekmez |

Şirket içi Exchange IMAP üzerinden bağlanır: hesap eklerken *Other IMAP
server*, sunucu adı BT'nin verdiği iç adres, şifreleme olarak **STARTTLS**
(Exchange'in IMAP4 servisi varsayılan olarak 143'te yayınlanır ve bağlantı
yükseltilmeden parola kabul etmez). Kayıt ya da client ID gerekmez. Kurum
temel kimlik doğrulamayı kapatmışsa NTLM gerekir ve o henüz yok — bu durumda
hata mesajı sunucunun hangi mekanizmaları sunduğunu adlandırır.

Microsoft ve Google için kendi OAuth istemcinizi kaydetmeniz gerekir — bu, açık
kaynak olmanın bir sonucu ve aslında bir avantaj: posta kutunuza erişim sizin
kontrolünüzde kalır. Birkaç dakikalık kurulum
[docs/oauth-setup.md](docs/oauth-setup.md) içinde anlatılıyor.

## Mimari

```
frontend/          React + TypeScript + Vite
      │
internal/app       Wails servisleri: arayüze açılan tek yüzey
      │
internal/sync      Senkron motoru: ilk senkron, delta, IDLE, işlem kuyruğu
      │
      ├── internal/imapx    go-imap sarmalayıcısı (MailBackend arayüzü)
      ├── internal/store    SQLite: şema, migration, repository'ler
      └── internal/auth     Kimlik sağlayıcıları ve sır saklama
```

Bağımlılık tek yönlüdür ve bu bir tercih değil, CI'da `depguard` ile zorlanan
bir kuraldır. En önemli sınır şu: `sync` motoru go-imap'i doğrudan görmez,
`MailBackend` arayüzünü görür. Bunun sayesinde tüm senkron mantığı, go-imap'in
kendi bellek içi sunucusuna karşı gerçek ağ olmadan test edilebiliyor.

Ayrıntılar: [docs/design/p0-architecture.md](docs/design/p0-architecture.md)

## Kaynaktan derleme

**Gereksinimler:** Go 1.26+, Node 20+, Wails v3 CLI.
Linux'ta ayrıca `libgtk-4-dev` ve `libwebkitgtk-6.0-dev` — Wails v3'ün
`-tags gtk3` olmadan bağlandığı yığın bu.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9
git clone https://github.com/eywatronics/nexus-mail.git
cd nexus-mail
wails3 generate bindings
cd frontend && npm ci && npm run build && cd ..
wails3 build
```

`main.go`, `frontend/dist` dizinini gömdüğü için **frontend bir kez
derlenmeden hiçbir Go paketi derlenmez.** Yukarıdaki sıra bu yüzden önemli.

### Testler

```bash
go test ./...
cd frontend && npm test
```

Yarış dedektörü (`-race`) cgo gerektirir ve CI'da çalışır.

### Kurulum paketi

```bash
wails3 task package
```

Bulunduğunuz platform için paket üretir: Windows'ta NSIS kurulum dosyası,
macOS'ta `.app`, Linux'ta AppImage / `.deb` / `.rpm`. Çıktılar `bin/` altında.

Çapraz derleme yok: her platform kendi üzerinde derleniyor. Wails'in WebView
bağlaması her sistemin kendi araç zincirini gerektiriyor ve Docker ile
zorlamak, üretilen şeyin çalıştığını kimsenin denemediği bir yapı üretirdi.
Release iş akışı da bu yüzden üç ayrı runner kullanıyor.

### Sürüm çıkarma

`main`'e her birleşme, üç platform için yapı üretip **`main` etiketli** yuvarlak
bir ön-sürümü değiştirir. Kalıcı sürüm için `build/config.yml` içindeki
`info.version` değerini yükseltin ve aynı numarayla etiket atın:

```bash
git tag v0.6.0 && git push origin v0.6.0
```

Sürüm numarası tek yerde durur: kurulum dosyasının kendi sürümü ile göründüğü
release'in adı birbirinden ayrılamasın diye.

## Verinin nerede durduğu

| Platform | Yol |
|---|---|
| Windows | `%APPDATA%\nexus-mail\` |
| macOS | `~/Library/Application Support/nexus-mail/` |
| Linux | `$XDG_DATA_HOME/nexus-mail/` (yoksa `~/.local/share/nexus-mail/`) |

Bu dizinde `mail.db`, ekler, `config.json` ve loglar bulunur. Sırlar buraya
**yazılmaz** — yalnızca işletim sisteminin anahtarlığına gider.

### Saklama penceresi

Canlı senkron aylarca çalıştıkça veritabanı sürekli büyür. Varsayılan olarak
klasör başına **son 365 gün veya 25.000 mesaj** tutulur; hangisi önce dolarsa.
Dışarıda kalanlar yerel veritabanından silinir — **sunucuya dokunulmaz**, mailler
orada durmaya devam eder. **Yıldızlı mesajlar yaşına bakılmaksızın muaftır.**

`config.json` ile değiştirilebilir:

```json
{
  "retentionDays": 365,
  "retentionMaxMessages": 25000
}
```

Her ikisine de `0` yazmak pencereyi kapatır: **her şey tutulur.** Diski nasıl
kullanacağınız sizin kararınız; yerel-önce bir uygulamada doğru varsayılan bu.

### Geri alma penceresi

Silme ve taşıma, sunucuya gitmeden önce **beş saniye** bekler. O aralıkta
Ctrl+Z ya da penceredeki **Undo** işlemi tamamen geri alır — mesaj hiç
taşınmamış olur. Süre dolduktan sonra teklif kaybolur; başarısız olacak bir
düğme bırakmaktansa hiç bırakmamak daha iyi.

```json
{
  "undoWindowSeconds": 5
}
```

`0` yazmak pencereyi kapatır: değişiklikler anında gider ve geri alma
sunulmaz.

### Bildirim önizlemesi

Yeni mail bildirimi varsayılan olarak gönderenin adını ve konuyu gösterir.
Windows bildirimleri aksi söylenmedikçe **kilit ekranında** da gösterir;
masanın başında duran birinin kimin ne hakkında yazdığını okuyabilmesini
istemiyorsanız:

```json
{
  "notificationPreview": false
}
```

O zaman bildirim yalnızca kaç mesaj geldiğini ve hangi hesaba geldiğini söyler.
Windows'un kendi "kilitliyken bildirim içeriğini gizle" ayarı da aynı işi
sistem genelinde yapar.

## Katkıda bulunma

CI yalnızca testleri değil, mimariyi de denetler:

- **Katman sınırları** `depguard` ile kontrol edilir. `store`, `imapx` ve
  `auth` yukarı doğru import edemez; `sync` go-imap'i doğrudan import edemez.
- **Windows ve Linux derlemeleri `CGO_ENABLED=0` ile** zorunlu kontroldür. Bu
  aynı zamanda bağımlılıklarımızın saf Go kaldığının kanıtıdır: cgo gerektiren
  bir kütüphane eklenirse bu iki iş kırılır.
- **Arama indeksine doğrudan yazma yasağı** grep ile kontrol edilir; indeks
  veritabanı tetikleyicileriyle korunur.
- **`go test -race ./...`** ve ayrıca saf Go yolunda ikinci bir test koşumu.

Proje kuralları [CLAUDE.md](CLAUDE.md) dosyasında; commit ve dal akışı da orada
tanımlı.

### Depo yöneticileri için

`build (windows-latest)` ve `build (ubuntu-latest)` işleri, dal koruma
ayarlarında **zorunlu kontrol** olarak işaretlenmelidir. cgo muhafızı bunlara
dayanıyor.

## Lisans

[GNU General Public License v3.0](LICENSE)

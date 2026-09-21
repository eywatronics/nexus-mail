<div align="center">

# Nexus Mail

**Yerel-önce, gizlilik odaklı, gecikmesiz masaüstü e-posta istemcisi.**

Go · Wails v3 · React · SQLite

[![CI](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml/badge.svg)](https://github.com/eywatronics/nexus-mail/actions/workflows/ci.yml)
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

## Durum

Geliştirme aşamasında. **M1–M3 bitti:** hesap bağlama, canlı senkron ve
çevrimdışı dayanıklı durum yazma çalışıyor. Uygulama artık salt okunur değil —
okundu, yıldız, taşı ve sil işlemleri anında görünüyor, kuyruğa alınıyor ve
bağlantı geldiğinde sunucuya gidiyor.

**M5 büyük ölçüde bitti:** ekleri indirme, konuşma gruplama, kaynağı görüntüleme
ve `.eml` kaydetme, kodlama onarımı, gövde görüntüleme kipleri, SPECIAL-USE
klasör rolleri. **M4 kısmen bitti:** pencere kapanınca uygulama tepside kalıyor
ve senkron sürüyor, yeni mail bildirimi gönderiyor.

Henüz **gönderme yok** (M6). Yazdırma, sandbox'lı okuma paneli yüzünden ayrı bir
pencere gerektiriyor ve M6'ya ertelendi.

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
Linux'ta ayrıca `libgtk-3-dev` ve `libwebkit2gtk-4.1-dev`.

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

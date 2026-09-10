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

Geliştirme aşamasında. Şu an **M1 (salt okunur istemci)** üzerinde çalışılıyor.

| Kilometre taşı | Kapsam | Durum |
|---|---|---|
| **M1** | Hesap bağlama (3 yol), klasör ve başlık senkronu, izole okuma, arama, klavye navigasyonu | Devam ediyor |
| **M2** | IMAP IDLE ile canlı senkron, delta senkron, saklama penceresi | Planlandı |
| **M3** | Çevrimdışı dayanıklı durum yazma (okundu, yıldız, taşı, sil) | Planlandı |
| **M4** | Sistem tepsisi, bildirimler, arka planda çalışma | Planlandı |
| **M5** | Ekler, konuşma gruplama, kaynak/yazdır/kaydet, kodlama onarımı | Planlandı |
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
| Genel IMAP | Parola veya uygulama parolası | Kayıt gerekmez |

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

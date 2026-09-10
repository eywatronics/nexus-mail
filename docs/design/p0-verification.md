# Nexus Mail — P0 Doğrulama ve Denetim Stratejisi

- **Tarih:** 2026-09-09
- **İlgili tasarım:** [2026-09-09-nexus-mail-p0-design.md](2026-09-09-nexus-mail-p0-design.md)
- **Amaç:** Tasarım dokümanındaki kuralların kod tarafında gerçekten uygulandığını
  otomatik olarak kanıtlamak. Bir kural CI'da zorlanmıyorsa, o kural yoktur.

---

## 1. Mimari Kuralların Zorlanması

### 1.1 cgo politikası — platforma göre farklı

**Önemli düzeltme:** `CGO_ENABLED=0` üç platformda sabitlenemez. Wails'in macOS
WebView bağlaması cgo gerektirir. Dolayısıyla kural şudur:

| Platform | CGO | Gerekçe |
|---|---|---|
| Windows | `CGO_ENABLED=0` (zorunlu) | Bağımlılıklarımız saf Go olmalı. |
| Linux | `CGO_ENABLED=0` (zorunlu) | Aynı. |
| macOS | cgo açık | Wails WebView bağlaması gerektiriyor. Kaçınılamaz. |

Bu asimetri bize bedava bir muhafız verir: **Windows ve Linux'un cgo'suz derlenmesi,
SQLite ve keyring bağımlılıklarının gerçekten saf Go olduğunun kanıtıdır.** Biri
`mattn/go-sqlite3` gibi bir cgo bağımlılığı eklerse bu iki build derlenmez ve PR
kırılır. macOS'ta cgo açık olduğu için o build bu ihlali yakalamaz — bu yüzden
Windows/Linux job'ları CI'da **zorunlu (required) check** olarak işaretlenecek.

### 1.2 Bağımlılık yönü — `depguard`

`golangci-lint` içindeki `depguard` ile alt katmanların üst katmanları import etmesi
engellenir. Tasarımdaki yön: `app → sync → {imapx, store, auth}`.

```yaml
# .golangci.yml (özet)
linters-settings:
  depguard:
    rules:
      store-katmani:
        files: ["**/internal/store/**"]
        deny:
          - pkg: "nexusmail/internal/sync"
          - pkg: "nexusmail/internal/imapx"
          - pkg: "nexusmail/internal/app"
      imapx-katmani:
        files: ["**/internal/imapx/**"]
        deny:
          - pkg: "nexusmail/internal/sync"
          - pkg: "nexusmail/internal/app"
          - pkg: "nexusmail/internal/store"
      sync-katmani:
        files: ["**/internal/sync/**"]
        deny:
          - pkg: "nexusmail/internal/app"
```

Ek olarak `sync` paketinin `go-imap`'i **doğrudan** import etmediği kontrol edilir —
motor `MailBackend` arayüzü üzerinden çalışmalı, yoksa sahte sunucuyla test
edilemez hale gelir:

```yaml
      sync-imap-sizinti:
        files: ["**/internal/sync/**"]
        deny:
          - pkg: "github.com/emersion/go-imap/v2"
            desc: "sync motoru IMAP'e MailBackend arayuzu uzerinden baglanir"
```

### 1.3 Sürüm sabitleme

Commit hash'e pinlemek **gereksizdir**: `v3.0.0-beta.9` etiketi `go.sum` ile birlikte
zaten değişmez ve kriptografik olarak doğrulanabilir. Gerçek risk dışarıdan bir
kırılma değil, kendi `go get -u` çağrımızdır. Kontroller:

- CI'da `go mod tidy` sonrası `git diff --exit-code go.mod go.sum` — kirli diff PR'ı kırar.
- Renovate/Dependabot yapılandırmasında `wailsapp/wails` ve `emersion/go-imap`
  otomatik güncellemeden **muaf**, yalnızca manuel onayla yükseltilir.
- **Wails CLI sürümü kütüphane sürümüyle eşleşmeli.** Uyuşmazlık binding üretimini
  sessizce bozar. CI'da doğrulanır:
  `wails3 version` çıktısı ile `go.mod`'daki `github.com/wailsapp/wails/v3` sürümü
  karşılaştırılır.

---

## 2. Veritabanı ve Senkronizasyon Doğrulaması

### 2.1 Eşzamanlılık — iki ayrı test gerekiyor

**Önemli düzeltme:** `go test -race`, "database is locked" (SQLITE_BUSY) hatasını
yakalamaz. `-race` veri yarışlarını (aynı belleğe senkronizasyonsuz erişim) bulur;
SQLITE_BUSY ise kilit çekişmesidir — race detector için görünmez bir sınıf. İkisi
ayrı ayrı test edilir:

**Test A — veri yarışı:** `go test -race ./...` tüm pakette, CI'da zorunlu.

**Test B — kilit çekişmesi stres testi:**
- 50 goroutine sürekli mesaj listesi okur.
- 1 goroutine sürekli mesaj yazar/siler.
- 10 saniye boyunca çalışır.
- *Assert:* Hiçbir çağrı `SQLITE_BUSY` / "database is locked" döndürmez.
- *Assert:* Toplam yazma sayısı ile veritabanındaki son durum tutarlıdır.

Bu test "1 yazıcı + N okuyucu" mimarisinin gerçekten uygulandığının kanıtıdır. Testi
geçmek için `busy_timeout`'a güvenmek yeterli değildir — yazma işlemleri tek bir
goroutine üzerinden serileştirilmiş olmalıdır.

### 2.2 UIDVALIDITY değişimi — kabul testi

Sahte sunucu (`go-imap/v2/imapserver`) üzerinden:

1. Senkron motoru çalıştırılır, N mesaj yerel veritabanına iner.
2. Sahte sunucuda klasörün `UIDVALIDITY` değeri değiştirilir (klasör silinip yeniden
   yaratılmış senaryosu).
3. Senkron motoru tekrar çalıştırılır.
4. *Assert:* O klasöre ait tüm eski satırlar silinmiştir.
5. *Assert:* Mesajlar sıfırdan yeniden çekilmiştir.
6. *Assert:* **Diğer klasörler etkilenmemiştir** — sıfırlama klasör bazındadır,
   hesap bazında değil.

### 2.3 İşlem kuyruğu

**Idempotency:** Aynı `flag_add` işlemi iki kez uygulandığında sunucu durumu değişmez
ve hata oluşmaz.

**Çevrimdışı birikme:** Bağlantı yokken "okundu" + "yıldız" işlemleri yapılır.
- *Assert:* `operations` tablosunda `state='pending'` iki satır.
- *Assert:* UI anında güncellenmiş (iyimser yazma).
- Bağlantı geri geldiğinde ikisi de `done` olur ve sahte sunucuda bayraklar doğrudur.

**Bloke olmama:** Kuyruğa kalıcı olarak başarısız olacak bir işlem (silinmiş klasöre
taşıma) konur, ardından 5 geçerli işlem eklenir.
- *Assert:* Bozuk işlem `failed` olur.
- *Assert:* Diğer 5 işlem `done` olur — kuyruk bloke olmaz.

### 2.4 Arama indeksi tetikleyicileri

Tetikleyiciler atlanabilir bir yol bırakmamalı. Dört test:

1. **Ekleme:** `messages`'a satır yazılır; `fts_messages` üzerinde `MATCH`
   sorgusu onu bulur.
2. **Upsert:** Aynı UID ikinci kez yazılır (konu değişmiş olarak).
   - *Assert:* FTS'te **tek** satır var — mükerrer indeks yok.
   - *Assert:* Arama **yeni** konuyla bulur, eskisiyle bulmaz.
   - Bu test upsert'in INSERT değil UPDATE tetikleyicisini çalıştırdığını
     doğrular; bu ayrım atlanırsa indeks eski değerlerle birikir.
3. **Silme:** `ResetFolder` çağrılır.
   - *Assert:* O klasöre ait hiçbir satır FTS'te kalmaz.
   - *Assert:* Kardeş klasörün satırları FTS'te durur.
4. **İndeks bütünlüğü:** Yukarıdaki üç adımdan sonra
   `INSERT INTO fts_messages(fts_messages) VALUES('integrity-check')` çalıştırılır.
   - *Assert:* Hata dönmez. Bu, `'delete'` komutlarına doğru eski değerlerin
     verildiğinin tek gerçek kanıtıdır — yanlış değer verilen bir indeks
     sorgularda sessizce yanlış sonuç döndürür ama yalnızca bu kontrol bağırır.

Ek olarak bir **negatif test**: uygulama kodunda `fts_messages`'a doğrudan yazan
bir yer olmadığı grep ile doğrulanır ve CI'da kontrol edilir:

```bash
! grep -rn "INSERT INTO fts_messages" --include='*.go' internal/ \
  | grep -v '_test.go' \
  || (echo "application code must not write to fts_messages; use triggers" && exit 1)
```

### 2.5 UIDVALIDITY damgası ile bekleyen işlemler (M3)

Tasarım §6.6'nın kabul testi. Bu, projenin yanlış maili silebileceği tek yol.

1. Klasör `UIDVALIDITY=100` ile senkronlanır, mesajlar iner.
2. Bağlantı kesilir; `UID 55` için "sil" işlemi kuyruğa girer.
   - *Assert:* `operations` satırı `uid_validity = 100` damgası taşır.
3. Sunucuda klasör yeniden yaratılır (`UIDVALIDITY=999`), farklı mesajlar konur.
4. Bağlantı geri gelir, senkron ve kuyruk boşaltma çalışır.
   - *Assert:* İşlem `state='dropped'` olur.
   - *Assert:* Sahte sunucuda **hiçbir** mesaj silinmemiştir. Bu iddia testin
     kalbi: damga yoksa buradaki `UID 55` silinir ve o başka bir maildir.
   - *Assert:* Kullanıcıya bildirim üretilmiştir (olay yayınlanmıştır).
5. Aynı senaryo `UIDVALIDITY` **değişmeden** tekrarlanır.
   - *Assert:* İşlem normal şekilde `done` olur ve doğru mesaj silinir — damga
     kontrolü geçerli işlemleri engellemez.

### 2.6 CONDSTORE'suz sunucu

Sahte sunucu CONDSTORE capability'sini **bildirmeden** başlatılır.
- *Assert:* Yedek delta yolu devreye girer.
- *Assert:* Yeni mesajlar, bayrak değişiklikleri ve silmeler doğru yansır.

---

## 3. Güvenlik ve Gizlilik Doğrulaması

### 3.1 Sandbox ve tracker testi — otomatik

Elle değil, otomatik test olarak kurulur. Zararlı bir test maili şunları içerir:

```html
<script>window.__xssFired = true</script>
<img src="http://127.0.0.1:<port>/tracker.png">
<div style="background:url('http://127.0.0.1:<port>/css-tracker.png')"></div>
<body background="http://127.0.0.1:<port>/attr-tracker.png">
```

Test yerel bir `httptest` sunucusu açar ve **isabet sayacı** tutar.

- *Assert:* `window.__xssFired` tanımsız — script çalışmadı.
- *Assert:* Sayaç **sıfır** — üç tracker vektörünün hiçbiri istek atmadı.
- Kullanıcı "resimleri göster" dedikten sonra: istekler Go proxy'sinden gelir,
  `Referer` başlığı yoktur ve istemci IP'si doğrudan sızmaz.

Üç vektörün hepsi ayrı ayrı test edilir; yalnızca `<img src>` engellemek yetersizdir.

**Not — konum değişikliği:** Tasarım §7.3 uyarınca temizleme Go tarafına
(`internal/mailhtml`, bluemonday) taşındı. Yukarıdaki vektör testleri artık Go
testleri olarak yazılır ve **§3.1'in "jsdom uzak kaynak indirmiyor" kısıtı ortadan
kalkar:** Go tarafında `httptest` sunucusu ve gerçek isabet sayacı kullanılabilir.
Yani tam otomasyon artık M2'ye ertelenmiyor, P0'da mümkün:

1. Zararlı mail gövdesi veritabanına yazılır.
2. `mailhtml.Sanitize` çağrılır.
3. Çıktı gerçek bir HTTP istemcisiyle taranır — çıktıdaki her URL istenir.
   - *Assert:* Sayaç sıfır; hiçbir vektör istek üretmedi.
4. `allowRemote=true` ile tekrarlanır.
   - *Assert:* İstekler **Go proxy'sinden** gelir, `Referer` başlığı boş, ve
     istemci IP'si proxy'nin kendisidir.

### 3.3 Özel protokol handler'ı

Gövde artık `wails://mail-body/<id>` üzerinden servis edildiği için handler'ın
kendisi test edilir (Wails'e ihtiyaç duymadan, `http.Handler` olarak):

- *Assert:* Yanıt gerçek bir `Content-Security-Policy` başlığı taşır ve
  `default-src 'none'` içerir.
- *Assert:* Var olmayan bir mesaj kimliği 404 döner, panik atmaz.
- *Assert:* Yol geçişi (path traversal) denemesi — `mail-body/../../secrets` —
  reddedilir.
- *Assert:* Başka bir hesabın mesaj kimliği istendiğinde içerik döner ama
  **yalnızca yerel veritabanından**; handler hiçbir koşulda ağa çıkmaz
  (gövde yoksa 404, senkron tetiklemez).
- *Assert:* Yanıt gövdesi temizlenmiş HTML'dir — ham gövde asla servis edilmez.

### 3.4 Log gizliliği

Tasarım §9.1'in kabul testi. Gizlilik odaklı bir uygulamada "logları dışa aktar"
düğmesi, denetlenmezse sızıntı yoluna dönüşür.

Bilinen "kanarya" değerlerle bir senkron çalıştırılır: konu
`CANARY-SUBJECT-9f3a`, gönderen `canary-sender@example.invalid`, parola
`CANARY-PASSWORD-7b21`, refresh token `CANARY-TOKEN-4c8d`.

- *Assert:* Log çıktısında dört kanaryanın **hiçbiri** geçmiyor.
- *Assert:* Aynı iddia `DEBUG` seviyesinde de geçerli — hata ayıklama modu
  gizliliği gevşetmez.
- *Assert:* Log satırları `account_id`, `folder_path`, `operation` alanlarını
  taşıyor; yani gizlilik kuralı logu işe yaramaz hale getirmemiş.

### 3.5 Sanitizer altın dosya testleri

XSS vektör dosyaları `testdata/xss/` altında tutulur; her biri için beklenen
temizlenmiş çıktı bir altın dosyada saklanır. Yeni bir bypass keşfedildiğinde
vektör dosyası eklenir — test seti zamanla birikir.

---

## 4. Performans Bütçeleri

### 4.1 DOM sanallaştırma

Gelen kutusuna 50.000 sahte mesaj yüklenir.
- *Assert:* Liste konteynerindeki DOM node sayısı < 100 (ekranda görünen ~20-30
  satır + tampon).
- *Assert:* Kaydırma sırasında node sayısı sabit kalır — artıyorsa sanallaştırma
  sızdırıyor demektir.

### 4.2 RAM bütçesi

**Önemli düzeltme:** WebView2 çok süreçlidir. Go süreci, WebView host süreci ve
renderer ayrı ayrı görünür; tek süreci ölçmek gerçeğin bir kısmını gösterir.

Bütçe **süreç ağacının toplamı** olarak tanımlanır:

| Durum | Hedef (süreç ağacı toplamı, private working set) |
|---|---|
| Boşta, 1 hesap, 10.000 mesaj senkronize | ≤ 250 MB |
| Aktif kullanım (liste kaydırma, mail açma) | ≤ 400 MB |

Bu sayılar orijinal plandaki 30-150 MB'dan yüksek; sebebi WebView2 renderer'ının
kendi başına ~100 MB taban maliyeti olması. Ölçüm süreç ağacını toplayan bir
script ile yapılır ve sonuç bir referans değere karşı raporlanır — CI'da katı
bir eşik olarak zorlanmaz (runner'lar arası varyans yüksek), ancak sürüm öncesi
elle doğrulanır.

#### İlk gerçek ölçüm (2026-09-10, Windows 11, hesap eklenmemiş)

Tabloyu yazarken atlanan şey, **hangi bellek metriği** olduğuydu. Ölçünce iki
metrik arasında 3,5 kat fark çıktı, yani metrik belirtmeyen bir bütçe aslında
bir şey söylemiyor:

| | Private working set | Working set |
|---|---|---|
| Go süreci | 12 MB | 42 MB |
| WebView2 (6 süreç) | 105 MB | 372 MB |
| **Toplam** | **117 MB** | **414 MB** |

Fark paylaşılan sayfalardan geliyor: Edge çalışma zamanının kod sayfaları altı
sürecin her birinde ayrı ayrı sayılıyor, üstelik makinedeki diğer WebView2
uygulamalarıyla da paylaşılıyorlar. Bu ölçüm sırasında makinede zaten 26 tane
başka WebView2 süreci vardı; uygulamanın payı, açılıştan önceki ve sonraki
listenin farkı alınarak hesaplandı.

**Bütçe bundan böyle private working set üzerinden tanımlanır.** Working set,
uygulamanın gerçekten sahip olduğu belleği değil, sistemin başka yerlerde de
duran sayfalarını ona fatura eder.

Bu sayı henüz tablonun karşılığı değil: ölçüm **hiç hesap eklenmeden**, hesap
ekleme ekranı açıkken yapıldı. Yani 117 MB bir taban, bütçelenen "1 hesap,
10.000 mesaj" durumu değil. O satır gerçek bir hesapla doldurulmayı bekliyor.

---

## 5. Kilometre Taşı Kabul Kriterleri

### M1 — Salt okunur istemci

- [ ] Windows ve Linux'ta `CGO_ENABLED=0` ile derleniyor; macOS'ta cgo açık derleniyor.
- [ ] OAuth girişinde sistem tarayıcısı açılıyor, onay sonrası **rastgele** loopback
      portunda kod yakalanıyor.
- [ ] Refresh token OS anahtarlığına yazıldı — Windows Credential Manager / Keychain
      açılıp gözle doğrulandı.
- [ ] Veritabanı dosyasında hiçbir token/parola **yok** (`strings mail.db | grep` ile
      doğrulanır).
- [ ] Üç auth yolunun her biri gerçek bir hesapla en az bir kez çalıştı
      (Microsoft, Google, genel IMAP).
- [ ] Klasörler ve başlıklar UI'da listelendi.
- [ ] **Local-first kanıtı:** Uygulama kapatılıp ağ bağlantısı kesildikten sonra
      açıldığında mailler anında ekranda.
- [ ] Zararlı test maili açıldığında ne script çalışıyor ne tracker isteği gidiyor.
- [ ] Arama indeksi doldu: `SELECT count(*) FROM fts_messages` sayısı
      `SELECT count(*) FROM messages` ile aynı.
- [ ] İndeks bütünlüğü temiz: `INSERT INTO fts_messages(fts_messages)
      VALUES('integrity-check')` hata döndürmüyor.
- [ ] Gövde `wails://mail-body/<id>` üzerinden yükleniyor — geliştirici
      araçlarının ağ sekmesinde bu istek görünüyor ve IPC'de büyük bir yük yok.
- [ ] 5 MB'ı aşan bir e-bülten açıldığında arayüz donmuyor (gözle: kaydırma
      akıcı kalıyor).
- [ ] **Yaşam döngüsü:** Pencere X ile kapatıldığında süreç gerçekten sonlanıyor
      — Görev Yöneticisi'nde ne `nexus-mail` ne WebView süreci kalıyor. Tepside
      ikon yok (P0 kuralı).
- [ ] **Loglar:** `logs/nexus.log` yazılıyor, JSON satırları `account_id` ve
      `folder_path` taşıyor, ve dosyada mail konusu, adres veya token geçmiyor:

```bash
grep -iE 'bearer|refresh_token|@' logs/nexus.log || echo "temiz"
```

- [ ] Ayarlar ekranındaki "Export logs" düğmesi zip üretiyor ve kaydetmeden önce
      içeriği gösteriyor.

### M2 — Canlı senkron

- [ ] Başka bir cihazdan mail gönderildiğinde, **yenile tuşuna basılmadan** saniyeler
      içinde listede görünüyor (IDLE kanıtı).
- [ ] Ağ kesilip geri açıldığında IDLE yeniden bağlanıyor ve arada kaçan
      değişiklikler delta senkronla yakalanıyor.
- [ ] Başka bir cihazdan okundu işareti konduğunda bayrak PC'ye yansıyor.
- [ ] CONDSTORE destekleyen ve desteklemeyen sunucularda ikisi de çalışıyor
      (sahte sunucu testleriyle).
- [ ] 29 dakikayı aşan bekleme sonrası IDLE hâlâ canlı (uzun süreli el testi).

### M3 — Çevrimdışı durum yazma

- [ ] Ağ kapalıyken yıldız ekleme ve çöpe taşıma UI'da **anında** yansıyor.
- [ ] `operations` tablosunda iki satır `state='pending'`.
- [ ] Ağ açıldığında satırlar `done` oluyor ve sunucuda mail gerçekten çöpte.
- [ ] Kuyrukta kalıcı hata varken diğer işlemler akmaya devam ediyor.
- [ ] Aynı mesaja çevrimdışı yapılan işlem ile sunucudaki değişiklik çakıştığında
      "sunucu kazanır" kuralı uygulanıyor.

---

## 6. CI/CD Yapılandırması

GitHub Actions, üç job:

**1. Lint & Format**
- `golangci-lint run` (depguard kuralları dahil)
- `gofmt -l` boş çıktı
- ESLint + Prettier
- `go mod tidy` sonrası `git diff --exit-code go.mod go.sum`
- Wails CLI ↔ kütüphane sürüm eşleşmesi kontrolü

**2. Test**
- `go test -race ./...` (sahte IMAP sunucu testleri dahil)
- Eşzamanlılık stres testi (SQLITE_BUSY assertion'ı)
- Sanitizer altın dosya testleri
- `vitest run`

**3. Build matrisi** — çapraz derleme **tek komutla yapılamaz**; `wails3 build` host
işletim sistemini hedefler. Native runner'lar kullanılır:

| Runner | Komut | CGO |
|---|---|---|
| `windows-latest` | `wails3 build` | `CGO_ENABLED=0` — **zorunlu check** |
| `ubuntu-latest` | `wails3 build` | `CGO_ENABLED=0` — **zorunlu check** |
| `macos-latest` | `wails3 build` | cgo açık |

Alternatif olarak `wails-cross` Docker imajı ile tek runner'dan üç platform
derlenebilir, ancak çapraz derlenen macOS binary'si imzasız çıkar ve dağıtım öncesi
imzalanması gerekir. P0 için native matris tercih edilir; Docker yolu dağıtım
fazında (P2+) değerlendirilir.

---

## 7. Kaynaklar

- [Wails v3 build sistemi](https://v3.wails.io/concepts/build-system/) ve
  [çapraz platform derleme](https://v3.wails.io/guides/build/cross-platform/)
- [Wails FAQ — çapraz derleme ve v3 durumu](https://github.com/wailsapp/wails/discussions/5139)
- [golangci-lint depguard](https://golangci-lint.run/usage/linters/#depguard)

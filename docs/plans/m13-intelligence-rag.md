# M13 — Yerel-önce RAG ve posta zekâsı

**Hedef:** uzun zincirleri özetleyen, gelen kutusundaki yazışmalardan doğal
dille bilgi çıkaran, yanıt taslağı üreten ve anlamsal arama yapabilen bir
katman — kullanıcının gizliliğinden ödün vermeden.

İki çalışma kipi: makinede çalışan **yerel model** (Ollama), ya da kullanıcının
kendi anahtarıyla **bulut sağlayıcı** (BYOK).

M13 daha önce Sohbet'ti (Matrix + XMPP). O iş [M15](roadmap.md#m15--sohbet)'e
taşındı; kapsamı değişmedi, sırası değişti.

---

## 1. Önce dürüst olalım: bu, bir kuralın istisnası

Yol haritasının "bilerek kapsam dışı" tablosunda **işletim sistemi arama
entegrasyonu** şu gerekçeyle reddedilmişti: *"postayı OS indeksine verir."*
Telemetri de aynı yerde, "asla" notuyla.

Bulut sağlayıcı kipi bundan **daha büyük** bir adım: mesaj gövdesinin tamamı
üçüncü bir tarafın sunucusuna gider. Bunu "ama opt-in" diyerek geçiştirmek,
projenin kendi tablosuyla çelişmek olur.

Kapsamda tutulmasının gerekçesi şu: OS arama entegrasyonunda veriyi veren
uygulamaydı ve kullanıcı bunu göremiyordu. Burada veriyi veren kullanıcının
kendisi, kendi anahtarıyla, tek tek hangi mesaj için olduğunu görerek. Fark
rızanın olup olmaması değil, **rızanın görünür ve dönülebilir olması.**

Bu yüzden aşağıdaki üç şey pazarlık konusu değil:

- **Varsayılan kapalı.** Kurulumdan sonra hiçbir şey açılmaz, hiçbir yere
  bağlanılmaz.
- **Yerel kip varsayılan öneri.** Ayarlar ekranı önce Ollama'yı sorar; bulut
  ikinci seçenek olarak durur, tersi değil.
- **Her bulut çağrısı öncesi ne gittiği söylenir.** "Bu işlem şu mesajın
  gövdesini \<sağlayıcı\> sunucularına iletir" — sayfa sonunda küçük yazı
  değil, eylemin yanında.

Yerel kipte tek bayt makineden çıkmaz ve bunu bir test kanıtlar: sahte bir
sağlayıcıya karşı çalışırken `127.0.0.1` dışına giden bir bağlantı denemesi
testi düşürür. `mailhtml` katmanındaki `net/http` yasağının aynısı, ters
yönde.

### Loglar

`CLAUDE.md`: *"Loglara mail konusu, snippet, gövde, ek dosya adı veya e-posta
adresi yazılmaz."*

Bir RAG prompt'u tanım gereği bunların hepsini birden içerir. Dolayısıyla
**prompt, yanıt ve chunk içeriği hiçbir seviyede loglanmaz.** Loglanabilecek
olan: sağlayıcı adı, model adı, token sayısı, süre, hata sınıfı. Bu, hata
ayıklamayı zorlaştırır ve kabul edilen bedel odur.

---

## 2. Katman ve bağımlılık yönü

```text
internal/ai/
├── provider/
│   ├── provider.go        # LLMProvider ve EmbeddingProvider arayüzleri
│   ├── ollama.go          # yerel çalışma zamanı (127.0.0.1:11434)
│   ├── openai.go          # OpenAI uyumlu: OpenAI, DeepSeek, Groq, OpenRouter
│   └── anthropic.go       # Claude
├── rag/
│   ├── chunker.go         # düz metne indirgeme, imza/alıntı kırpma, parçalama
│   ├── retrieve.go        # FTS5 + vektör, RRF ile harmanlama
│   └── prompt.go          # özet, soru-cevap ve taslak şablonları
├── worker/
│   └── indexer.go         # arka plan vektörleştirme
└── service.go             # app katmanına açılan kapı
```

Bağımlılık yönü, `imapx.MailBackend` deseninin aynısı: motor somut sağlayıcıyı
değil arayüzü görür, sağlayıcı kütüphanesi yalnızca kendi paketinde import
edilir. `internal/ai` yalnızca `internal/store` ve `internal/auth` ile
konuşur; `app` bu katmana Wails servisleri üzerinden erişir.

`.golangci.yml` içindeki `ai-layer` ve `ai-network` kuralları **kod
yazılmadan önce** eklendi — projenin kendi kuralı: *"kural yazılmamışsa mimari
kural yok demektir."* Sonradan eklenen bir kural, ona ihtiyaç duyan kodun
onsuz yazılmış olması demek.

`ai-network` mimari değil **gizlilik** kuralı: yalnızca `provider` alt paketi
`net/http` import edebilir, yani "hangi kod bir mail gövdesini modele
gönderebilir" sorusunun tek dizinlik bir cevabı olur. `mailhtml` katmanındaki
aynı yasağın kardeşi.

```go
type CompletionRequest struct {
	System      string
	Prompt      string
	Temperature float32
}

type LLMProvider interface {
	Name() string
	Generate(ctx context.Context, req CompletionRequest) (string, error)
	// Stream, onChunk her parçada çağrılarak akar. Hata dönerse üretim durur:
	// pencere kapandıysa modelin konuşmaya devam etmesinin anlamı yok.
	Stream(ctx context.Context, req CompletionRequest, onChunk func(string) error) error
}

type EmbeddingProvider interface {
	// Model ve Dimension birlikte saklanır: vektörler ancak kendilerini üreten
	// modelle karşılaştırılabilir. Bkz. §4.
	Model() string
	Dimension() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

### API anahtarları

IMAP parolalarıyla aynı yol: `internal/auth`'un `SecretStore`'u, yani OS
anahtarlığı. Veritabanına ve `config.json`'a **yazılmaz**. `config.json`'da
duracak olan yalnızca sağlayıcı adı, model adı ve uç nokta adresi.

---

## 3. Sağlayıcı kipleri

| | Yerel (Ollama) | Bulut (BYOK) |
|---|---|---|
| Nereye gider | hiçbir yere | sağlayıcının sunucusuna |
| İnternet | gerekmez | gerekir |
| Donanım | 8 GB RAM'den itibaren | önemsiz |
| Model | `llama3.2:3b`, `qwen2.5:3b` | GPT-4o mini, Claude Haiku, DeepSeek |
| Gömme | `nomic-embed-text` (768 boyut) | `text-embedding-3-small` (1536) |

Ollama yerel porttan yoklanır ve yüklü modeller açılır menüye doldurulur —
kullanıcıya model adı yazdırmak, yazım hatasını bizim hatamız gibi gösterir.

Yerel LLM için **3B üstü model önerilmez**. Daha büyüğü düşük donanımda
sistemi kilitler, ve bunu kullanıcı "uygulama dondu" diye yaşar.

Gömme ve üretim ayrı seçilebilir: gömmeyi yerelde tutup üretimi buluta vermek
makul bir orta yol — indekslenen her şey diskte kalır, yalnızca seçilen
parçalar dışarı çıkar.

---

## 4. Depolama: saf Go vektör

`CGO_ENABLED=0` zorunlu bir CI kontrolü. `sqlite-vec` gibi uzantılar cgo
gerektirdiği için kapsam dışı; vektörler BLOB olarak saklanır ve benzerlik
Go'da hesaplanır.

### Şema (`internal/store/migrations/004_mail_chunks.sql`)

Spec'ten gelen taslakta kolon tipleri bu depoya uymuyordu ve **hata vermeden
yanlış çalışırdı**; düzeltilmiş hâli:

```sql
CREATE TABLE mail_chunks (
  id          INTEGER PRIMARY KEY,
  -- messages(id)'ye referans, INTEGER. Taslakta TEXT'ti ve adı da
  -- message_id'ydi -- ama bu depoda messages.message_id ZATEN VAR ve
  -- RFC 5322 Message-ID başlığı demek, satır kimliği değil. SQLite gevşek
  -- tipli olduğu icin TEXT bir foreign key hata vermez, sadece hiçbir zaman
  -- eşleşmez: sessizce boş kalan bir indeks.
  message_id  INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  chunk_index INTEGER NOT NULL,
  content     TEXT    NOT NULL,
  token_count INTEGER NOT NULL DEFAULT 0,

  -- Vektör, yanında onu üreten modelle birlikte. Bu iki kolon olmadan
  -- kullanıcı gömme modelini değiştirdiğinde 768 boyutlu eski vektörlerle
  -- 1536 boyutlu yenileri aynı tabloda karışır; karşılaştırma ya panikler ya
  -- da anlamsız benzerlik üretir. Model değişince eskiler silinip yeniden
  -- üretilir, ve bunun mümkün olması için model adının yazılı olması gerekir.
  embedding   BLOB    NOT NULL,
  model       TEXT    NOT NULL,
  dim         INTEGER NOT NULL,

  created_at  INTEGER NOT NULL,
  UNIQUE(message_id, chunk_index)
);

CREATE INDEX idx_chunks_message ON mail_chunks(message_id);
CREATE INDEX idx_chunks_model   ON mail_chunks(account_id, model);
```

`ON DELETE CASCADE`, saklama penceresi mesajı sildiğinde parçalarının da
gitmesini sağlar — ayrı bir temizlik işi gerekmez.

### Parçalama

Gövde düz metne indirgenir (`mailhtml.PlainText` zaten var), imza blokları ve
tekrarlanan alıntılar kırpılır, 400–512 token'lık örtüşmeli parçalara ayrılır.

Alıntı kırpma göründüğünden önemli: on mesajlık bir zincirde son mesaj
öncekilerin tamamını içerir, ve kırpılmazsa aynı metin on kez vektörleştirilip
arama sonuçlarının tamamını tek bir zincirle doldurur.

### Hibrit arama

1. FTS5 ile anahtar kelime (BM25) — `fts_messages` tablosu zaten var ve
   tetikleyicilerle bakılıyor. **Yalnızca okunur;** uygulama kodunun bu tabloya
   yazması CI'da kontrol ediliyor.
2. Vektör benzerliği (kosinüs), Go'da.
3. İkisi RRF ile harmanlanır, ilk 5 parça prompt'a girer.

**Ölçek sorunu, baştan söylenmeli.** Saklama penceresi klasör başına 25.000
mesaj. Mesaj başına ~3 parça, 768 boyut, float32 → her sorguda ~230 MB
okumak ve 75.000 nokta çarpımı yapmak demek. Bu, "çalışır ama yavaş"ın
sınırında.

Üç hafifletme, gerektikçe:

- **Önce daraltma:** hesap, klasör ve tarih aralığıyla aday kümesini küçültmek.
  Sorguların çoğu zaten "geçen ay muhasebeden gelen" gibi bir kapsam taşıyor.
- **Nicemleme:** float32 yerine int8 → dörtte bir bellek, kabul edilebilir
  doğruluk kaybı.
- **Parça sayısını düşürmek:** kısa mesajlar tek parça kalır; bugünkü mailin
  çoğu kısadır.

İlk sürümde ölçülmeden optimize edilmez; ama şema `dim` taşıdığı için
nicemlemeye geçiş migration gerektirmez.

---

## 5. Kapatılmamış boşluk: gövdeler indirilmiş değil

**Bu, planın en büyük eksiğiydi ve uygulamadan önce karara bağlanmalı.**

Bu istemci gövdeleri **tembel** çekiyor: `messages.body_fetched` sıfır kalıyor
ve `EnsureBody` ancak mesaj açıldığında sunucuya gidiyor. Yani tipik bir
kullanıcının diskinde, 25.000 mesajın belki birkaç yüzünün gövdesi var.

"Son 1.000 maili vektörleştir" demek, dolayısıyla, **önce o 1.000 gövdeyi
indirmek** demek. Bu:

- senkron profilini değiştirir (başlık senkronu ucuzdu, bu değil),
- veritabanını büyütür,
- ölçülü bir bağlantıda fark edilir bir trafik üretir.

Üç seçenek, kararı kullanıcı-görünür yapmak şartıyla:

| Seçenek | Ne olur | Bedel |
|---|---|---|
| **A. Yalnızca okunmuşlar** | Sadece gövdesi zaten diskte olan mesajlar indekslenir | RAG kapsamı dar; "hiç açmadığım mail" sorusu cevapsız |
| **B. İstek üzerine dolum** | Kullanıcı "gelen kutumu indeksle" der, ilerleme çubuğuyla gövdeler çekilir | Açık rıza, ama bir kerelik uzun iş |
| **C. Sessiz arka plan dolumu** | İndeksleyici eksik gövdeleri kendi çeker | Kullanıcının istemediği trafik ve disk — **önerilmez** |

**Öneri: A ile başlamak, B'yi eklemek.** C, "yerel-önce"nin kullanıcıya
sormadan davranması olurdu.

---

## 6. Arayüz

Üç yüzey, `design-taste-frontend` kuralları geçerli (tek accent, tek yarıçap,
`@phosphor-icons/react`, emoji yok).

**Zincir özeti.** Okuma panelinin üstünde "Özetle". Yan panelde üç maddelik
özet, varsa aksiyon maddeleri, varsa tarihler. Akarak gelir — 3B bir modelde
ilk token birkaç saniye sürebilir ve boş panel "dondu" gibi görünür.

**Gelen kutusu asistanı.** `Ctrl+K` ile soru kutusu. Yanıtın altında
**kaynak mesajlar kart olarak** durur; tıklayınca o mesaj açılır. Kaynak
göstermek süs değil: modelin uydurduğu bir cümleyi kullanıcının
doğrulayabilmesinin tek yolu.

**Taslak asistanı.** Yanıt ekranında "kısa onay", "kibar ret", "bilgi talebi".
Taslak **gönderilmez, editöre yazılır** — bu projede tek tıkla giden bir mail
yok, ve olmayacak.

### Halüsinasyon

Sistem prompt'u kesin: *"Yalnızca verilen parçalardaki bilgiyi kullan. Bilgi
metinde yoksa 'Bu bilgi yazışmalarda bulunamadı' de."* Ve her yanıtın altında
kaynak. İkisi birlikte; prompt tek başına yeterli değildir, kaynak olmadan
kullanıcı kontrol edemez.

---

## 7. Aşamalar

| Faz | Kapsam | Gerçek bağımlılık |
|---|---|---|
| **M13.1** | Sağlayıcı katmanı, anahtarlıkta API anahtarı, Ollama algılama, ayarlar ekranı | — |
| **M13.2** | Açık mesajı/zinciri özetleme. Vektör gerekmez, doğrudan context'e girer. Akan yanıt | M13.1 |
| **M13.3** | `mail_chunks`, parçalayıcı, arka plan indeksleyici, hibrit arama | M13.2 + §5 kararı |
| **M13.4** | Gelen kutusu soru-cevap, kaynak kartları | M13.3 |
| **M13.5** | Taslak asistanı, ton eşleme | M13.4 **ve M6 (gönderme)** |

Taslakta "M13.1 → M6'ya bağlı" yazıyordu. Değil: sağlayıcı katmanı ve
özetleme, gönderme olmadan tamamen çalışır. **Yalnızca M13.5** composer'a
ihtiyaç duyar. Fark önemli, çünkü bu hâliyle M13.1–M13.4 M6 beklemeden
teslim edilebilir.

---

## 8. Kabul kriterleri

- Yapay zekâ kapalıyken uygulama hiçbir sağlayıcıya bağlanmaz; bunu bir test
  kanıtlar.
- Yerel kipte `127.0.0.1` dışına çıkan bağlantı testi düşürür.
- API anahtarı veritabanında, `config.json`'da ve loglarda **aranıp
  bulunamaz** — parola testlerindeki canary deseninin aynısı.
- Prompt, yanıt ve chunk içeriği hiçbir log seviyesinde görünmez.
- Gömme modeli değişince eski vektörler geçersiz sayılır ve yeniden üretilir.
- Saklama penceresi bir mesajı sildiğinde parçaları da gider.
- Soru-cevap yanıtı, bilgi yoksa uydurmak yerine bulunamadığını söyler.
- `CGO_ENABLED=0` ile derleme ve testler yeşil kalır.

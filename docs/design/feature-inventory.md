# Thunderbird özellik envanteri

Bu doküman Thunderbird'ün (comm-central) kullanıcıya görünen özelliklerini
çıkarır ve her birinin Nexus Mail'de ne olacağını kaydeder.

**Ne için var:** ileride bir özelliği tasarlarken "Thunderbird bunu nasıl
çözmüş" sorusunun cevabına bakılacak yer. Her satır kaynak yolunu taşır;
yol, ağaçtaki koda doğrudan gider.

**Neyin envanteri:** `thunderbird-desktop` ağacı — 41.836 dosya, 686 MB, dört
ürün alanı (`mail/`, `mailnews/`, `calendar/`, `chat/`) ve SeaMonkey (`suite/`).
Sürüm: 2026-09 tarihli `main` dalı.

**Kod kopyalanmadı.** Çıkarılan şey özellik listesidir. Lisanslar zaten uyumlu
olurdu (Thunderbird MPL-2.0, Nexus Mail GPL-3.0) ama bu dokümanın konusu değil.

## Karar sütunu nasıl okunur

| İşaret | Anlamı |
|---|---|
| `✔ M1` | Nexus Mail'de zaten var |
| `M2`–`M14` | Hedef kilometre taşı — [roadmap](../plans/roadmap.md) |
| `—` | **Bilerek kapsam dışı**, gerekçesi yazılı |

Her özellik Nexus Mail'in dört iddiasına karşı tartıldı: yerel-önce, izleyici
engelleme, gerçek izolasyon, sırlar OS anahtarlığında. "Thunderbird'de var"
tek başına kapsama sokmaz.

---

## 1. Protokoller

### 1.1 IMAP

Thunderbird'ün desteklediği uzantılar `mailnews/imap/src/nsImapCore.h`
içindeki `eIMAPCapabilityFlag` sabitlerinde sayılı; pazarlık
`nsImapProtocol.cpp` ve `nsImapServerResponseParser.cpp` içinde.

| Uzantı | Ne sağlar | KT | Karar |
|---|---|---|---|
| IMAP4rev1 (RFC 3501), rev2 | Temel protokol | ✔ M1 | go-imap v2 |
| STARTTLS / örtük TLS | Şifreli taşıma | ✔ M1 | Yalnızca örtük TLS; düz metin bağlantı reddediliyor |
| AUTH: PLAIN, LOGIN, CRAM-MD5 | Parola kimlik doğrulama | ✔ M1 kısmi | PLAIN var; CRAM-MD5 eski sunucular için M8 |
| AUTH: XOAUTH2 | Google/Microsoft OAuth | ✔ M1 | Elle uygulandı, go-sasl'da yok |
| AUTH: GSSAPI (Kerberos), NTLM | Kurumsal Windows etki alanı | M10 | Graph işiyle birlikte |
| AUTH: EXTERNAL | İstemci sertifikası | — | Niş |
| IDLE (RFC 2177) | Sunucu itmeli yeni posta | M2 | Planlı |
| CONDSTORE (RFC 4551) | `HIGHESTMODSEQ` ile delta senkron | M2 | Planlı |
| **QRESYNC (RFC 7162)** | Kopan bağlantıdan hızlı toparlanma | M2 | **Thunderbird'de yok.** go-imap v2 destekliyor — bizde artı |
| SPECIAL-USE (RFC 6154) / XLIST | `\Sent \Drafts \Trash \Archive \Junk` klasör rolleri | M5 | M6 gönderme için şart: "Gönderilenler" hangi klasör? |
| UIDPLUS (RFC 4315) | `APPENDUID` / `COPYUID` | M6 | Gönderilen kopyanın UID'sini öğrenmek |
| MOVE (RFC 6851) | Tek komutla taşıma | M3 | Yoksa COPY + STORE `\Deleted` + EXPUNGE |
| NAMESPACE (RFC 2342) | Kişisel / paylaşılan / diğer kullanıcı alanları | M8 | Kurumsal paylaşılan posta kutuları |
| ACL (RFC 4314) | Klasör başına yetki | M8 | Paylaşılan kutuda "neden yazamıyorum" sorusunun cevabı |
| QUOTA (RFC 2087) | Kota kullanımı ve limiti | M8 | Kota dolmadan uyarmak |
| COMPRESS=DEFLATE (RFC 4978) | Bağlantı sıkıştırma | M8 | Yavaş bağlantıda gözle görülür |
| LITERAL+ (RFC 2088), ENABLE (RFC 5161) | Protokol verimliliği | M8 | go-imap zaten kullanıyor |
| LIST-EXTENDED (RFC 5258) | Tek komutta abonelik + öznitelik | M8 | |
| UTF8=ACCEPT (RFC 6855) | UTF-8 klasör adları | M5 | Türkçe klasör adları için doğrudan ilgili |
| Gmail X-GM-EXT-1 | `X-GM-LABELS`, `X-GM-THRID`, `X-GM-MSGID` | M8 | Gmail etiketlerini gerçek etiket olarak göstermek |
| ID (RFC 2971), CLIENTID | İstemci kimliğini sunucuya bildirme | — | Gizlilik açısından da istenmez |
| XSERVERINFO, XSENDER, LANGUAGE | Sunucuya özgü | — | Niş |

Diğer IMAP davranışları:

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Bağlantı havuzu | Hesap başına bağlantı sayısı, boşta izleme, yeniden kullanım | `mailnews/imap/public/nsIImapIncomingServer.idl` | M2 |
| Silme modelleri | Çöpe taşı / `\Deleted` işaretle / hemen sil | `nsIImapIncomingServer.idl` (`nsMsgImapDeleteModels`) | M3 |
| Sunucu tarafı SEARCH | Yerelde olmayan eski maili sunucuda aramak | `mailnews/search/src/nsMsgSearchImap.h` | M8 |
| Çevrimdışı işlem kaydı ve oynatma | Çevrimdışıyken yapılan bayrak/taşı/sil işlemleri | `mailnews/imap/src/nsImapOfflineSync.cpp` | M3 |
| AutoSync — arka planda seçmeli indirme | Yaş/boyut ölçütleriyle klasör ve mesaj stratejileri | `mailnews/imap/public/nsIAutoSyncManager.idl` | M2 |
| Parça parça gövde indirme | Büyük gövdeyi bölerek çekmek | `nsIImapIncomingServer.idl` (`fetchByChunks`) | M5 |

### 1.2 Diğer protokoller

| Protokol | Yol | KT | Karar |
|---|---|---|---|
| SMTP (STARTTLS, 8BITMIME, SIZE, SMTPUTF8, DSN) | `mailnews/compose/src/SmtpClient.sys.mjs` | M6 | `emersion/go-smtp` |
| CardDAV | `mailnews/addrbook/modules/CardDAVDirectory.sys.mjs` | M7 | `emersion/go-webdav` |
| LDAP (bind, arama, BER, kontroller, URL) | `mailnews/addrbook/modules/LDAPClient.sys.mjs` | M7 | `go-ldap/ldap/v3` |
| LDAP çevrimdışı kopyası | `mailnews/addrbook/public/nsIAbLDAPReplicationService.idl` | M7 | Yerel-önce ilkesinin kişilere uygulanmış hali |
| CalDAV | `calendar/providers/caldav/CalDavCalendar.sys.mjs` | M9 | `emersion/go-webdav` |
| ICS / webcal | `calendar/providers/ics/CalICSCalendar.sys.mjs` | M9 | `emersion/go-ical` |
| Exchange EWS + Microsoft Graph | `mailnews/protocols/exchange/src/IExchangeClient.idl` | M10 | Posta IMAP'ten çalışıyor; asıl kazanç takvim ve kişiler |
| POP3 | `mailnews/local/src/Pop3Client.sys.mjs` | — | Sunucu tarafı durum yok; yerel-önce modelimizle çelişiyor |
| NNTP / haber grupları | `mailnews/news/src/NntpClient.sys.mjs` | — | Eski yük |
| RSS / Atom | `mailnews/extensions/newsblog/FeedParser.sys.mjs` | — | Tutarlı ama e-posta değil; talep gelirse ayrı değerlendirilir |
| MAPI sunucusu (Windows "E-posta ile gönder") | `mailnews/mapi/mapihook/` | — | Windows'a özgü COM yüzeyi |
| JMAP | — | — | Gmail/Outlook desteklemiyor; `MailBackend` arayüzü kapıyı açık tutuyor |

---

## 2. Hesap kurulumu ve kimlikler

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Hesap Merkezi (Account Hub) | E-posta, takvim, kişi, sohbet, içe aktarma için tek akış | `mail/components/accountcreation/content/accountHub.js` | M6 |
| Otomatik yapılandırma | ISPDB, diskteki tanım, DNS MX/SRV, sunucu tahmini | `mail/components/accountcreation/modules/FetchConfig.sys.mjs`, `GuessConfig.sys.mjs` | M6 |
| Exchange autodiscover | EWS/Graph hesabı tespiti | `mail/components/accountcreation/modules/ExchangeAutoDiscover.sys.mjs` | M10 |
| Yapılandırmayı doğrula | Hesabı oluşturmadan önce sunucuya bağlanıp sınamak | `mail/components/accountcreation/modules/ConfigVerifier.sys.mjs` | M6 |
| **Hesap başına çoklu kimlik** | Ad, adres, reply-to, imza, HTML tercihi | `mailnews/base/public/nsIMsgIdentity.idl` | M6 |
| Kimlik başına: Fcc/Taslak/Şablon/Arşiv klasörü | Gönderilen kopya nereye | `nsIMsgIdentity.idl` (`fccFolderURI`) | M6 |
| Kimlik başına: kendine CC/BCC, catch-all adresler | | `nsIMsgIdentity.idl` | M6 |
| Kimlik başına: şifreleme politikası, imzalama, anahtar ekleme | | `nsIMsgIdentity.idl` (`encryptionPolicy`, `attachPgpKey`) | M9 |
| Hesap rengi | Klasör ve liste panelinde hesabı ayırt etmek | `mail/modules/AccountColorUtils.sys.mjs` | M8 |
| Klasör aboneliği (IMAP) | Sunucudaki klasörlerin hangisi görünsün | `mailnews/base/public/nsISubscribableServer.idl` | M8 |
| Çevrimiçi / çevrimdışı anahtarı | Elle çevrimdışına geçme, "şimdi eşitle" | `mailnews/base/public/nsIMsgOfflineManager.idl` | M2 |
| Özel OAuth2 ayrıntıları | Kendi client id / uç nokta / kapsam | `mailnews/base/src/OAuth2CustomDetails.sys.mjs` | ✔ M1 |
| Hesabı kaldır (veriyi tut / sil seçeneği) | | `mail/locales/en-US/messenger/removeAccount.ftl` | M5 |

---

## 3. Klasör ve mesaj listesi

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Üç sütun düzeni | Klasör / liste / okuma | `mail/base/content/about3Pane.js` | ✔ M1 |
| Düzen modları | Klasik / geniş / dikey | `mail/base/content/mainCommandSet.inc.xhtml` | M12 |
| Klasör paneli modları | Tümü / birleşik / okunmamış / favori / son / etiket | `mail/base/content/about3Pane.js` | M8 |
| **Birleşik (smart) klasörler** | Hesaplar arası tek Gelen Kutusu, Gönderilmiş, Çöp | `mail/modules/SmartMailboxUtils.sys.mjs` | M8 |
| Klasör işlemleri | Yeni, yeniden adlandır, sil, çöpü boşalt, tümünü okundu işaretle | `mail/base/content/modules/FolderCommands.mjs` | M5 |
| Klasör özellikleri: saklama politikası | Klasör başına "N günden eski olanı sil" | `mailnews/base/content/RetentionSettingsUI.mjs` | M2 |
| Sanallaştırılmış liste | 50.000 mesajda sabit DOM | `mail/base/content/widgets/tree-view.mjs` | ✔ M1 |
| Liste sütunları | Konu, gönderen, tarih, boyut, etiket, ek, junk, hesap, konum… | `mail/base/content/modules/ThreadPaneColumns.mjs` | M8 |
| Sıralama | Her sütuna göre, varsayılan sıralamayı klasörlere uygula | `mail/base/content/messenger-menubar.inc.xhtml` | M8 |
| **Konuşma gruplama (threading)** | `References`/`In-Reply-To` zinciri | `mailnews/base/src/nsMsgThreadedDBView.cpp` | M5 |
| Gruplayarak sıralama | Tarihe/gönderene/etikete göre başlıklı öbekler | `mailnews/base/src/nsMsgGroupView.cpp` | M8 |
| Kart görünümü | Çok satırlı satırlar, avatar, etiket | `mail/base/content/widgets/treeview/thread-card.mjs` | M12 |
| Mesaj listesi görünümleri | Tümü / okunmamış / etiketli / özel kayıtlı görünüm | `mail/extensions/mailviews/` | M8 |
| Gezinme | Sonraki/önceki okunmamış, sonraki yıldızlı, geri/ileri | `mail/base/content/msgViewNavigation.js` | M5 |
| Çoklu mesaj özeti | Seçili mesajların özet görünümü | `mail/base/content/multimessageview.js` | — |

---

## 4. Okuma ve görüntüleme

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| İzole HTML render | Mail gövdesini uygulamadan ayırmak | `mail/base/docs/mail_display.md` | ✔ M1 |
| Uzak içerik engelleme | İzleyici pikselleri, uzak CSS | `mailnews/base/src/nsMsgContentPolicy.cpp` | ✔ M1 |
| Mesaj başlığı paneli | Konu, gönderen, tarih, alıcılar | `mail/base/content/msgHdrView.js` | ✔ M1 |
| Başlık görüntüleme kipleri | Mikro / normal / tüm başlıklar | `mailnews/mime/public/nsIMimeEmitter.idl` | M5 |
| Posta listesi başlıkları | `List-Unsubscribe`, `List-Post`, `List-Archive` | `mail/base/content/msgHdrView.inc.xhtml` | M8 |
| Gövde kipi | Özgün HTML / sade HTML / düz metin | `mail/base/content/messenger-menubar.inc.xhtml` (`viewBodyMenu`) | M5 |
| **Ekleri indir / aç / kaydet / ayır** | | `mail/base/content/msgAttachmentView.inc.xhtml`, `mail/modules/AttachmentInfo.sys.mjs` | M5 |
| Satır içi ek gösterimi | Resimleri gövde içinde göstermek | `mail/base/content/msgHdrView.js` | M5 |
| Kaynağı görüntüle | Ham RFC 5322 | `mail/base/content/viewSource.js` | M5 |
| `.eml` olarak kaydet | | `mailnews/base/public/nsIMessenger.idl` (`saveAs`) | M5 |
| Yazdır | | `mail/base/content/printUtils.js` | M5 |
| **Kodlamayı onar** | Yanlış çözülmüş mesaj için charset seçici | `mail/base/content/messenger-menubar.inc.xhtml` (`repair-text-encoding-button`) | M5 |
| Mesaj gövdesinde karanlık mod | HTML maili koyu temaya uydurmak | `mail/base/content/modules/DarkReader.mjs` | M5 |
| Mesajda bul | Gövde içinde arama çubuğu | `mail/base/content/widgets/lazy-findbar.mjs` | M5 |
| Okundu işaretleme davranışı | Görüntülemede / gecikmeli / açınca / tarihe göre | `mail/components/preferences/general.inc.xhtml` | M5 |
| Mesaj güvenlik paneli | İmza ve şifreleme durumu | `mail/base/content/msgSecurityPane.js` | M9 |
| Kimlik avı tespiti | Bağlantı metni ≠ hedef uyarısı | `mail/modules/PhishingDetector.sys.mjs` | M8 |

### 4.1 MIME çözümleme

| Özellik | Yol | KT |
|---|---|---|
| multipart/mixed, /alternative, /related, /digest, /parallel | `mailnews/mime/src/mimemmix.cpp`, `mimemalt.cpp`, `mimemrel.cpp` | ✔ M1 (go-message) |
| message/rfc822 iç içe mesaj | `mailnews/mime/src/mimemsg.cpp` | M5 |
| format=flowed düz metin (RFC 3676) | `mailnews/mime/src/mimetpfl.cpp` | M5 |
| Aktarım kodlamaları: base64, QP, uuencode, yEnc | `mailnews/mime/src/mimeenc.cpp` | ✔ M1 kısmi (uuencode/yEnc yok) |
| RFC 2047 kodlanmış başlık sözcükleri | `mailnews/mime/public/nsIMimeConverter.idl` | ✔ M1 |
| Adres başlığı ayrıştırma (grup sözdizimi dahil) | `mailnews/mime/public/nsIMsgHeaderParser.idl` | ✔ M1 |
| Charset takma adları | `mailnews/intl/charsetalias.properties` | ✔ M1 |
| IMAP değiştirilmiş UTF-7 | `mailnews/intl/nsMUTF7ToUnicode.cpp` | ✔ M1 (go-imap) |
| PGP/MIME köprüsü | `mailnews/mime/public/nsIPgpMimeProxy.idl` | M9 |
| S/MIME MIME entegrasyonu | `mailnews/mime/src/mimecms.cpp`, `mimemcms.cpp` | M9 |

---

## 5. Yazma ve gönderme

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Compose türleri | Yeni, yanıtla, tümünü yanıtla, listeye yanıtla, ilet (satır içi/ek), yönlendir, yeni olarak düzenle, taslak, şablon | `mailnews/compose/public/nsIMsgComposeParams.idl` | M6 |
| Teslim kipleri | Şimdi, sonra (outbox), taslak kaydet, şablon kaydet, otomatik taslak | `mailnews/compose/public/nsIMsgSend.idl` | M6 |
| Gönderim biçimi | Otomatik / düz metin / HTML / her ikisi | `mailnews/compose/public/nsIMsgCompose.idl` | M6 |
| Compose alanları | To/Cc/Bcc/Reply-To/Fcc/Subject/References/Priority + rastgele başlık | `mailnews/compose/public/nsIMsgCompFields.idl` | M6 |
| Alıcı "pill" arayüzü | Silinebilir alıcı rozetleri, satırlar arası taşıma | `mail/components/compose/content/addressingWidgetOverlay.js` | M6 |
| Alıcı otomatik tamamlama | Adres defterlerinden ve toplanan adreslerden | `mailnews/addrbook/src/AbAutoCompleteSearch.sys.mjs` | M6 |
| Adres toplama | Yazdığınız kişileri deftere ekle | `mailnews/compose/src/AddressCollector.sys.mjs` | M6 |
| Zengin metin editörü | Kalın/italik, başlık, liste, hizalama, renk | `mail/components/compose/content/ComposerCommands.js` | M6 |
| Alıntılama | Özgün gövdeyi alıntıla, yanıt konumu, alıntı temizleme | `mailnews/compose/public/nsIMsgQuote.idl`, `mail/modules/QuoteSanitizer.sys.mjs` | M6 |
| İmzalar | Kimlik başına metin/HTML/dosya | `mailnews/base/public/nsIMsgIdentity.idl` | M6 |
| Ekler | Dosya, web sayfası, vCard, OpenPGP açık anahtarı | `mailnews/compose/public/nsIMsgAttachment.idl` | M6 |
| Gömülü resimler (`cid:`) | Satır içi resimleri MIME parçasına çevirme | `mailnews/compose/public/nsIMsgSend.idl` | M6 |
| **Ek hatırlatıcı** | Gövde "ekte" diyor ama ek yok uyarısı | `mail/modules/AttachmentChecker.worker.js` | M6 |
| Taslak otomatik kaydetme | N dakikada bir | `mail/components/preferences/compose.inc.xhtml` | M6 |
| **Outbox / sonra gönder** | Kuyruğa al, bağlantı gelince gönder | `mailnews/compose/public/nsIMsgSendLater.idl` | M6 |
| Fcc — gönderilen kopya | Hangi klasöre | `mailnews/compose/public/nsIMsgCopy.idl` | M6 |
| Gönderim raporu | Hangi aşamada hata: MIME kurma / SMTP / kopyalama / filtre | `mailnews/compose/public/nsIMsgSendReport.idl` | M6 |
| Yazım denetimi | Yazarken, göndermeden önce, sözlük yönetimi | `mail/components/compose/content/dialogs/EdSpellCheck.xhtml` | M6 (WebView'in kendi denetimi) |
| Öncelik | En yüksek → en düşük | `mailnews/compose/public/nsIMsgCompFields.idl` | M8 |
| DSN (teslim durumu bildirimi) | | `mailnews/compose/src/SmtpClient.sys.mjs` | M8 |
| **MDN (okundu bilgisi)** | İstek gönderme + gelen isteğe yanıt politikası | `mailnews/extensions/mdn/nsMsgMdnGenerator.cpp` | M8 |
| `mailto:` işleyici | Sistemden gelen `mailto:` bağlantıları | `mailnews/compose/src/MailtoProtocolHandler.sys.mjs` | M6 |
| Bulut ek (FileLink) | Büyük eki üçüncü tarafa yükleyip bağlantı koymak | `mail/components/cloudfile/` | — Gizlilik iddiasıyla çelişir |
| Tablo / matematik (TeXZilla) editörü | | `mail/components/compose/texzilla/` | — Aşırı kapsam |

---

## 6. Arama ve filtreleme

### 6.1 Arama

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Tam metin indeksi | Konu / gönderen / snippet | `mailnews/db/gloda/modules/GlodaDatastore.sys.mjs` | ✔ M1 |
| **Gövde indeksi** | Mesaj gövdelerini de aramak | `mailnews/db/gloda/modules/GlodaMsgIndexer.sys.mjs` | M8 |
| Kök bulma (stemming) tokenizer | FTS3 + Porter stemmer | `mailnews/extensions/fts3/fts3_porter.c` | M8 — **Türkçe için ayrı karar** |
| Hesaplar arası küresel arama | | `mailnews/db/gloda/modules/GlodaMsgSearcher.sys.mjs` | M8 |
| Facet'li sonuç sekmesi | Gönderen / etiket / klasör / zaman çizelgesi | `mail/base/content/glodaFacetView.js` | M8 |
| Gelişmiş arama diyaloğu | Çok ölçütlü, çok klasörlü | `mail/base/content/SearchDialog.js` | M8 |
| Arama nitelikleri (30+) | Konu, gönderen, gövde, tarih, boyut, etiket, junk skoru, ek durumu, **rastgele başlık**, klasör bayrağı | `mailnews/search/public/nsMsgSearchCore.idl` | M8 |
| Arama operatörleri | contains, is, beginsWith, endsWith, isBefore, isGreaterThan, matches, isInAB… | `mailnews/search/public/nsMsgSearchCore.idl` | M8 |
| Geçerlilik tabloları | Hangi nitelik hangi kapsamda hangi operatörle kullanılabilir | `mailnews/search/public/nsIMsgSearchValidityManager.idl` | M8 |
| Sunucu tarafı IMAP SEARCH | Yerelde olmayan eski maili sunucuda aramak | `mailnews/search/src/nsMsgSearchImap.h` | M8 |
| Özel arama terimleri | Eklentinin kendi yüklemi | `mailnews/search/public/nsIMsgSearchCustomTerm.idl` | — |
| Hızlı filtre çubuğu | Okunmamış/yıldızlı/ekli/etiket + metin, yapışkan | `mail/base/content/quickFilterBar.js` | M8 |
| İşletim sistemi arama entegrasyonu | Windows Search / Spotlight | `mail/components/search/` | — Postayı OS indeksine verir |

### 6.2 Filtreler (kural motoru)

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Kural listesi | Sunucu/klasör başına sıralı kurallar | `mailnews/search/public/nsIMsgFilterList.idl` | M8 |
| **Çalışma anları** | InboxRule, Manual, **PostPlugin** (junk sınıflandırmasından sonra), **PostOutgoing**, **Archive**, **Periodic** | `mailnews/search/public/nsMsgFilterCore.idl` | M8 |
| Eylemler | Taşı/kopyala, öncelik, sil, okundu/okunmadı, yıldız, etiket, konuşmayı yoksay/izle, şablonla yanıtla, ilet, **yürütmeyi durdur**, junk skoru | `mailnews/search/public/nsMsgFilterCore.idl` | M8 |
| Kuralı mesajdan üret | Seçili mesajdan önceden doldurulmuş kural | `mail/base/content/mainCommandSet.inc.xhtml` | M8 |
| Elle çalıştır | Klasöre veya seçime kuralları uygula | `mailnews/search/public/nsIMsgFilterService.idl` | M8 |
| **Filtre günlüğü** | "Kural neden çalışmadı" sorusunun tek cevabı | `mailnews/search/public/nsIMsgFilterList.idl` | M8 |
| Özel eylemler | Eklentinin kendi eylemi | `mailnews/search/public/nsIMsgFilterCustomAction.idl` | — |

### 6.3 Junk

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Bayes sınıflandırıcı | Naive-Bayes, token eğitimi, chi-square birleştirme | `mailnews/extensions/bayesian-spam-filter/nsBayesianFilter.cpp` | M8 |
| Eğitim | "Spam" / "spam değil" ile öğretmek | `mailnews/search/public/nsIMsgFilterPlugin.idl` | M8 |
| Eğitim verisini sıfırla / dışa aktar | Kullanıcının kendi verisi üzerinde kontrolü | `nsIMsgCorpus` (`nsIMsgFilterPlugin.idl`) | M8 |
| Adres defteri beyaz listesi | Tanıdıklardan gelen asla junk değil | `mailnews/base/public/nsISpamSettings.idl` | M8 |
| Sunucu başlıklarına güven | SpamAssassin / Rspamd başlıkları | `mailnews/base/public/nsISpamSettings.idl` | M8 |
| Junk klasörüne taşı, okundu işaretle, N gün sonra sil | | `mailnews/base/src/nsSpamSettings.cpp` | M8 |
| Nitelik (trait) servisi | Junk dışında rastgele sınıflandırma etiketleri | `mailnews/search/public/nsIMsgTraitService.idl` | — |

---

## 7. Organizasyon

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Etiketler | Renkli, 1–9 kısayolu, yönetilebilir liste | `mailnews/base/public/nsIMsgTagService.idl` | M8 |
| Yıldız / bayrak | | `mailnews/base/public/nsMsgMessageFlags.idl` | M5 |
| Arşivle | Tarih bazlı arşiv klasörleri, kimlik başına ayrıntı düzeyi | `mail/modules/MessageArchiver.sys.mjs` | M8 |
| Taşı / kopyala + "yine taşı" | Son ve favori hedefler | `mail/base/content/mainCommandSet.inc.xhtml` | M5 |
| **Sanal klasörler (kayıtlı arama)** | Arama sonucunu klasör gibi göstermek | `mailnews/base/public/nsIVirtualFolderWrapper.idl` | M8 |
| Favori klasörler | | `mail/base/content/about3Pane.js` | M8 |
| Konuşmayı yoksay / izle | Gürültülü konuşmayı susturmak | `mailnews/base/public/nsMsgMessageFlags.idl` | M8 |
| Geri al / yinele | Sil, taşı, kopyala, tümünü okundu işaretle | `mailnews/base/public/nsIMsgTxn.idl` | M5 |

---

## 8. Kişiler

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Adres defteri alanı | Dizinler, kişiler, listeler | `mail/components/addrbook/content/aboutAddressBook.js` | M7 |
| Yerel adres defteri (SQLite) | | `mailnews/addrbook/modules/SQLiteDirectory.sys.mjs` | M7 |
| vCard düzenleyici | Ad, e-posta, telefon, adres, URL, IM, kurum, özel tarihler, zaman dilimi | `mail/components/addrbook/content/vcard-edit/` | M7 |
| vCard dönüşümü | | `mailnews/addrbook/modules/VCardUtils.sys.mjs` | M7 |
| CardDAV senkronu | Keşif, sync-collection, ctag/etag | `mailnews/addrbook/modules/CardDAVDirectory.sys.mjs` | M7 |
| LDAP dizini | Base DN, kapsam, öznitelik eşlemesi, SASL | `mailnews/addrbook/public/nsIAbLDAPDirectory.idl` | M7 |
| LDAP çevrimdışı kopyası | Dizini yerele indirir | `mailnews/addrbook/public/nsIAbLDAPReplicationService.idl` | M7 |
| Posta listeleri | Dağıtım grubu; compose'da açılır | `mailnews/addrbook/modules/AddrBookMailingList.sys.mjs` | M7 |
| Kişi arama | Yapılandırılmış boolean sorgu | `mailnews/addrbook/public/nsIAbDirectoryQuery.idl` | M7 |
| Compose'da kişi kenar çubuğu | | `mail/components/addrbook/content/abContactsPanel.xhtml` | M7 |
| Mesaj başlığından kişi ekle | | `mail/base/content/editContactPanel.js` | M7 |
| Kişi avatarları | | `mail/base/content/widgets/treeview/contact-avatar.mjs` | M7 — yalnızca yerel/vCard fotoğrafı, uzak servis yok |
| vCard eki tanıma | Maildeki vCard'ı içe aktarma | `mail/actors/VCardChild.sys.mjs` | M7 |
| İşletim sistemi adres defterleri | macOS Contacts, Windows Outlook/MAPI | `mailnews/addrbook/src/nsAbOSXDirectory.mm`, `nsAbOutlookDirectory.cpp` | — |

---

## 9. Güvenlik ve gizlilik

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| OpenPGP: şifrele/imzala/çöz/doğrula | RNP tabanlı | `mail/extensions/openpgp/content/modules/RNP.sys.mjs` | M9 |
| GnuPG (GPGME) desteği | Harici gpg kullanımı | `mail/extensions/openpgp/content/modules/GPGME.sys.mjs` | — |
| Anahtar yöneticisi | Listele, içe/dışa aktar, yedekle, iptal et, süre değiştir | `mail/extensions/openpgp/content/ui/enigmailKeyManager.js` | M9 |
| Anahtar oluşturma sihirbazı | | `mail/extensions/openpgp/content/ui/keyWizard.js` | M9 |
| **Anahtar asistanı** | Compose'da alıcı başına anahtar durumu | `mail/extensions/openpgp/content/ui/keyAssistant.js` | M9 |
| WKD anahtar keşfi | Alan adından anahtar bulma | `mail/extensions/openpgp/content/modules/wkdLookup.sys.mjs` | M9 |
| Anahtar sunucusu sorgusu | | `mail/extensions/openpgp/content/modules/keyserver.sys.mjs` | M9 opsiyonel — meta veri sızdırır |
| Konu satırını şifrele | | `mail/base/content/messenger-menubar.inc.xhtml` | M9 |
| S/MIME | CMS imzala/şifrele, sertifika seçimi | `mailnews/extensions/smime/nsICMSMessage.idl` | M9 |
| Compose güvenlik kancası | İmzalama/şifrelemenin takıldığı yer | `mailnews/compose/public/nsIMsgComposeSecure.idl` | M9 |
| Ana parola | Saklanan kimlik bilgilerini parolayla korumak | `mail/modules/PrimaryPassword.sys.mjs` | ✔ M1 (Argon2id dosya deposu) |
| Parola yöneticisi | Kayıtlı parolaları görüntüle/sil | `mail/components/preferences/passwordManager.js` | M12 |
| Sertifika yönetimi, OCSP | | `mail/components/preferences/privacy.inc.xhtml` | M9 |
| Çerez ve web içeriği denetimi | | `mail/components/preferences/cookies.js` | — Bizde mail içeriği çerez alamıyor |
| DNS-over-HTTPS | | `mail/components/preferences/privacy.inc.xhtml` | — |
| Kurumsal politikalar | Yöneticinin JSON ile kilitlediği ayarlar | `mail/components/enterprisepolicies/` | — |
| **Telemetri** | Kullanım ölçümü | `mail/metrics.yaml` | — **Asla.** |
| DKIM / SPF / ARC göstergesi | Gönderen doğrulaması | — (Thunderbird'de eklenti) | M9 — **bizde artı** |

---

## 10. Takvim ve görevler

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Yerel takvim (SQLite) | | `calendar/providers/storage/CalStorageCalendar.sys.mjs` | M9 |
| Çevrimdışı öğe modeli | Ağ takvimi için bekleyen yerel değişiklikler | `calendar/providers/storage/CalStorageOfflineModel.sys.mjs` | M9 |
| CalDAV | Keşif, ACL, ctag/etag, sunucu tarafı planlama | `calendar/providers/caldav/CalDavCalendar.sys.mjs` | M9 |
| ICS / webcal aboneliği | | `calendar/providers/ics/CalICSCalendar.sys.mjs` | M9 |
| Birleşik takvim görünümü | Birden çok takvimi tek görünümde | `calendar/providers/composite/CalCompositeCalendar.sys.mjs` | M9 |
| Microsoft Graph takvimi | | `calendar/providers/graph/GraphCalendar.sys.mjs` | M10 |
| Etkinlik (VEVENT) | | `calendar/base/public/calIEvent.idl` | M9 |
| Görev (VTODO) | Giriş/bitiş/tamamlanma, yüzde | `calendar/base/public/calITodo.idl` | M9 |
| **Yinelenme** | RRULE + RDATE + EXDATE + tekil istisnalar | `calendar/base/public/calIRecurrenceInfo.idl` | M9 — en zor parça |
| Hatırlatıcılar | DISPLAY/EMAIL/AUDIO, göreli veya mutlak, erteleme | `calendar/base/public/calIAlarm.idl` | M9 |
| Alarm servisi ve diyaloğu | Tüm takvimlerde tetikleme, ertele/kapat | `calendar/base/src/CalAlarmService.sys.mjs` | M9 |
| Katılımcılar | Rol, PARTSTAT, RSVP, CUTYPE, devretme | `calendar/base/public/calIAttendee.idl` | M9 |
| Zaman dilimleri | IANA, alan başına seçici | `calendar/base/public/calITimezoneService.idl` | M9 |
| Kategoriler | Renkli | `calendar/base/modules/utils/calCategoryUtils.sys.mjs` | M9 |
| iCalendar okuma/yazma | RFC 5545 | `calendar/base/public/calIICSService.idl` | M9 |
| **iTIP işleme** | REQUEST/REPLY/CANCEL/REFRESH/COUNTER (RFC 5546) | `calendar/base/modules/utils/calItipUtils.sys.mjs` | M9 |
| iMIP e-posta taşıması | Daveti `text/calendar` olarak göndermek | `calendar/itip/CalItipEmailTransport.sys.mjs` | M9 |
| **Postada davet çubuğu** | Kabul / Belki / Reddet | `calendar/base/content/imip-bar.js` | M9 — kurumsalda en görünür özellik |
| Davet görüntüleme paneli | Değişiklikleri vurgulayan zengin gösterim | `calendar/base/content/calendar-invitation-display.js` | M9 |
| Bekleyen davetler yöneticisi | CalDAV planlama gelen kutusunu yoklamak | `calendar/base/content/calendar-invitations-manager.js` | M9 |
| Serbest/meşgul sorgusu | | `calendar/base/public/calIFreeBusyProvider.idl` | M10 |
| Görünümler | Gün, hafta, çok hafta, ay | `calendar/base/content/calendar-views.js` | M9 |
| Görev görünümü | Sıralanabilir ağaç, filtre, satır içi tamamlama | `calendar/base/content/calendar-task-view.js` | M9 |
| Hızlı görev girişi | | `calendar/base/content/item-editing/calendar-task-editing.js` | M9 |
| Bugün paneli / ajanda | | `calendar/base/content/today-pane.js` | M9 |
| Etkinlik arama (unifinder) | | `calendar/base/content/calendar-unifinder.js` | M9 |
| Yinelenen öğede "bu / tümü" sorusu | | `calendar/base/content/dialogs/calendar-occurrence-prompt.js` | M9 |
| Çakışma çözümü | Sunucuyla eşzamanlı değişiklik | `calendar/base/content/dialogs/calendar-conflicts-dialog.js` | M9 |
| Çevrimiçi toplantı bağlantısı | Etkinlikteki katılım URL'sini bulmak | `mail/components/calendar/modules/JoinLinkParser.sys.mjs` | M9 |
| ICS içe / dışa aktarma | | `calendar/import-export/CalIcsImportExport.sys.mjs` | M9 |
| Takvim yazdırma | | `calendar/base/content/calendar-print.js` | M10 |
| Takvim yayınlama (WebDAV PUT) | | `calendar/base/content/publish.js` | — Niş |
| Postadan etkinlik çıkarma | Metinden tarih/saat tanıma | `calendar/base/modules/calExtract.sys.mjs` | — Yerelleştirmesi ağır, doğruluğu düşük |
| Geri al / yinele (takvim) | | `calendar/base/src/CalTransactionManager.sys.mjs` | M9 |

---

## 11. Sohbet

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Matrix | Homeserver'a bağlan, oda ve DM | `chat/protocols/matrix/matrix.sys.mjs` | M13 |
| Matrix uçtan uca şifreleme | | `chat/protocols/matrix/matrixAccount.sys.mjs` | M13 |
| Matrix cihaz/kullanıcı doğrulama | SAS/emoji doğrulama | `mail/components/im/content/verify.xhtml` | M13 |
| XMPP | Roster, 1:1, MUC | `chat/protocols/xmpp/xmpp.sys.mjs` | M13 |
| XMPP SASL (SCRAM-SHA-1/256) | | `chat/protocols/xmpp/xmpp-authmechs.sys.mjs` | M13 |
| OTR şifreleme | 1:1 konuşmalar için | `chat/modules/OTR.sys.mjs` | M13 |
| OTR doğrulama | Paylaşılan sır / soru-cevap / parmak izi | `chat/content/otr-auth.xhtml` | M13 |
| Konuşma günlüğü | Yerel JSON, otomatik temizleme | `chat/components/src/logger.sys.mjs` | M13 |
| Kişi listesi, gruplar, durum | | `chat/components/src/imContacts.sys.mjs` | M13 |
| Mesaj temizleme | Gelen mesajın güvenli HTML'i | `chat/modules/imContentSink.sys.mjs` | M13 |
| Bildirimler | | `mail/components/im/modules/chatNotifications.sys.mjs` | M13 |
| IRC (+CAP, SASL, CTCP, DCC, servisler) | | `chat/protocols/irc/irc.sys.mjs` | — Eski yük; şifreleme yok |
| Facebook / Twitter / Yahoo | | `chat/protocols/facebook/facebook.sys.mjs` | — Thunderbird'de zaten ölü saplama |
| Google Talk, Odnoklassniki | | `chat/protocols/gtalk/gtalk.sys.mjs` | — Ölü / niş |
| Konuşma temaları | bubbles, dark, mail, papersheets, simple | `chat/modules/imThemes.sys.mjs` | — |

---

## 12. Sistem entegrasyonu ve bildirimler

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Yeni posta bildirimi | Masaüstü bildirimi, uyarı penceresi | `mailnews/base/src/MailNotificationManager.sys.mjs` | M4 |
| Yeni posta sesi | Sistem sesi veya özel dosya | `mail/modules/NotificationSounds.sys.mjs` | M4 |
| Biff (yoklama zamanlayıcı) | Sunucu başına yeni posta kontrolü | `mailnews/base/public/nsIMsgBiffManager.idl` | M2 |
| Sistem tepsisi | Tepsiye küçült, tepside başlat, okunmamış rozeti | `mail/base/content/closeToTray.mjs` | M4 |
| Windows okunmamış rozeti | | `mailnews/base/src/WinUnreadBadge.sys.mjs` | M4 |
| Görev çubuğu ilerlemesi | | `mail/modules/TaskbarProgress.sys.mjs` | M4 |
| Windows Jump List | | `mail/modules/WindowsJumpLists.sys.mjs` | M4 |
| macOS dock rozeti | | `mail/components/preferences/dockoptions.js` | M4 |
| Varsayılan posta istemcisi | | `mail/components/shell/` | M4 |
| **Etkinlik yöneticisi** | Senkron, gönderme, filtre işlemlerinin günlüğü | `mail/components/activity/` | M8 |
| Bağlantı hatası bildirimi | | `mail/modules/ConnectionNotifications.sys.mjs` | ✔ M1 |
| Kapanışta yapılacaklar | Çöpü boşalt, sıkıştır, bekleyeni gönder | `mailnews/base/public/nsIMsgShutdown.idl` | M4 |
| İndirilenler | Mailden kaydedilen dosyalar | `mail/components/downloads/content/aboutDownloads.xhtml` | M5 |
| Oturum geri yükleme | Sekme ve panel durumu | `mail/modules/SessionStore.sys.mjs` | M12 |
| Yazılım güncelleme | | `mail/base/content/aboutDialog-appUpdater.js` | M14 |
| Uygulama içi ürün bildirimleri | Bağış, sürüm notu, anket | `mail/components/inappnotifications/` | — Pazarlama kanalı |

---

## 13. İçe / dışa aktarma ve göç

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Thunderbird / SeaMonkey profili | Hesap, posta, kişi, ayar | `mailnews/import/modules/ThunderbirdProfileImporter.sys.mjs` | M11 |
| Outlook (MAPI + RTF gövde) | | `mailnews/import/src/nsOutlookImport.cpp`, `rtfDecoder.cpp` | M11 |
| Apple Mail (`.emlx`) | | `mailnews/import/src/nsAppleMailImport.cpp` | M11 |
| mbox deposu | | `mailnews/local/src/nsMsgBrkMBoxStore.cpp` | M11 — yalnızca kaynak |
| maildir deposu | | `mailnews/local/src/nsMsgMaildirStore.cpp` | M11 — yalnızca kaynak |
| Kişi dosyası (CSV/LDIF/vCard/Mork) | CSV alan eşleme arayüzü dahil | `mailnews/import/modules/AddrBookFileImporter.sys.mjs` | M11 |
| Ayar içe aktarma | Başka istemcinin hesap ayarları | `mailnews/import/public/nsIImportSettings.idl` | M11 |
| Filtre içe aktarma | | `mailnews/import/public/nsIImportFilters.idl` | M11 |
| Takvim dosyası içe aktarma | | `mailnews/import/modules/CalendarFileImporter.sys.mjs` | M9 |
| Profil dışa aktarma | Veriniz çıkabiliyor | `mailnews/export/modules/ProfileExporter.sys.mjs` | M11 |
| Depo biçimi dönüştürme | mbox ↔ maildir | `mailnews/base/src/mailstoreConverter.sys.mjs` | — Tek depomuz var |
| QR ile mobile aktarma | | `mail/modules/QRExport.sys.mjs` | — Mobil istemcimiz yok |

---

## 14. Yerelleştirme, erişilebilirlik, görünüm

| Özellik | Ne yapar | Yol | KT |
|---|---|---|---|
| Arayüz dili değiştirme | Dil paketi kur, sırala, çalışırken değiştir | `mail/components/preferences/messengerLanguages.js` | M12 |
| Tüm dizeler dışarıda | Fluent `.ftl` + eski DTD/properties | `mail/locales/en-US/messenger/` | M12 — **bizde bugün hiç yok** |
| Bölgesel tarih biçimi | | `mail/components/preferences/general.inc.xhtml` | M12 |
| Özelleştirilebilir kısayollar | | `mail/components/customizableshortcuts/` | M12 |
| Arayüz yoğunluğu | Sıkışık / varsayılan / geniş | `mail/modules/UIDensity.sys.mjs` | M12 |
| Arayüz font boyutu ve yakınlaştırma | | `mail/modules/UIFontSize.sys.mjs` | M12 |
| Caret (klavyeyle imleç) gezinme | | `mail/locales/en-US/messenger/messenger.ftl` | M12 |
| Erişilebilirlik test altyapısı | axe | `docs/testing/` | M12 |
| Açık / koyu / sistem teması | | `mail/themes/BuiltInThemes.sys.mjs` | ✔ M1 |
| Vurgu rengi seçimi | | `mail/components/preferences/appearance.inc.xhtml` | — Tek accent kararımız var |
| Font ve renk tercihleri | | `mail/components/preferences/fonts.js` | M12 |
| Platforma özgü tema | | `mail/themes/windows/`, `osx/`, `linux/` | — |

---

## 15. Genişletilebilirlik

| Özellik | Yol | KT |
|---|---|---|
| WebExtension API'leri (accounts, messages, compose, folders, addressBook, cloudFile, spaces, menus, theme…) | `mail/components/extensions/schemas/` | — Çok uzun vadeli |
| Eklenti yöneticisi | `mail/base/content/aboutAddonsExtra.js` | — |
| Takvim sağlayıcı arayüzü | `calendar/base/public/calICalendarProvider.idl` | — |
| Sohbet protokol arayüzü | `chat/components/public/prplIProtocol.idl` | — |
| JS ile hesap türü ekleme | `mailnews/jsaccount/public/msgIOverride.idl` | — Bizde `MailBackend` arayüzü aynı işi görüyor |
| Mozilla hesap senkronu | `mail/services/sync/modules/engines/` | — Ayarları sunucuya taşır; yerel-önce ile çelişir |

---

## 16. Arka uç altyapısı — bizim için mimari değeri olanlar

Bunlar kullanıcıya görünen özellik değil; Thunderbird'ün çözdüğü ve bizim de
çözmemiz gereken problemler.

| Konu | Thunderbird'de | Bizde |
|---|---|---|
| **Mesaj veritabanı** | Mork'tan SQLite'a geçiş (`mailnews/db/panorama/`), canlı SQL görünümleriyle | Baştan SQLite. Aynı yön. |
| **Protokolden bağımsız istemci arayüzü** | `mailnews/protocols/exchange/src/IExchangeClient.idl` — EWS ve Graph aynı arayüzü uyguluyor | `imapx.MailBackend`. Aynı desen. |
| **Çevrimdışı işlem kuyruğu** | `mailnews/imap/src/nsImapOfflineSync.cpp` + `nsIMsgOfflineImapOperation` | `operations` tablosu (M3) |
| **Seçmeli indirme / saklama** | AutoSync: yaş ve boyut ölçütleri, klasör ve mesaj stratejileri ayrı (`nsIAutoSyncManager.idl`) | Saklama penceresi (M2) |
| **Klasör önbelleği** | `folderCache.json` — her `.msf` açılmadan arayüz çizilebilsin diye (`mailnews/base/src/nsMsgFolderCache.cpp`) | Tek SQLite dosyası; ayrı önbellek gerekmiyor |
| **Sahte sunucuya karşı test** | IMAP, POP3, SMTP, NNTP, LDAP, EWS, Graph için betiklenebilir sunucular (`mailnews/test/fakeserver/`) | `imapmemserver`; M6'da SMTP için `go-smtp` sunucu tarafı |
| **Eklemeli depo soyutlaması** | `mailnews/base/public/nsIMsgPluggableStore.idl` — mbox ve maildir | **Gerekmiyor.** Tek depo (SQLite) seçildi; mbox/maildir yalnızca M11'de okunur |
| **Kök bulma (stemming)** | FTS3 + Porter (`mailnews/extensions/fts3/fts3_porter.c`) | FTS5 `unicode61`, kök bulma yok. Porter İngilizce için; **Türkçe sondan eklemeli**, M8'de ayrı karar |
| **Klasör sıkıştırma** | mbox'ta silinen yeri geri kazanma (`mailnews/base/src/FolderCompactor.cpp`) | SQLite `VACUUM`; farklı problem |
| **Veritabanı önbellek yönetimi** | Boştaki `.msf` dosyalarını kapatarak belleği sınırlama (`mailnews/base/src/MsgDBCacheManager.sys.mjs`) | Tek bağlantı havuzu; gerekmiyor |

---

## Bu envanterin sınırları

- **Kullanıcıya görünen özelliklere** odaklandı. Derleme sistemi, XPCOM
  altyapısı, Rust/C++ köprüleri kapsam dışı.
- Thunderbird'ün `docs/` ağacında **tek bir "özellik listesi" dokümanı yok**;
  bu envanter dizin yapısı, `.idl` arayüzleri, tercih panelleri ve
  `mots.yaml` modül haritası okunarak çıkarıldı.
- `suite/` (SeaMonkey, ~3.031 dosya) ayrı bir uygulama ağacı; taranmadı.

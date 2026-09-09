# OAuth kurulumu

Nexus Mail açık kaynak olduğu için kendi OAuth kimlik bilgileriyle gelmez.
İstemcinizi bir kez kaydedersiniz — birkaç dakika sürer ve posta kutunuza
erişimin sizin kontrolünüzde kalmasını sağlar.

Client ID'ler veri dizinindeki `config.json` dosyasına yazılır:

| Platform | Yol |
|---|---|
| Windows | `%APPDATA%\nexus-mail\config.json` |
| macOS | `~/Library/Application Support/nexus-mail/config.json` |
| Linux | `$XDG_DATA_HOME/nexus-mail/config.json` |

```json
{
  "googleClientId": "…apps.googleusercontent.com",
  "microsoftClientId": "…",
  "oauthRedirectPort": 0
}
```

**Client ID bir sır değildir**, bir tanımlayıcıdır: masaüstü uygulaması "public
client" sayılır ve kullanıcının makinesinde sır saklayamaz. Hassas olan kısım
refresh token'dır ve o bu dosyaya değil, işletim sisteminin anahtarlığına gider.

`oauthRedirectPort` varsayılan olarak `0`'dır; bu "rastgele port" demektir ve
doğru olan budur. Yalnızca güvenlik yazılımınız rastgele port bağlamayı
engelliyorsa sabit bir değer verin — o zaman aynı portu sağlayıcıya da
kaydetmeniz gerekir.

---

## Microsoft 365 ve Outlook.com

1. [Microsoft Entra yönetim merkezi](https://entra.microsoft.com) → **App
   registrations** → **New registration**.
2. İsim serbest. **Supported account types** altında *Accounts in any
   organizational directory and personal Microsoft accounts* seçin — böylece
   hem kurumsal hem Outlook.com adresleri giriş yapabilir.
3. **Redirect URI** altında **Public client/native** seçin ve `http://localhost`
   girin. Nexus Mail rastgele bir loopback portu dinler; Entra yerel
   istemcilerde port farkını yok sayar.
4. **Application (client) ID** değerini kopyalayıp `microsoftClientId` alanına
   yazın.
5. **API permissions** → **Add a permission** → **APIs my organization uses** →
   *Office 365 Exchange Online* → **Delegated permissions** altından
   `IMAP.AccessAsUser.All` ve `SMTP.Send` izinlerini ekleyin.

> SMTP client submission için temel kimlik doğrulama 30 Nisan 2026'da tamamen
> kapatıldı. Bu kurulum opsiyonel değil, zorunludur.

Kurumsal bir kiracıda çalışıyorsanız yöneticinizin izinleri onaylaması
gerekebilir.

---

## Gmail ve Google Workspace

1. [Google Cloud konsolu](https://console.cloud.google.com) → yeni proje
   oluşturun → **Gmail API**'yi etkinleştirin.
2. **OAuth consent screen** yapılandırın. *Testing* modunda bırakın ve **Test
   users** altına kendi adresinizi ekleyin. Testing modundaki bir uygulama 100
   kullanıcıya kadar izin verir; kendi hesaplarınız için fazlasıyla yeterli ve
   Google'ın doğrulama sürecini tamamen devre dışı bırakır.
3. **Credentials** → **Create credentials** → **OAuth client ID** →
   **Desktop app**.
4. Client ID'yi `googleClientId` alanına yazın.

> ⚠️ **İstemci türü kesinlikle "Desktop app" olmalıdır.** "Web application"
> seçerseniz Google yalnızca tam olarak kaydettiğiniz yönlendirme adreslerini
> (port dahil) kabul eder ve rastgele loopback portu
> `400 redirect_uri_mismatch` ile reddedilir. Desktop app istemcileri her
> loopback portunu kabul eder.

Nexus Mail'in ihtiyaç duyduğu `https://mail.google.com/` kapsamı *restricted
scope*'tur. Bu kapsamı isteyen bir uygulamayı yayınlamak yıllık CASA Tier 2
güvenlik denetimi gerektirir. Kendi istemcinizi testing modunda kullandığınız
için bunların hiçbiri sizi bağlamaz.

---

## Diğer sağlayıcılar

Hesap eklerken *Other IMAP server* seçin ve parolanızı ya da bir uygulama
parolası kullanın. Kayıt gerekmez.

Yaygın sağlayıcılar (Yandex, Zoho, Fastmail, iCloud, Yahoo ve diğerleri) için
sunucu adresleri gömülüdür; adresinizi yazmanız yeterli. Kurumsal bir alan adı
kullanıyorsanız IMAP sunucusunu ve portunu elle girmeniz gerekir — kurumsal
sunucular tahmin edilemez.

# Penche

Forum takibi yapıyorsun, tehdit içeren bir başlık görüyorsun, `Ctrl+Shift+X`'e basıyorsun — başlık, URL ve ekran görüntüsü Taiga Kanban'ına kart olarak düşüyor. Bu kadar.

Clear web forumlarında ve Tor Browser üzerinden `.onion` sitelerde çalışır. Bot koruması eklentiyi görmez — sen zaten tarayıcıdasın, eklenti sadece senin yaptığın şeyi kayıt altına alıyor.

---

## Nasıl Çalışır

```
Sen (forum sayfası) → Ctrl+Shift+X
  → Eklenti → başlık + URL + screenshot
    → Lokal Go servisi (127.0.0.1:8787)
      → Taiga API → kart + ekran görüntüsü
```

Taiga token'ın hiç tarayıcıya gelmiyor. Eklenti sadece kendi makinendeki servise bağlanıyor, o servis Taiga'ya gidiyor.

---

## Gereksinimler

İki program yüklü olması lazım:

**Node.js** — eklentiyi derlemek için  
→ https://nodejs.org adresine gir, "LTS" yazan yeşil butona tıkla, indir ve kur.

**Go** — lokal servisi çalıştırmak için  
→ https://go.dev/dl adresine gir, Windows için `.msi` dosyasını indir, kur.

Kurulumdan sonra yeni bir terminal aç ve şunları yaz:
```
node --version
go version
```
İkisi de çıktı veriyorsa hazırsın.

---

## Kurulum

### Adım 1 — Eklentiyi Derle

Terminalden `extension` klasörüne gir:

```
cd "C:\Users\Yasin\Downloads\Penche Scraper\penche-scraper\extension"
npm install
npm run build:chrome
```

Bu işlem `dist/chrome` klasörünü oluşturur. Yaklaşık 10-30 saniye sürer.

### Adım 2 — Eklentiyi Tarayıcıya Yükle

**Chrome:**
1. Adres çubuğuna `chrome://extensions` yaz, Enter'a bas
2. Sağ üstte **Geliştirici modu** toggle'ını aç (sağa kaydır)
3. Solda çıkan **Paketlenmemiş öğe yükle** butonuna tıkla
4. Şu klasörü seç: `penche-scraper\extension\dist\chrome`
5. "Penche Scraper" listede görünmeli

**Edge:**
1. Adres çubuğuna `edge://extensions` yaz
2. Sol altta **Geliştirici modu**nu aç
3. **Paketlenmemiş öğe yükle** → aynı `dist\chrome` klasörünü seç

---

### Adım 3 — Taiga Bilgilerini Al

İki şey lazım: API token ve proje slug.

**API Token:**

1. [tree.taiga.io](https://tree.taiga.io) adresinde oturum aç
2. Sağ üstte kullanıcı adına tıkla → **User settings**
3. Sol menüden **API** sekmesine tıkla
4. Token görünüyorsa kopyala. Görünmüyorsa sayfada "Generate new token" benzeri bir buton vardır, ona tıkla.

Token şuna benzer bir şey: `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

**Proje Slug:**

Taiga'da projenin URL'ine bak. Örneğin:
```
https://tree.taiga.io/project/enesm-monitoring-team/kanban
```
Buradaki `enesm-monitoring-team` senin proje slug'ın. Bunu bir yere not al.

---

### Adım 4 — Router'ı Ayarla

`penche-scraper\router\config.yaml` dosyasını bir metin editörüyle aç (Notepad da olur).

Şu üç satırı doldur:

```yaml
auth:
  shared_secret: "istedigin-bir-sifre-yaz"   # herhangi bir şey, boş bırakma

adapters:
  taiga:
    enabled: true
    base_url: "https://tree.taiga.io"
    project_slug: "enesm-monitoring-team"     # az önce URL'den aldığın kısım
    auth_token: ""                             # buraya token'ı yapıştır
```

`auth_token`'ı direkt dosyaya yazmak yerine env variable ile geçmek daha temiz — ama ikisi de çalışır.

---

### Adım 5 — Router'ı Başlat

Terminalde `router` klasörüne gir ve şu komutu çalıştır:

```
cd "C:\Users\Yasin\Downloads\Penche Scraper\penche-scraper\router"
go run ./cmd/server -config config.yaml
```

İlk çalıştırmada Go gerekli paketleri indirir, birkaç dakika sürebilir. Sonra şuna benzer bir çıktı görürsün:

```json
{"level":"INFO","msg":"adapter registered","name":"taiga"}
{"level":"INFO","msg":"server starting","addr":"127.0.0.1:8787"}
```

Servis çalıştığı sürece bu terminal açık kalmalı.

Çalışıp çalışmadığını test etmek için yeni bir terminalde:
```
curl http://127.0.0.1:8787/v1/health
```
`{"status":"ok"}` dönerse hazır.

---

### Adım 6 — Eklentiyi Bağla

1. Tarayıcıda Penche ikonuna tıkla → **Options**
2. **Router URL**: `http://127.0.0.1:8787`
3. **Shared Secret**: `config.yaml`'a ne yazdıysan aynısı
4. **Save Global Settings**
5. **Test Router Connection** → "Router is online" yazmalı

---

### Adım 7 — Forum Profili Tanımla

Her forum sitesi için bir kez yapılır, sonrası otomatik.

1. Takip edeceğin foruma git (örn. xss.is'te bir thread aç)
2. Options → **Domain Profiles** → **Add**
3. **Domain host**: `xss.is`
4. **🎯 Pick** butonuna bas
5. Sayfa üstünde kırmızı highlight açılır — thread başlığına gel, tıkla
6. Selector otomatik doluyor, preview gösteriyor
7. **Save Profile**

---

## Kullanım

Forum sayfasındayken **`Ctrl+Shift+X`** — bitti.

Sağ altta yeşil tik ve "Captured and sent!" yazısı çıkarsa Taiga'da kart açılmıştır.

---

## Webhook Alternatifi

Taiga yerine Discord, Slack veya başka bir servise göndermek istersen:

`config.yaml`'da:
```yaml
routes:
  default: "webhook"

adapters:
  taiga:
    enabled: false
  webhook:
    enabled: true
    url: "https://discord.com/api/webhooks/..."   # webhook URL'ini buraya yapıştır
```

Discord webhook URL'i almak için: Discord sunucunda → Ayarlar → Entegrasyonlar → Webhook oluştur → URL'yi kopyala. Başka bir şey gerekmez.

---

## Sonraki Forumu Eklemek

Her yeni forum için Adım 7'yi tekrar yap — sadece o domain için. Daha önce tanımladıkların aynen çalışmaya devam eder.

---

## Bir Şeyler Değiştirdikten Sonra

Eklenti kodunu değiştirdiysen yeniden derle:
```
cd extension
npm run build:chrome
```
Sonra `chrome://extensions`'da Penche kartındaki **⟳ yenile** ikonuna bas. Router'da değişiklik yaptıysan terminali kapatıp `go run` komutunu tekrar çalıştırman yeterli.

---

## Sık Karşılaşılan Durumlar

**"Router offline" yazıyor:**  
`go run` çalışmıyor demektir. Router terminaline bak, hata mesajı var mı kontrol et.

**Token hatası alıyorum:**  
`config.yaml`'daki `auth_token` ile Taiga'dan aldığın token aynı mı kontrol et. Token'da başta/sonda boşluk olmadığından emin ol.

**Proje bulunamadı hatası:**  
`project_slug` alanına Taiga URL'inden gördüğün kısmı aynen yazdığından emin ol. Büyük/küçük harf farklılığı olursa çalışmaz.

**Kart açıldı ama ekran görüntüsü yok:**  
Attachment ayrı bir API çağrısı — Taiga'nın rate limitine takılmış olabilir. Bir sonraki denemede gelecektir (router otomatik yeniden dener).

**Profil tanımlamadan önce kısayola bastım:**  
"No profile for this domain" uyarısı çıkar. Options'a gidip o domain için profil oluşturman yeterli.

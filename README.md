# Penche

Forum sayfasındasın, tehdit içeren bir başlık görüyorsun, `Ctrl+Shift+X`'e basıyorsun. Başlık, URL ve ekran görüntüsü kaydediliyor — ister yerel klasöre, ister Taiga'ya, ister Discord'a.

Clear web ve `.onion` sitelerde çalışır. Bot koruması devreye girmez.

---

## Genel Bakış

İki parçası var:

- **Eklenti** — tarayıcıya yüklenir, kısayolu dinler, veriyi toplar
- **Router** — senin makinende çalışan küçük bir servis, veriyi nereye göndereceğine karar verir

İkisi arasında haberleşme `localhost:8787` üzerinden olur. Hiçbir şey dışarıya çıkmaz.

**Varsayılan davranış:** Taiga veya webhook ayarlamak zorunda değilsin. Router başladığında capture'lar otomatik olarak `router/output/` klasörüne kaydedilir ve `http://127.0.0.1:8787/ui` adresinde bir galeri olarak görünür. Oradan tek tıkla kopyalayıp Taiga'ya elle yapıştırabilirsin.

---

## Gereksinimler

**Node.js** — eklentiyi derlemek için  
https://nodejs.org → "LTS" butonuna tıkla, indir, kur.

**Go** — router'ı çalıştırmak için  
https://go.dev/dl → Windows için `.msi` dosyasını indir, kur.

Kurulumdan sonra yeni bir terminal aç:
```
node --version
go version
```
İkisi de versiyon yazıyorsa hazırsın.

---

## Kurulum

### 1 — Eklentiyi Derle

```
cd "C:\Users\Yasin\Downloads\Penche Scraper\penche-scraper\extension"
npm install
npm run build:chrome
```

Bu `dist\chrome` klasörünü oluşturur. Bir kere yapman yeterli.

### 2 — Eklentiyi Tarayıcıya Yükle

**Chrome veya Edge:**

1. Adres çubuğuna yaz:
   - Chrome için: `chrome://extensions`
   - Edge için: `edge://extensions`
2. Sağ üstte **Geliştirici modu** toggle'ını aç
3. **Paketlenmemiş öğe yükle** butonuna tıkla
4. Şu klasörü seç:
   ```
   penche-scraper\extension\dist\chrome
   ```
5. "Penche Scraper" listede çıkmalı

### 3 — Router'ı Başlat

`router\config.yaml` dosyasını aç, `shared_secret` satırını değiştir:

```yaml
auth:
  shared_secret: "istedigin-bir-sifre"
```

Sonra terminalde:

```
cd "C:\Users\Yasin\Downloads\Penche Scraper\penche-scraper\router"
go run ./cmd/server -config config.yaml
```

İlk çalıştırmada Go gerekli paketleri indirir (bir kerelik, internet gerekir). Hazır olunca şunu görürsün:

```
{"level":"INFO","msg":"server starting","addr":"127.0.0.1:8787"}
```

Bu terminal açık kaldığı sürece router çalışır.

### 4 — Eklentiyi Bağla

1. Tarayıcıda Penche ikonuna tıkla → **Options**
2. **Router URL**: `http://127.0.0.1:8787`
3. **Shared Secret**: config.yaml'a yazdığın şifre
4. **Save Global Settings**
5. **Test Router Connection** → "Router is online" yazmalı

### 5 — Forum Profili Tanımla

Her forum sitesi için bir kez yapılır.

1. Takip edeceğin foruma git
2. Options → **Domain Profiles** → **Add**
3. **Domain host**: örn. `xss.is`
4. **🎯 Pick** butonuna bas
5. Sayfa üstünde kırmızı kutu açılır — thread başlığına gel, tıkla
6. Selector otomatik doluyor
7. **Save Profile**

---

## Kullanım

Profil tanımladığın bir forum sayfasındayken **Ctrl+Shift+X** bas.

Sağ altta yeşil bildirim çıkar. Capture `router/output/` klasörüne kaydedilir.

**Kaydedilenleri görmek için:** `http://127.0.0.1:8787/ui` adresini aç. Tüm capture'lar tarih ve domain bazında listelenir. Her kartın altında **Kopyala** butonu var — başlık, URL ve zaman damgasını panoya alır, Taiga'ya elle yapıştırabilirsin.

---

## Taiga'ya Otomatik Gönderim (İsteğe Bağlı)

Taiga entegrasyonu için bir API token lazım. Taiga'nın arayüzünde bu token görünmüyor — curl ile almanı gerekiyor.

**Token alma:**

Bir terminal aç ve şunu çalıştır (kullanıcı adı ve şifreni yaz):

```
curl -s -X POST https://tree.taiga.io/api/v1/auth ^
  -H "Content-Type: application/json" ^
  -d "{\"type\":\"normal\",\"username\":\"KULLANICI_ADIN\",\"password\":\"SIFREN\"}"
```

Dönen JSON'da `auth_token` alanı senin token'ın. Uzun bir string, kopyala.

> Git Bash veya PowerShell kullanıyorsan `^` yerine satır sonu olmadan tek satır yaz.

**Proje slug:**

Taiga URL'ine bak:
```
https://tree.taiga.io/project/enesm-monitoring-team/kanban
                              ^^^^^^^^^^^^^^^^^^^^
                              bu senin slug'ın
```

**config.yaml'ı güncelle:**

```yaml
routes:
  default: "taiga"

adapters:
  local:
    enabled: false
  taiga:
    enabled: true
    base_url: "https://tree.taiga.io"
    project_slug: "enesm-monitoring-team"
    auth_token: "buraya-tokenini-yapistir"
```

Router'ı yeniden başlat. Artık her capture Taiga'da kart olarak açılır, ekran görüntüsü de karta eklenir.

---

## Webhook Alternatifi (Discord, Slack vb.)

Discord veya başka bir webhook URL'in varsa:

```yaml
routes:
  default: "webhook"

adapters:
  local:
    enabled: false
  webhook:
    enabled: true
    url: "https://discord.com/api/webhooks/..."
```

Discord webhook URL'i almak: Sunucu Ayarları → Entegrasyonlar → Webhook → Yeni Webhook → URL Kopyala.

---

## Farklı Klasör veya Proje Bazlı Kayıt

Capture'lar zaten tarih ve domain bazında klasörleniyor:

```
router/output/
  2026-04-11/
    xss.is/
      143201_a1b2c3d4.jpg
      143201_a1b2c3d4.json
    exploit.in/
      ...
```

Farklı bir klasör istiyorsan `config.yaml`'da:

```yaml
adapters:
  local:
    enabled: true
    output_dir: "C:/CTI/Penche"
```

---

## Sık Karşılaşılan Durumlar

**"Router offline" yazıyor:**  
Router terminali kapalı veya hata var. Terminale bak.

**Kısayola basınca hiçbir şey olmuyor:**  
O domain için profil tanımlanmamış demektir. Options → Domain Profiles → Add ile profil ekle.

**Taiga token hatası alıyorum:**  
Token'da başta/sonda boşluk olmadığından emin ol. Curl çıktısında `auth_token` değerini `"` işaretleri olmadan kopyala.

**Proje bulunamadı hatası:**  
`project_slug` Taiga URL'indeki kısımla birebir aynı olmalı — büyük/küçük harf duyarlı.

**Kod değiştirdikten sonra:**  
Extension için: `npm run build:chrome` → `chrome://extensions`'da ⟳ yenile.  
Router için: terminali kapat, `go run ./cmd/server -config config.yaml` tekrar çalıştır.

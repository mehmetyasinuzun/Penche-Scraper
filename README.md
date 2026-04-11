# Penche

Forum sayfasındasın, tehdit içeren bir başlık görüyorsun. `Ctrl+Shift+X`. Başlık, URL ve ekran görüntüsü kaydedildi — ister yerel klasöre, ister Taiga'ya, ister Discord'a. Clear web ve `.onion` sitelerde çalışır. Bot koruması devreye girmez.

---

## Nasıl Çalışır

İki parçası var:

| Parça | Ne yapar |
|-------|----------|
| **Eklenti** | Tarayıcıya yüklenir, kısayolu dinler, veriyi toplar |
| **Router** | Kendi makinende çalışır, veriyi nereye göndereceğine karar verir |

İkisi `localhost:8787` üzerinden haberleşir. Dışarıya hiçbir şey çıkmaz.

**Varsayılan davranış:** Taiga veya webhook kurmak zorunda değilsin. Router başladığında capture'lar `router/output/` klasörüne kaydedilir, `http://127.0.0.1:8787/ui` adresinde galeri olarak görünür. Oradan tek tıkla Taiga'ya elle yapıştırabilirsin.

---

## Gereksinimler

**Node.js** — eklentiyi derlemek için  
→ https://nodejs.org — "LTS" butonu, indir, kur.

**Go** — router'ı çalıştırmak için  
→ https://go.dev/dl — Windows için `.msi` dosyasını indir, kur.

Kurulumdan sonra yeni bir terminal aç ve kontrol et:

```
node --version
go version
```

İkisi de versiyon yazıyorsa hazırsın.

---

## Kurulum

Repoyu bir yere klonla ya da ZIP olarak indir:

```
git clone https://github.com/mehmetyasinuzun/Penche-Scraper.git
cd Penche-Scraper
```

### 1 — Eklentiyi Derle

```
cd extension
npm install
npm run build:chrome
```

`dist/chrome` klasörü oluşur. Bir kere yapmak yeterli.

### 2 — Eklentiyi Tarayıcıya Yükle

**Chrome veya Edge:**

1. Adres çubuğuna gir:
   - Chrome: `chrome://extensions`
   - Edge: `edge://extensions`
2. Sağ üstte **Geliştirici modu** açık olsun
3. **Paketlenmemiş öğe yükle** → `extension/dist/chrome` klasörünü seç
4. Listede **Penche Scraper** görünmeli

### 3 — Router'ı Ayarla

```
cd router
```

`config.yaml` dosyasını aç, `shared_secret` kısmını değiştir:

```yaml
auth:
  shared_secret: "istedigin-bir-sifre"
```

Ne yazdığın önemli değil — eklenti ile router arasında eşleşmesi yeterli.

### 4 — Router'ı Başlat

```
go run ./cmd/server -config config.yaml
```

İlk çalıştırmada Go gerekli paketleri indirir (bir kerelik, internet gerekir). Hazır olunca:

```
{"level":"INFO","msg":"server starting","addr":"127.0.0.1:8787"}
```

Bu terminal açık kaldığı sürece router çalışır.

### 5 — Eklentiyi Bağla

1. Tarayıcıda Penche ikonuna tıkla → **Options**
2. **Router URL:** `http://127.0.0.1:8787`
3. **Shared Secret:** `config.yaml`'a yazdığın şifre
4. **Save Global Settings** → **Test Router Connection**
5. "Router online" yazmalı

### 6 — Forum Profili Tanımla

Her forum için bir kez yapılır.

1. Takip edeceğin foruma git (örn. xss.is, exploit.in)
2. Options → **Domain Profiles** → **Add**
3. **Domain host:** `xss.is`
4. **Pick** butonuna bas
5. Sayfada kırmızı bir kutu açılır — thread başlığına gel, tıkla
6. Selector otomatik dolar
7. **Save Profile**

---

## Kullanım

Profil tanımladığın bir forum sayfasındayken **`Ctrl+Shift+X`** bas.

Sağ altta yeşil bildirim çıkar. Capture `router/output/` klasörüne kaydedilir.

**Kaydedilenleri görmek için:** `http://127.0.0.1:8787/ui` — tarih ve domain bazında listelenir, her kartın altında **Kopyala** var.

---

## Taiga'ya Otomatik Gönderim

Taiga'nın arayüzünde API token bulunmuyor. Terminalle almanı gerekiyor:

```
curl -s -X POST https://tree.taiga.io/api/v1/auth ^
  -H "Content-Type: application/json" ^
  -d "{\"type\":\"normal\",\"username\":\"KULLANICI_ADIN\",\"password\":\"SIFREN\"}"
```

Dönen JSON'daki `auth_token` değeri senin token'ın.

**Proje slug** Taiga URL'inden alınır:
```
https://tree.taiga.io/project/enesm-monitoring-team/kanban
                              ^^^^^^^^^^^^^^^^^^^^
                              bu kısım
```

`router/config.yaml`'ı güncelle:

```yaml
routes:
  default: "taiga"

adapters:
  local:
    enabled: false
  taiga:
    enabled: true
    base_url: "https://tree.taiga.io"
    project_slug: "proje-slug-buraya"
    auth_token: "token-buraya"
```

Router'ı yeniden başlat. Artık her capture Taiga'da kart olarak açılır, ekran görüntüsü karta eklenir.

---

## Webhook (Discord, Slack, vb.)

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

---

## Capture Klasör Yapısı

```
router/output/
  2026-04-11/
    xss.is/
      143201_a1b2c3d4.jpg
      143201_a1b2c3d4.json
    exploit.in/
      ...
```

Farklı bir dizin için `config.yaml`'da `output_dir` değerini değiştir.

---

## Sık Karşılaşılan Durumlar

**"Router offline" yazıyor**  
Router terminali kapalı veya hata var. Terminale bak.

**Kısayola basınca hiçbir şey olmuyor**  
O domain için profil tanımlanmamış. Options → Domain Profiles → Add.

**Taiga token hatası**  
Token'da başta/sonda boşluk olmamasına dikkat et. Tırnak işaretleri olmadan kopyala.

**Proje bulunamadı hatası**  
`project_slug` URL'deki kısımla birebir aynı olmalı.

**Değişiklik sonrası:**

```
# Eklenti değişikliği
cd extension && npm run build:chrome
# Sonra chrome://extensions → ⟳ yenile

# Router değişikliği
# Terminali kapat, tekrar başlat:
go run ./cmd/server -config config.yaml
```

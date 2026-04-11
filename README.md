# Penche

Forum sayfasındasın, tehdit içeren bir başlık görüyorsun, `Ctrl+Shift+X`'e basıyorsun. Başlık, URL ve ekran görüntüsü kaydediliyor — ister yerel klasöre, ister Taiga'ya, ister Discord'a.

Clear web ve `.onion` sitelerde çalışır. Bot koruması devreye girmez.

---

## Nasıl Çalışır

İki parçası var:

- **Eklenti** — tarayıcıya yüklenir, kısayolu dinler, veriyi toplar
- **Router** — senin makinende çalışan küçük bir servis, veriyi nereye göndereceğine karar verir

İkisi arasında haberleşme `localhost:8787` üzerinden olur. Hiçbir şey dışarıya çıkmaz.

**Varsayılan davranış:** Taiga veya webhook ayarlamak zorunda değilsin. Router başladığında capture'lar `router/output/` klasörüne kaydedilir, `http://127.0.0.1:8787/ui` adresinde galeri olarak görünür. Oradan tek tıkla kopyalayıp Taiga'ya elle yapıştırabilirsin.

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

Bu `dist/chrome` klasörünü oluşturur. Bir kere yapman yeterli.

### 2 — Eklentiyi Tarayıcıya Yükle

**Chrome veya Edge:**

1. Adres çubuğuna yaz:
   - Chrome: `chrome://extensions`
   - Edge: `edge://extensions`
2. Sağ üstte **Geliştirici modu** toggle'ını aç
3. **Paketlenmemiş öğe yükle** butonuna tıkla
4. `extension/dist/chrome` klasörünü seç
5. "Penche Scraper" listede çıkmalı

### 3 — Router'ı Ayarla

`router/config.yaml` dosyasını bir metin editörüyle aç, `shared_secret` kısmını değiştir:

```yaml
auth:
  shared_secret: "istedigin-bir-sifre"
```

Boş bırakma, ama ne yazdığın önemli değil — eklenti ile router arasında eşleşmesi yeterli.

### 4 — Router'ı Başlat

```
cd router
go run ./cmd/server -config config.yaml
```

İlk çalıştırmada Go gerekli paketleri indirir (bir kerelik, internet gerekir). Hazır olunca:

```
{"level":"INFO","msg":"server starting","addr":"127.0.0.1:8787"}
```

Bu terminal açık kaldığı sürece router çalışır.

### 5 — Eklentiyi Bağla

1. Tarayıcıda Penche ikonuna tıkla → **Options**
2. **Router URL**: `http://127.0.0.1:8787`
3. **Shared Secret**: config.yaml'a yazdığın şifre
4. **Save Global Settings**
5. **Test Router Connection** → "Router is online" yazmalı

### 6 — Forum Profili Tanımla

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

**Kaydedilenleri görmek için:** Tarayıcıda `http://127.0.0.1:8787/ui` adresini aç. Tüm capture'lar tarih ve domain bazında listelenir. Her kartın altında **Kopyala** butonu var — başlık, URL ve zaman damgasını panoya alır.

---

## Taiga'ya Otomatik Gönderim (İsteğe Bağlı)

Taiga'nın arayüzünde API token bulunmuyor — curl ile almanı gerekiyor:

```
curl -s -X POST https://tree.taiga.io/api/v1/auth ^
  -H "Content-Type: application/json" ^
  -d "{\"type\":\"normal\",\"username\":\"KULLANICI_ADIN\",\"password\":\"SIFREN\"}"
```

Dönen JSON'daki `auth_token` değeri senin token'ın.

**Proje slug:** Taiga URL'inden alınır:
```
https://tree.taiga.io/project/enesm-monitoring-team/kanban
                              ^^^^^^^^^^^^^^^^^^^^
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

## Webhook Alternatifi

Discord, Slack veya herhangi bir HTTP endpoint:

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

Farklı bir yol için `config.yaml`'da `output_dir` değerini değiştir.

---

## Sık Karşılaşılan Durumlar

**"Router offline" yazıyor** — Router terminali kapalı veya hata var. Terminale bak.

**Kısayola basınca hiçbir şey olmuyor** — O domain için profil tanımlanmamış. Options → Domain Profiles → Add.

**Taiga token hatası** — Token'da başta/sonda boşluk olmamasına dikkat et. `auth_token` değerini tırnak işaretleri olmadan kopyala.

**Proje bulunamadı hatası** — `project_slug` URL'deki kısımla birebir aynı olmalı.

**Değişiklik yaptıktan sonra:**
- Extension: `npm run build:chrome` → `chrome://extensions`'da ⟳ yenile
- Router: terminali kapatıp `go run ./cmd/server -config config.yaml` tekrar çalıştır

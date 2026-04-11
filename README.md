# Penche

Pençe. Bir forum sayfasındayken tek tuşa basıyorsun, başlık + link + ekran görüntüsü doğrudan Taiga kartına düşüyor. O kadar.

CTI süreçlerinde en çok zamanı yiyen şeylerden biri forum takibi — giriyorsun, görüyorsun, ekran görüntüsü alıyorsun, kopyala yapıştır yapıyorsun, ticket açıyorsun. Penche bu adımların hepsini bir klavye kısayoluna indirgiyor.

---

## Nasıl Çalışır

Tarayıcıya bir eklenti yüklenliyor. Sen forumda geziniyorsun, ilgili bir başlık gördüğünde `Ctrl+Shift+X`'e basıyorsun. Eklenti sayfanın başlığını, URL'ini ve ekran görüntüsünü alıp yerelde çalışan bir Go servisine gönderiyor. O servis de Taiga'da ilgili projeye otomatik kart açıyor, ekran görüntüsünü de karta attach ediyor.

```
Sen (forum sayfası) → Ctrl+Shift+X
  → Extension (başlık + URL + screenshot)
    → Lokal Go Router (127.0.0.1:8787)
      → Taiga API (issue + attachment)
```

Taiga tokenın hiç tarayıcıya gelmiyor. Eklenti sadece localhost'taki servise konuşuyor. Servis de senin makinende, sadece 127.0.0.1'i dinliyor.

Clear web forumlarında çalışıyor, Tor Browser üzerinden .onion sitelerde de — eklenti DOM üzerinde çalıştığı için Cloudflare veya benzeri bir bot koruması eklentiyi görmüyor.

---

## Kurulum

### Router (Go servisi)

Go 1.23 gerekiyor.

```bash
cd router
cp config.yaml config.local.yaml
```

`config.local.yaml` dosyasını aç, `shared_secret` kısmına güçlü bir şey yaz. Taiga ayarlarını ya bu dosyaya yaz ya da env var olarak geç:

```bash
export PENCHE_AUTH_SECRET="buraya-guclu-bir-sifre"
export PENCHE_TAIGA_TOKEN="taiga-api-tokenin"
export PENCHE_TAIGA_PROJECT_ID="proje-id-numarasi"

go run ./cmd/server -config config.local.yaml
```

Çalışıyor mu kontrol et:
```bash
curl http://127.0.0.1:8787/v1/health
```

### Eklenti

Node.js 20+ gerekiyor.

```bash
cd extension
npm install
npm run build
```

Bu komut `dist/chrome/` ve `dist/firefox/` klasörlerini oluşturuyor.

**Chrome / Brave / Chromium:**
- `chrome://extensions` aç
- "Geliştirici modu"nu aç
- "Paketlenmemiş öğe yükle" → `extension/dist/chrome/` klasörünü seç

**Firefox / Tor Browser:**
- `about:debugging#/runtime/this-firefox` aç
- "Geçici Eklenti Yükle" → `extension/dist/firefox/manifest.json` dosyasını seç

### Eklenti Ayarları

Eklenti ikonuna tıkla → Options:
- Router URL: `http://127.0.0.1:8787`
- Shared Secret: router'da ne yazdıysan aynısı
- "Test Router Connection" ile bağlantıyı doğrula

---

## Alan Adı Profili Tanımlama

Her forum sitesi için bir kez yapılıyor, sonra o sitede her kullanımda otomatik çalışıyor.

1. Takip etmek istediğin foruma git (örn. `xss.is/threads/12345`)
2. Options → Domain Profiles → Add
3. Domain host yaz: `xss.is`
4. "🎯 Pick" butonuna bas — sayfa üstünde bir seçici açılıyor
5. Thread başlığının üstüne gel, kırmızı highlight'ı gördüğünde tıkla
6. Selector otomatik doluyor, preview gösteriyor
7. Screenshot modunu ayarla (genelde `top_px` ile 2200-3000px yeterli)
8. Kaydet

Artık o sitede `Ctrl+Shift+X`'e basınca her şey otomatik.

### Screenshot Modları

| Mod | Ne Yapar |
|---|---|
| `viewport` | Sadece ekranda görünen alan. En hızlı. |
| `top_px` | Sayfanın en üstünden N piksel. Varsayılan 2200px, ayarlanabilir. |
| `full_page` | Tüm sayfayı scroll ederek parçalara böler, birleştirir. Max 16000px. |
| `element` | Belirli bir CSS selector'ün bounding box'ını crop eder. |

Her domain için ayrı mod tanımlayabilirsin, yoksa global default kullanılır.

---

## Router Kapalıyken

Eklenti tarafa da bir outbox var. Router'a ulaşamazsa payload `browser.storage.local`'a yazılıyor, 30 saniyede bir tekrar deneniyor. Router ayağa kalkınca kuyruk boşalıyor.

---

## Taiga Token Almak

1. [tree.taiga.io](https://tree.taiga.io) → User Settings → API
2. Application token'ı kopyala

Proje ID için:
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://tree.taiga.io/api/v1/projects/by_slug?slug=KULLANICI-PROJE_SLUG"
```
`id` alanındaki sayı proje ID'si.

---

## Yeni Hedef Eklemek (Taiga Dışı)

Bir `DestinationAdapter` interface'i var:

```go
type DestinationAdapter interface {
    Name() string
    Send(ctx context.Context, evt *domain.StoredEvent) (DeliveryResult, error)
    ValidateConfig() error
}
```

`router/internal/adapters/` altına yeni bir klasör açıp bu interface'i implement edersen, `cmd/server/main.go`'ya kaydetmek yeterli. Eklentiye dokunman gerekmiyor.

Halihazırda iki adapter var: `taiga` ve `webhook` (generic HTTP POST). Domain bazında farklı hedeflere yönlendirme de config üzerinden yapılabiliyor.

---

## Güvenlik

- Taiga token eklentide tutulmuyor, sadece router config'inde
- Router sadece `127.0.0.1` dinliyor, dışarıya açık değil
- Her istek HMAC-SHA256 ile imzalanıyor, 5 dakika replay koruması var
- Log'larda screenshot base64 verisi hiç yazılmıyor

---

## Geliştirme

```bash
make test          # Go testleri çalıştır
make ext-dev       # Extension'ı Chrome için watch modunda build et
make build         # Her şeyi derle (router binary + her iki ext hedefi)
make help          # Tüm komutlar
```

---

## Klasör Yapısı

```
penche-scraper/
  extension/
    src/
      background/    servis worker, capture, screenshot, outbox
      content/       toast bildirimleri, element picker
      popup/         hızlı durum ekranı
      options/       ayarlar + profil editörü
      shared/        ortak tipler, config, hmac, logger
    manifest.chrome.json
    manifest.firefox.json
  router/
    cmd/server/      uygulama entry point
    internal/
      api/           HTTP handler'lar
      auth/          HMAC doğrulama
      config/        YAML config + env override
      domain/        core tipler
      storage/       SQLite + migration'lar
      adapters/      taiga, webhook
      worker/        retry + dead-letter queue
    config.yaml      örnek config
  docs/
    api.md           Router REST API referansı
    architecture.md  detaylı mimari açıklama
```

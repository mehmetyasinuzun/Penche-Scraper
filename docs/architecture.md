# Mimari

## Genel Bakış

İki bileşen var: tarayıcı eklentisi ve yerel bir Go servisi. Bunlar birbirinden bağımsız — eklenti sadece `localhost:8787`'ye istek atıyor, hedef sistemin ne olduğunu bilmiyor.

```
Tarayıcı
  └── Extension (TypeScript)
        │  Ctrl+Shift+X tetiklenince
        ▼
     Capture Orchestrator
        │  POST /v1/events  (HMAC-SHA256 imzalı)
        ▼
  Go Router  →  127.0.0.1:8787
        │
        ├── SQLite  (events + delivery_jobs + job_attempts)
        │
        └── Worker
              ├── Taiga Adapter  →  Taiga REST API
              └── Webhook Adapter → herhangi bir HTTP endpoint
```

Bu ayrımın pratik faydası şu: yarın Taiga yerine başka bir yere göndermek istersen eklentiyi güncelleme dağıtmana gerek yok. Router'daki config'i değiştirmek yeterli.

---

## Extension Tarafı

### background/index.ts
Servis worker. `browser.commands.onCommand` ile kısayolu dinliyor. Capture tamamlanınca content script'e toast mesajı fırlatıyor, popup açıksa sonucu aktarıyor. 30 saniyede bir outbox'ı boşaltmaya çalışıyor.

### background/capture.ts
Ana orchestrator. Sırasıyla şunları yapıyor:
1. Aktif tab'ın URL'ini alır
2. Host + path'i config'deki domain profilleriyle eşleştirir
3. Profil bulunursa CSS selector chain'i ile başlığı çeker
4. Screenshot engine'i doğru modda çağırır
5. Payload'ı imzalayıp router'a POST eder
6. Başarısız olursa outbox'a yazar

### background/screenshot.ts
Dört mod var:

**viewport** — `captureVisibleTab` ile tek çekim. Hızlı, tek call.

**full_page** — Content script üzerinden sayfa yüksekliği alınır. Viewport yüksekliği kadar parçalara bölünür, her parça için scroll + capture yapılır, sonra OffscreenCanvas üzerinde birleştirilir. Max yükseklik limiti aşılırsa o noktada durur.

**top_px** — full_page ile aynı mekanizma ama belirtilen piksel kadarında durur. Scroll'un tamamını almaya gerek yoksa bu daha verimli.

**element** — Content script üzerinden elemanın bounding rect'i alınır, viewport'a getirilir, capture alınır, canvas üzerinde crop edilir. Eleman bulunamazsa viewport moduna düşer.

Tüm birleştirme işlemleri OffscreenCanvas'ta yapılıyor — servis worker'ı bloklamıyor.

### background/outbox.ts
`browser.storage.local`'da JSON array olarak tutulan basit bir kuyruk. Router erişilemezse payload buraya yazılıyor. Her item'ın `nextRetryAt` ve `attemptCount` alanı var. Max deneme sayısına ulaşınca item kuyruktan siliniyor (dead-letter log'a düşüyor).

### background/router-client.ts
`fetch` wrapper'ı. HMAC headerlarını ekliyor, timeout yönetiyor, hata türünü sınıflandırıyor (retryable / non-retryable). Network hatası retryable, 4xx hataları değil.

### content/picker.ts
Sayfa üstüne bir overlay mount ediyor. Hover olaylarını dinleyip kırmızı bir highlight kutusu çiziyor. Click gelince:
1. Tıklanan elemanı alır
2. Önce ID'ye bakar (stabil görünüyorsa)
3. Tag + class kombinasyonunu dener, tek eşleşiyorsa kullanır
4. Yoksa 4 seviye derinliğe kadar yapısal path üretir
5. Sonucu background'a mesaj olarak gönderir

ESC ile iptal.

### shared/hmac.ts
Web Crypto API kullanarak HMAC-SHA256 imzası üretiyor. `HMAC(secret, timestamp + "." + body)` formatında. Hem servis worker'da hem content script'te çalışıyor (Web Crypto her ikisinde de var).

### shared/config.ts
Domain profil resolution mantığı burada. Önce tam host eşleşmesi, sonra opsiyonel path regex kontrolü, en son wildcard subdomain (`*.domain.com`) deniyor.

---

## Router Tarafı

### cmd/server/main.go
Bağımlılıkları wire ediyor: config → store → verifier → handler → worker. Graceful shutdown'da önce HTTP server'ı kapatıyor, worker'ın in-flight job'ları bitirmesini bekliyor.

### internal/api/handler.go
Üç endpoint:
- `GET /v1/health` — her zaman 200, monitoring için
- `GET /v1/metrics` — status bazında event sayıları
- `POST /v1/events` — body okur, HMAC doğrular, event persist eder, delivery job açar

Duplicate event_id gelirse 200 döner, yeni job açmaz. Extension retry yaptığında çift kart açılmıyor.

### internal/auth/hmac.go
Header'lardan timestamp ve signature alıp doğruluyor. Clock skew kontrolü var (±5 dakika). Body üzerinden signature yeniden hesaplanıp constant-time compare ile karşılaştırılıyor.

### internal/storage/sqlite.go
`modernc.org/sqlite` kullanıyor — CGO gerektirmiyor, cross-compile basit. Migration SQL dosyaları embed ediliyor, uygulama açılışında otomatik çalışıyor. Single writer connection, WAL modu.

Üç tablo:
- `events` — capture edilen her sayfa
- `delivery_jobs` — her (event × destination) çifti için bir job
- `job_attempts` — her deneme audit kaydı

### internal/worker/worker.go
Basit polling loop. Her N saniyede `delivery_jobs`'ı sorgular, due olan job'ları alır, semaphore ile concurrency sınırlar, goroutine'de işler. Başarısızlıkta exponential backoff ile `next_run_at`'ı ilerletir. Max deneme sayısı aşılınca `dead_letter` state'e geçer, event status'ü de güncellenir.

### internal/adapters/

`DestinationAdapter` interface'ini implement eden her paket bir "destination" oluyor:

```go
type DestinationAdapter interface {
    Name() string
    Send(ctx context.Context, evt *domain.StoredEvent) (DeliveryResult, error)
    ValidateConfig() error
}
```

**taiga adapter:** Issue oluşturuyor (title, description, tags), sonra ayrı bir call ile screenshot'ı attachment olarak yüklüyor. Attachment için önce issue ref → ID lookup yapıyor çünkü Taiga attachment API'si ref değil ID istiyor.

**webhook adapter:** Sadece metadata gönderiyor (screenshot binary'sini dahil etmiyor, payload büyür). Configurable method, headers, timeout.

---

## Veritabanı Şeması

```sql
events (
  event_id TEXT UNIQUE,   -- idempotency key
  status TEXT,            -- pending | delivered | dead_letter
  screenshot_data BLOB,   -- binary olarak saklanıyor, log'a yazılmıyor
  ...
)

delivery_jobs (
  event_id → events.event_id,
  destination TEXT,       -- "taiga" | "webhook" | ...
  status TEXT,            -- queued | processing | done | failed | dead_letter
  attempt_count INT,
  next_run_at TEXT,       -- backoff ile ilerleyen timestamp
  ...
)

job_attempts (
  job_id → delivery_jobs.id,
  attempt_no INT,
  status TEXT,
  error TEXT
)
```

Trigger'lar `updated_at`'ı otomatik güncelliyor.

---

## Güvenlik Tasarımı

**Taiga token extension'da yok.** Eklenti sadece `sharedSecret` biliyor, o da sadece localhost router ile konuşmak için kullanılıyor.

**HMAC replay koruması.** Timestamp header'ı var, ±5 dakika dışındaki istekler reddediliyor. Biri network'ü dinleyip aynı isteği tekrarlasa bile pencere çok dar.

**Screenshot log'a gitmiyor.** `logger.ts`'de base64 içeren field'lar `[REDACTED]` olarak yazılıyor. Go tarafında da screenshot binary'si hiç log'lanmıyor.

**Localhost binding.** Router `127.0.0.1` dinliyor, `0.0.0.0` değil. Dışarıdan erişilemiyor.

---

## Yeni Adapter Ekleme

1. `router/internal/adapters/yeni/yeni.go` — interface implement et
2. `router/internal/config/config.go` — config struct ekle
3. `router/cmd/server/main.go` — `buildAdapters`'a kaydet
4. `router/config.yaml` — örnek config ekle

Extension'a dokunman gerekmiyor.

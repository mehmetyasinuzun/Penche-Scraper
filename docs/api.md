# Router API

Base URL: `http://127.0.0.1:8787`

---

## Kimlik Doğrulama

Mutating endpoint'lerde (`POST /v1/events`) HMAC-SHA256 imzası zorunlu. İki header gönderilmesi lazım:

| Header | İçerik |
|---|---|
| `X-Penche-Timestamp` | Unix timestamp (saniye) |
| `X-Penche-Signature` | `HMAC-SHA256(secret, timestamp + "." + body)` — hex string |

İmza penceresi ±5 dakika. Bu dışındaki timestamp'ler reddediliyor.

Extension bu imzayı otomatik oluşturuyor. Manuel test için Go test dosyalarındaki `auth.Sign()` helper'ını kullanabilirsin.

---

## GET /v1/health

Servis ayakta mı kontrolü. Auth gerektirmiyor.

**Response 200:**
```json
{
  "status": "ok",
  "time": "2026-04-11T12:00:00Z"
}
```

---

## GET /v1/metrics

Event'lerin durumuna göre sayım. Auth gerektirmiyor.

**Response 200:**
```json
{
  "events": {
    "pending": 3,
    "delivered": 87,
    "dead_letter": 1
  }
}
```

---

## POST /v1/events

Extension'dan capture edilen sayfayı alır, kuyruğa ekler.

**Request body:**
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "captured_at": "2026-04-11T14:32:00Z",
  "domain": "xss.is",
  "page_title": "Thread başlığı buraya geliyor",
  "page_url": "https://xss.is/threads/12345/",
  "screenshot": {
    "mime": "image/jpeg",
    "base64": "<base64 encoded image>"
  },
  "meta": {
    "browser": "firefox",
    "profile_id": "xss-default",
    "tags": ["cti", "forum"]
  }
}
```

| Alan | Zorunlu | Açıklama |
|---|---|---|
| `event_id` | Evet | UUID v4. Idempotency için. Aynı ID tekrar gelirse yeni kart açılmaz. |
| `captured_at` | Evet | ISO8601 timestamp |
| `domain` | Evet | Hostname. Router bu alanı hedef adapter seçmek için de kullanıyor. |
| `page_title` | Evet | Sayfadan çekilen başlık |
| `page_url` | Evet | Tam URL |
| `screenshot.mime` | Evet | `image/jpeg`, `image/png` veya `image/webp` |
| `screenshot.base64` | Evet | Base64 encode edilmiş görüntü |
| `meta.browser` | Hayır | Hangi tarayıcıdan geldiği |
| `meta.profile_id` | Hayır | Hangi domain profilinin kullanıldığı |
| `meta.tags` | Hayır | Domain profilindeki tag'ler |

**Response 202 — Kabul edildi:**
```json
{
  "status": "accepted",
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "destination": "taiga"
}
```

Kart hemen açılmıyor. Worker kuyruktan alıp işliyor.

**Response 200 — Zaten var:**
```json
{
  "status": "duplicate",
  "event_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

Extension retry yaptığında çift kart açılmasın diye.

**Response 400 — Eksik alan:**
```json
{ "error": "page_url is required" }
```

**Response 401 — İmza hatası:**
```json
{ "error": "unauthorized" }
```

---

## Retry Davranışı

Delivery başarısız olursa exponential backoff ile tekrar deneniyor:

| Deneme | Bekleme |
|---|---|
| 1 | 1 saniye |
| 2 | 2 saniye |
| 3 | 4 saniye |
| 4 | 8 saniye |
| 5 | 16 saniye |
| sonrası | max 60 saniye |

`max_retries` (default: 5) aşılınca job `dead_letter` state'e geçiyor, event status'ü de güncelleniyor. `GET /v1/metrics`'ten takip edebilirsin.

---

## Domain Bazlı Yönlendirme

Hangi domain'in hangi adapter'a gideceğini `config.yaml`'daki `routes.domain_map`'ten ayarlayabilirsin:

```yaml
routes:
  default: "taiga"
  domain_map:
    "exploit.in": "taiga"
    "custom.onion": "webhook"
```

Eşleşme yoksa `default` kullanılır.

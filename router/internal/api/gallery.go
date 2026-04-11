package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"github.com/penche/router/internal/domain"
)

// GalleryStore is the subset of storage needed for the gallery view.
type GalleryStore interface {
	ListEvents(ctx context.Context, limit int) ([]*domain.StoredEvent, error)
}

type galleryHandler struct {
	store GalleryStore
	tmpl  *template.Template
}

func newGalleryHandler(store GalleryStore) *galleryHandler {
	tmpl := template.Must(template.New("gallery").Funcs(template.FuncMap{
		"b64img": func(mime string, data []byte) string {
			return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		},
		"fmtTime": func(t time.Time) string {
			return t.Local().Format("02 Jan 2006 · 15:04:05")
		},
		"jsonTags": func(s string) []string {
			var tags []string
			_ = json.Unmarshal([]byte(s), &tags)
			return tags
		},
	}).Parse(galleryTmpl))
	return &galleryHandler{store: store, tmpl: tmpl}
}

type galleryCard struct {
	EventID    string
	CapturedAt time.Time
	Domain     string
	PageTitle  string
	PageURL    string
	Tags       []string
	ImgSrc     string // data URI
	Status     string
}

func (h *galleryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListEvents(r.Context(), 200)
	if err != nil {
		http.Error(w, "could not load events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var cards []galleryCard
	for _, e := range events {
		var tags []string
		_ = json.Unmarshal([]byte(e.MetaTags), &tags)

		imgSrc := ""
		if len(e.ScreenshotData) > 0 {
			imgSrc = "data:" + e.ScreenshotMIME + ";base64," +
				base64.StdEncoding.EncodeToString(e.ScreenshotData)
		}

		cards = append(cards, galleryCard{
			EventID:    e.EventID,
			CapturedAt: e.CapturedAt,
			Domain:     e.Domain,
			PageTitle:  e.PageTitle,
			PageURL:    e.PageURL,
			Tags:       tags,
			ImgSrc:     imgSrc,
			Status:     string(e.Status),
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.Execute(w, map[string]any{
		"Cards": cards,
		"Count": len(cards),
	})
}

// galleryTmpl is the self-contained HTML gallery page.
const galleryTmpl = `<!DOCTYPE html>
<html lang="tr">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Penche — Capture Gallery</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#0d0d1a;color:#e2e8f0;min-height:100vh}
.topbar{background:#1a1a2e;border-bottom:1px solid #2a2a4a;padding:14px 24px;display:flex;align-items:center;gap:16px;position:sticky;top:0;z-index:100}
.topbar h1{font-size:18px;font-weight:700;color:#a78bfa}
.topbar .count{font-size:13px;color:#4b5563;margin-left:auto}
.search{background:#111827;border:1px solid #374151;color:#e2e8f0;border-radius:6px;padding:7px 12px;font-size:13px;width:260px;outline:none}
.search:focus{border-color:#7c3aed}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(360px,1fr));gap:20px;padding:24px}
.card{background:#1a1a2e;border:1px solid #2a2a4a;border-radius:10px;overflow:hidden;display:flex;flex-direction:column;transition:border-color 0.15s}
.card:hover{border-color:#7c3aed}
.card-img{width:100%;max-height:240px;object-fit:cover;object-position:top;display:block;background:#111}
.card-img.placeholder{height:160px;display:flex;align-items:center;justify-content:center;color:#374151;font-size:13px}
.card-body{padding:14px;flex:1;display:flex;flex-direction:column;gap:8px}
.card-title{font-size:14px;font-weight:600;color:#f1f5f9;line-height:1.4;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.card-url{font-size:12px;color:#6366f1;text-decoration:none;word-break:break-all;display:-webkit-box;-webkit-line-clamp:1;-webkit-box-orient:vertical;overflow:hidden}
.card-url:hover{text-decoration:underline}
.card-meta{font-size:11px;color:#4b5563;display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.tag{background:#1e293b;color:#94a3b8;padding:2px 8px;border-radius:12px;font-size:11px}
.status{padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600}
.status-delivered{background:#14532d;color:#4ade80}
.status-pending{background:#1e3a5f;color:#60a5fa}
.status-dead_letter{background:#450a0a;color:#f87171}
.card-actions{padding:10px 14px;border-top:1px solid #1e1e38;display:flex;gap:8px}
.btn{padding:6px 12px;border:none;border-radius:6px;font-size:12px;font-weight:600;cursor:pointer;transition:background 0.15s}
.btn-copy{background:#312e81;color:#c7d2fe}
.btn-copy:hover{background:#3730a3}
.btn-open{background:#1e293b;color:#94a3b8}
.btn-open:hover{background:#334155}
.btn-copy.copied{background:#14532d;color:#4ade80}
.empty{text-align:center;padding:80px 20px;color:#374151}
.empty h2{font-size:20px;margin-bottom:8px;color:#4b5563}
.filter-bar{padding:0 24px 16px;display:flex;gap:8px;flex-wrap:wrap}
.filter-btn{background:#1e293b;border:1px solid #374151;color:#94a3b8;border-radius:16px;padding:4px 14px;font-size:12px;cursor:pointer;transition:all 0.15s}
.filter-btn:hover,.filter-btn.active{background:#312e81;border-color:#7c3aed;color:#c7d2fe}
</style>
</head>
<body>

<div class="topbar">
  <h1>⚡ Penche</h1>
  <input class="search" type="search" id="search" placeholder="Başlık veya domain ara…" oninput="filterCards()">
  <span class="count" id="count-label">{{.Count}} capture</span>
</div>

{{if .Cards}}
<div class="filter-bar" id="domain-filters">
  <button class="filter-btn active" onclick="setDomain('', this)">Tümü</button>
</div>

<div class="grid" id="grid">
{{range .Cards}}
<div class="card"
     data-domain="{{.Domain}}"
     data-title="{{.PageTitle}}"
     data-url="{{.PageURL}}">

  {{if .ImgSrc}}
  <img class="card-img" src="{{.ImgSrc}}" alt="screenshot" loading="lazy">
  {{else}}
  <div class="card-img placeholder">Ekran görüntüsü yok</div>
  {{end}}

  <div class="card-body">
    <div class="card-title" title="{{.PageTitle}}">{{.PageTitle}}</div>
    <a class="card-url" href="{{.PageURL}}" target="_blank" rel="noopener">{{.PageURL}}</a>
    <div class="card-meta">
      <span>{{fmtTime .CapturedAt}}</span>
      <span>·</span>
      <span>{{.Domain}}</span>
      {{if .Tags}}
      {{range .Tags}}<span class="tag">{{.}}</span>{{end}}
      {{end}}
      <span class="status status-{{.Status}}">{{.Status}}</span>
    </div>
  </div>

  <div class="card-actions">
    <button class="btn btn-copy" onclick="copyCard(this, {{.PageTitle | js}}, {{.PageURL | js}}, {{fmtTime .CapturedAt | js}})">
      Kopyala
    </button>
    <a class="btn btn-open" href="{{.PageURL}}" target="_blank" rel="noopener">Siteye Git</a>
    {{if .ImgSrc}}
    <button class="btn btn-open" onclick="downloadImg(this, {{.ImgSrc | js}}, {{.Domain | js}})">
      İndir
    </button>
    {{end}}
  </div>
</div>
{{end}}
</div>

{{else}}
<div class="empty">
  <h2>Henüz capture yok</h2>
  <p>Bir forum sayfasında <strong>Ctrl+Shift+X</strong> kısayolunu kullan.</p>
</div>
{{end}}

<script>
// Build domain filter buttons from existing cards
const allCards = Array.from(document.querySelectorAll('.card'));
const domains = [...new Set(allCards.map(c => c.dataset.domain))].sort();
const filterBar = document.getElementById('domain-filters');
let activeDomain = '';

domains.forEach(d => {
  const btn = document.createElement('button');
  btn.className = 'filter-btn';
  btn.textContent = d;
  btn.onclick = () => setDomain(d, btn);
  filterBar.appendChild(btn);
});

function setDomain(domain, btn) {
  activeDomain = domain;
  document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  filterCards();
}

function filterCards() {
  const q = document.getElementById('search').value.toLowerCase();
  let visible = 0;
  allCards.forEach(card => {
    const matchDomain = !activeDomain || card.dataset.domain === activeDomain;
    const matchSearch = !q ||
      card.dataset.title.toLowerCase().includes(q) ||
      card.dataset.url.toLowerCase().includes(q) ||
      card.dataset.domain.toLowerCase().includes(q);
    const show = matchDomain && matchSearch;
    card.style.display = show ? '' : 'none';
    if (show) visible++;
  });
  document.getElementById('count-label').textContent = visible + ' capture';
}

function copyCard(btn, title, url, capturedAt) {
  const text = title + '\n' + url + '\n' + capturedAt;
  navigator.clipboard.writeText(text).then(() => {
    const orig = btn.textContent;
    btn.textContent = '✓ Kopyalandı';
    btn.classList.add('copied');
    setTimeout(() => {
      btn.textContent = orig;
      btn.classList.remove('copied');
    }, 2000);
  });
}

function downloadImg(btn, src, domain) {
  const a = document.createElement('a');
  a.href = src;
  a.download = 'penche_' + domain + '_' + Date.now() + '.jpg';
  a.click();
}
</script>
</body>
</html>`

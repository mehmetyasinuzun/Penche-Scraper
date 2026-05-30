import 'webextension-polyfill';
import {
  loadConfig,
  saveConfig,
  saveDomainProfile,
  deleteDomainProfile,
  exportConfig,
  importConfig,
} from '../shared/config';
import { checkHealth } from '../background/router-client';
import { DomainProfile, ScreenshotMode, ExtMessage, PickElementResultMessage } from '../shared/types';
import { PRESETS, Preset } from '../shared/presets';

const $  = (id: string) => document.getElementById(id) as HTMLElement;
const inp = (id: string) => document.getElementById(id) as HTMLInputElement;
const sel = (id: string) => document.getElementById(id) as HTMLSelectElement;
const ta  = (id: string) => document.getElementById(id) as HTMLTextAreaElement;

// ── Navigation ─────────────────────────────────────────────────────────────

document.querySelectorAll('.sidebar nav a').forEach((link) => {
  link.addEventListener('click', (e) => {
    e.preventDefault();
    showSection((link as HTMLElement).dataset.section!);
  });
});

function showSection(section: string): void {
  document.querySelectorAll('.sidebar nav a').forEach((l) => l.classList.remove('active'));
  document.querySelectorAll('.section').forEach((s) => s.classList.remove('active'));
  document.querySelector(`.sidebar nav a[data-section="${section}"]`)?.classList.add('active');
  $(`sec-${section}`).classList.add('active');
  if (section === 'backup') loadBackupSection();
}

$('sb-gallery').addEventListener('click', async (e) => {
  e.preventDefault();
  const cfg = await loadConfig();
  browser.tabs.create({ url: `${cfg.global.router.baseUrl.replace(/\/+$/, '')}/ui` });
});

// ── Status bar ─────────────────────────────────────────────────────────────

let statusTimer: ReturnType<typeof setTimeout>;
function setStatus(msg: string, isError = false): void {
  const el = $('status-msg');
  el.textContent = msg;
  el.className = isError ? 'err' : 'msg';
  clearTimeout(statusTimer);
  statusTimer = setTimeout(() => { el.textContent = ''; }, 4000);
}

// ── Global section ─────────────────────────────────────────────────────────

async function loadGlobalSection(): Promise<void> {
  const cfg = await loadConfig();
  inp('router-url').value         = cfg.global.router.baseUrl;
  inp('router-secret').value      = cfg.global.router.sharedSecret;
  inp('router-timeout').value     = String(cfg.global.router.timeoutMs);
  sel('screenshot-mode').value    = cfg.global.defaultScreenshot.mode;
  inp('screenshot-toppx').value   = String(cfg.global.defaultScreenshot.topPx ?? 2200);
  inp('screenshot-maxh').value    = String(cfg.global.defaultScreenshot.maxFullPageHeightPx ?? 16000);
  sel('screenshot-format').value  = cfg.global.defaultScreenshot.imageType ?? 'jpeg';
  inp('screenshot-quality').value = String(cfg.global.defaultScreenshot.imageQuality ?? 0.82);
  updateTopPxVisibility();
}

sel('screenshot-mode').addEventListener('change', updateTopPxVisibility);
function updateTopPxVisibility(): void {
  $('row-top-px').style.display = sel('screenshot-mode').value === 'top_px' ? 'block' : 'none';
}

$('secret-toggle').addEventListener('click', () => {
  const field = inp('router-secret');
  const show = field.type === 'password';
  field.type = show ? 'text' : 'password';
  $('secret-toggle').textContent = show ? 'Gizle' : 'Göster';
});

$('save-global-btn').addEventListener('click', async () => {
  try {
    const cfg = await loadConfig();
    cfg.global.router.baseUrl      = inp('router-url').value.trim();
    cfg.global.router.sharedSecret = inp('router-secret').value.trim();
    cfg.global.router.timeoutMs    = parseInt(inp('router-timeout').value) || 7000;
    cfg.global.defaultScreenshot.mode                = sel('screenshot-mode').value as ScreenshotMode;
    cfg.global.defaultScreenshot.topPx               = parseInt(inp('screenshot-toppx').value) || 2200;
    cfg.global.defaultScreenshot.maxFullPageHeightPx = parseInt(inp('screenshot-maxh').value) || 16000;
    cfg.global.defaultScreenshot.imageType           = sel('screenshot-format').value as 'jpeg' | 'png' | 'webp';
    cfg.global.defaultScreenshot.imageQuality        = parseFloat(inp('screenshot-quality').value) || 0.82;
    await saveConfig(cfg);
    setStatus('✓ Ayarlar kaydedildi.');
  } catch (err) {
    setStatus(`Kayıt başarısız: ${err}`, true);
  }
});

$('test-router-btn').addEventListener('click', async () => {
  setStatus('Router test ediliyor…');
  const cfg = await loadConfig();
  cfg.global.router.baseUrl      = inp('router-url').value.trim() || cfg.global.router.baseUrl;
  cfg.global.router.sharedSecret = inp('router-secret').value.trim() || cfg.global.router.sharedSecret;
  const ok = await checkHealth(cfg.global.router);
  setStatus(ok ? '✓ Router çevrimiçi.' : '✗ Router erişilemiyor.', !ok);
});

// ── Presets ────────────────────────────────────────────────────────────────

async function renderPresets(): Promise<void> {
  const cfg = await loadConfig();
  const grid = $('preset-grid');
  grid.innerHTML = '';
  for (const p of PRESETS) {
    const added = !!cfg.domains[p.host];
    const btn = document.createElement('button');
    btn.className = 'preset';
    btn.disabled = added;
    btn.innerHTML = `<span class="n">${escHtml(p.name)}</span><span class="h">${escHtml(p.host)}</span>` +
      (added ? '<span class="added">✓ Eklendi</span>' : `<span class="h" style="color:var(--faint)">${escHtml((p.tags ?? []).join(', '))}</span>`);
    btn.addEventListener('click', () => applyPreset(p));
    grid.appendChild(btn);
  }
}

async function applyPreset(p: Preset): Promise<void> {
  const profile: DomainProfile = {
    enabled: true,
    match: { host: p.host, ...(p.pathRegex ? { pathRegex: p.pathRegex } : {}) },
    title: { primarySelector: p.primary, fallbackSelectors: p.fallbacks ?? [] },
    tags: p.tags ?? [],
    ...(p.mode ? { screenshot: { mode: p.mode, ...(p.topPx ? { topPx: p.topPx } : {}) } } : {}),
  };
  await saveDomainProfile(p.host, profile);
  setStatus(`✓ "${p.name}" profili eklendi.`);
  await renderPresets();
  await loadDomainList();
  openEditor(p.host, profile);
}

// ── Domain profiles ────────────────────────────────────────────────────────

async function loadDomainList(): Promise<void> {
  const cfg = await loadConfig();
  const list = $('domain-list') as HTMLUListElement;
  list.innerHTML = '';

  const entries = Object.entries(cfg.domains);
  if (entries.length === 0) {
    list.innerHTML = '<li class="empty-hint">Henüz profil yok. Yukarıdan hazır bir profil seç ya da boş profil ekle.</li>';
    return;
  }

  for (const [domain, profile] of entries) {
    const li = document.createElement('li');

    const sw = document.createElement('span');
    sw.className = `sw ${profile.enabled ? 'on' : ''}`;
    sw.title = profile.enabled ? 'Etkin — kapatmak için tıkla' : 'Devre dışı — açmak için tıkla';
    sw.addEventListener('click', async () => {
      profile.enabled = !profile.enabled;
      await saveDomainProfile(domain, profile);
      await loadDomainList();
    });

    const info = document.createElement('div');
    info.className = 'grow';
    info.innerHTML = `<div class="d-name">${escHtml(domain)}</div>` +
      `<div class="d-meta">${escHtml(profile.screenshot?.mode ?? 'genel mod')}${(profile.tags?.length ? ' · ' + escHtml(profile.tags.join(', ')) : '')}</div>`;

    const editBtn = document.createElement('button');
    editBtn.className = 'btn secondary';
    editBtn.style.cssText = 'padding:6px 12px;font-size:12px';
    editBtn.textContent = 'Düzenle';
    editBtn.addEventListener('click', () => openEditor(domain, profile));

    li.append(sw, info, editBtn);
    list.appendChild(li);
  }
}

function openEditor(key: string, profile: DomainProfile): void {
  const isNew = key === '__new__';
  $('editor-title').textContent = isNew ? 'Yeni Domain Profili' : `Düzenle: ${key}`;
  inp('d-host').value           = isNew ? '' : profile.match.host;
  inp('d-path-regex').value     = profile.match.pathRegex ?? '';
  sel('d-enabled').value        = String(profile.enabled);
  inp('d-tags').value           = (profile.tags ?? []).join(', ');
  inp('d-title-primary').value  = profile.title.primarySelector;
  ta('d-title-fallbacks').value = (profile.title.fallbackSelectors ?? []).join('\n');
  sel('d-ss-mode').value        = profile.screenshot?.mode ?? '';
  inp('d-ss-toppx').value       = profile.screenshot?.topPx ? String(profile.screenshot.topPx) : '';
  $('title-preview').textContent = 'Henüz önizleme yok';
  $('domain-editor').style.display = 'block';
  $('domain-editor').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

$('add-domain-btn').addEventListener('click', () => {
  openEditor('__new__', {
    enabled: true,
    match: { host: '' },
    title: { primarySelector: 'h1', fallbackSelectors: ["meta[property='og:title']"] },
    tags: ['cti', 'forum'],
  });
});

$('cancel-domain-btn').addEventListener('click', () => {
  $('domain-editor').style.display = 'none';
});

$('save-domain-btn').addEventListener('click', async () => {
  const host = inp('d-host').value.trim();
  if (!host) { setStatus('Domain host zorunlu.', true); return; }

  const profile: DomainProfile = {
    enabled: sel('d-enabled').value === 'true',
    match: {
      host,
      pathRegex: inp('d-path-regex').value.trim() || undefined,
    },
    title: {
      primarySelector: inp('d-title-primary').value.trim() || 'h1',
      fallbackSelectors: ta('d-title-fallbacks').value.split('\n').map((s) => s.trim()).filter(Boolean),
    },
    tags: inp('d-tags').value.split(',').map((s) => s.trim()).filter(Boolean),
    screenshot: buildScreenshotOverride(),
  };

  try {
    await saveDomainProfile(host, profile);
    setStatus(`✓ "${host}" profili kaydedildi.`);
    $('domain-editor').style.display = 'none';
    await renderPresets();
    await loadDomainList();
  } catch (err) {
    setStatus(`Kayıt başarısız: ${err}`, true);
  }
});

$('delete-domain-btn').addEventListener('click', async () => {
  const host = inp('d-host').value.trim();
  if (!host) return;
  if (!confirm(`"${host}" profilini sil?`)) return;
  await deleteDomainProfile(host);
  setStatus(`"${host}" profili silindi.`);
  $('domain-editor').style.display = 'none';
  await renderPresets();
  await loadDomainList();
});

$('test-domain-btn').addEventListener('click', async () => {
  const selector = inp('d-title-primary').value.trim();
  if (!selector) { setStatus('Önce bir seçici gir.', true); return; }

  const tab = await findTargetTab();
  if (!tab?.id) { setStatus('Açık bir forum sekmesi bulunamadı.', true); return; }

  try {
    const results = await browser.scripting.executeScript({
      target: { tabId: tab.id },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      func: ((s: string) => {
        const el = document.querySelector(s);
        if (!el) return null;
        if (s.indexOf('meta[') === 0) return (el as HTMLMetaElement).content?.trim() || 'Bulundu (içerik yok)';
        return (el as HTMLElement).innerText?.trim() || el.textContent?.trim() || 'Bulundu (metin yok)';
      }) as (...args: any[]) => any,
      args: [selector],
    });
    const text = (results[0] as { result: string | null }).result;
    $('title-preview').textContent = text
      ? `✓ [${tab.url ? hostOf(tab.url) : '?'}] "${text.slice(0, 200)}"`
      : `✗ ${tab.url ? hostOf(tab.url) : 'sekme'} sayfasında element bulunamadı`;
  } catch (err) {
    $('title-preview').textContent = `Hata: ${err}`;
  }
});

// ── Element picker ─────────────────────────────────────────────────────────

$('pick-title-btn').addEventListener('click', async () => {
  const tab = await findTargetTab();
  if (!tab?.id) { setStatus('Açık bir forum sekmesi bulunamadı. Forumu sekmede açık tut.', true); return; }

  const host = inp('d-host').value.trim() || (tab.url ? hostOf(tab.url) : 'unknown');

  setStatus(`"${hostOf(tab.url ?? '')}" sekmesinde bir öğeye tıkla…`);
  try {
    await browser.tabs.update(tab.id, { active: true });
    await browser.tabs.sendMessage(tab.id, { type: 'PICK_ELEMENT_START', tabId: tab.id, domain: host });
  } catch {
    setStatus('Picker enjekte edilemedi. Forum sekmesini yenile ve tekrar dene.', true);
  }
});

browser.runtime.onMessage.addListener((msg: ExtMessage) => {
  if (msg.type === 'PICK_ELEMENT_RESULT') {
    const m = msg as PickElementResultMessage;
    inp('d-title-primary').value = m.selector;
    $('title-preview').textContent = `✓ "${m.previewText.slice(0, 200)}"`;
    setStatus(`Seçici: ${m.selector}`);
  }
  if (msg.type === 'PICK_ELEMENT_CANCEL') {
    setStatus('Picker iptal edildi.');
  }
});

function buildScreenshotOverride(): DomainProfile['screenshot'] {
  const mode = sel('d-ss-mode').value as ScreenshotMode | '';
  if (!mode) return undefined;
  const override: DomainProfile['screenshot'] = { mode };
  const topPx = parseInt(inp('d-ss-toppx').value);
  if (!isNaN(topPx) && topPx > 0) override!.topPx = topPx;
  return override;
}

/** Find the forum tab to operate on — the most recently active http(s) tab
 *  that is NOT this options page. Options opens in its own tab, so querying
 *  the "active" tab would otherwise target the options page itself. */
async function findTargetTab(): Promise<{ id?: number; url?: string } | null> {
  const all = await browser.tabs.query({});
  const self = await browser.tabs.getCurrent().catch(() => undefined);
  const web = all.filter((t) => t.url && /^https?:/i.test(t.url) && t.id !== self?.id);
  if (!web.length) return null;
  web.sort((a, b) =>
    (Number(b.active) - Number(a.active)) ||
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (((b as any).lastAccessed ?? 0) - ((a as any).lastAccessed ?? 0)));
  return web[0];
}

function hostOf(u: string): string {
  try { return new URL(u).hostname; } catch { return u; }
}

// ── Backup ─────────────────────────────────────────────────────────────────

async function loadBackupSection(): Promise<void> {
  ta('config-json').value = await exportConfig();
}

$('export-btn').addEventListener('click', async () => {
  const json = await exportConfig();
  ta('config-json').value = json;
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `penche-config-${new Date().toISOString().slice(0, 10)}.json`;
  a.click();
  URL.revokeObjectURL(url);
  setStatus('✓ Config dışa aktarıldı.');
});

$('import-btn').addEventListener('click', async () => {
  const json = ta('config-json').value.trim();
  if (!json) { setStatus('Önce JSON yapıştır.', true); return; }
  await doImport(json);
});

inp('import-file').addEventListener('change', async (e) => {
  const file = (e.target as HTMLInputElement).files?.[0];
  if (!file) return;
  const text = await file.text();
  ta('config-json').value = text;
  await doImport(text);
});

async function doImport(json: string): Promise<void> {
  try {
    await importConfig(json);
    setStatus('✓ Config içe aktarıldı.');
    await loadGlobalSection();
    await renderPresets();
    await loadDomainList();
  } catch (err) {
    setStatus(`İçe aktarma başarısız: ${err}`, true);
  }
}

// ── Helpers ────────────────────────────────────────────────────────────────

function escHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Init ───────────────────────────────────────────────────────────────────

(async () => {
  await loadGlobalSection();
  await renderPresets();
  await loadDomainList();

  // If opened via the popup's "Profil ekle" link, jump straight into a
  // prefilled editor for that domain.
  const pending = await browser.storage.local.get('penche_add_domain');
  const host = pending.penche_add_domain as string | undefined;
  if (host) {
    await browser.storage.local.remove('penche_add_domain');
    const cfg = await loadConfig();
    showSection('domains');
    const existing = cfg.domains[host];
    openEditor(existing ? host : '__new__', existing ?? {
      enabled: true,
      match: { host },
      title: { primarySelector: 'h1', fallbackSelectors: ["meta[property='og:title']"] },
      tags: ['cti', 'forum'],
    });
    if (!existing) inp('d-host').value = host;
  }
})();

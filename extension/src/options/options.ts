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

const $  = (id: string) => document.getElementById(id) as HTMLElement;
const inp = (id: string) => document.getElementById(id) as HTMLInputElement;
const sel = (id: string) => document.getElementById(id) as HTMLSelectElement;
const ta  = (id: string) => document.getElementById(id) as HTMLTextAreaElement;

// ── Nav ───────────────────────────────────────────────────────────────────────

document.querySelectorAll('.sidebar nav a').forEach((link) => {
  link.addEventListener('click', (e) => {
    e.preventDefault();
    const section = (link as HTMLElement).dataset.section!;
    document.querySelectorAll('.sidebar nav a').forEach((l) => l.classList.remove('active'));
    document.querySelectorAll('.section').forEach((s) => s.classList.remove('active'));
    link.classList.add('active');
    $(`sec-${section}`).classList.add('active');
    if (section === 'backup') loadBackupSection();
  });
});

// ── Status bar ────────────────────────────────────────────────────────────────

let statusTimer: ReturnType<typeof setTimeout>;
function setStatus(msg: string, isError = false): void {
  const el = $('status-msg');
  el.textContent = msg;
  el.className = isError ? 'err' : 'msg';
  clearTimeout(statusTimer);
  statusTimer = setTimeout(() => { el.textContent = ''; }, 4000);
}

// ── Global section ────────────────────────────────────────────────────────────

async function loadGlobalSection(): Promise<void> {
  const cfg = await loadConfig();
  inp('router-url').value      = cfg.global.router.baseUrl;
  inp('router-secret').value   = cfg.global.router.sharedSecret;
  inp('router-timeout').value  = String(cfg.global.router.timeoutMs);
  sel('screenshot-mode').value = cfg.global.defaultScreenshot.mode;
  inp('screenshot-toppx').value  = String(cfg.global.defaultScreenshot.topPx ?? 2200);
  inp('screenshot-maxh').value   = String(cfg.global.defaultScreenshot.maxFullPageHeightPx ?? 16000);
  sel('screenshot-format').value = cfg.global.defaultScreenshot.imageType ?? 'jpeg';
  inp('screenshot-quality').value = String(cfg.global.defaultScreenshot.imageQuality ?? 0.82);
  updateTopPxVisibility();
}

sel('screenshot-mode').addEventListener('change', updateTopPxVisibility);

function updateTopPxVisibility(): void {
  $('row-top-px').style.display = sel('screenshot-mode').value === 'top_px' ? 'block' : 'none';
}

$('save-global-btn').addEventListener('click', async () => {
  try {
    const cfg = await loadConfig();
    cfg.global.router.baseUrl      = inp('router-url').value.trim();
    cfg.global.router.sharedSecret = inp('router-secret').value.trim();
    cfg.global.router.timeoutMs    = parseInt(inp('router-timeout').value) || 7000;
    cfg.global.defaultScreenshot.mode           = sel('screenshot-mode').value as ScreenshotMode;
    cfg.global.defaultScreenshot.topPx          = parseInt(inp('screenshot-toppx').value) || 2200;
    cfg.global.defaultScreenshot.maxFullPageHeightPx = parseInt(inp('screenshot-maxh').value) || 16000;
    cfg.global.defaultScreenshot.imageType      = sel('screenshot-format').value as 'jpeg' | 'png' | 'webp';
    cfg.global.defaultScreenshot.imageQuality   = parseFloat(inp('screenshot-quality').value) || 0.82;
    await saveConfig(cfg);
    setStatus('Ayarlar kaydedildi.');
  } catch (err) {
    setStatus(`Kayıt başarısız: ${err}`, true);
  }
});

$('test-router-btn').addEventListener('click', async () => {
  setStatus('Router test ediliyor…');
  const cfg = await loadConfig();
  const ok = await checkHealth(cfg.global.router);
  setStatus(ok ? '✓ Router çevrimiçi.' : '✗ Router erişilemiyor.', !ok);
});

// ── Domain profiles ───────────────────────────────────────────────────────────

/** Key of the profile currently open in the editor. Null when editor is closed. */
let activeEditKey: string | null = null;

async function loadDomainList(): Promise<void> {
  const cfg = await loadConfig();
  const list = $('domain-list') as HTMLUListElement;
  list.innerHTML = '';

  const entries = Object.entries(cfg.domains);
  if (entries.length === 0) {
    list.innerHTML = '<li style="color:#4b5563; padding:10px 0;">Henüz profil yok.</li>';
    return;
  }

  for (const [domain, profile] of entries) {
    const li = document.createElement('li');
    li.innerHTML = `
      <span class="d-enabled ${profile.enabled ? '' : 'off'}"></span>
      <span class="d-name">${escHtml(domain)}</span>
      <span class="d-mode">${profile.screenshot?.mode ?? 'global'} · ${(profile.tags ?? []).join(', ')}</span>
      <button class="btn secondary" style="padding:4px 10px;font-size:12px;">Düzenle</button>
    `;
    li.querySelector('button')!.addEventListener('click', () => openEditor(domain, profile));
    list.appendChild(li);
  }
}

function openEditor(key: string, profile: DomainProfile): void {
  activeEditKey = key === '__new__' ? null : key;
  $('editor-title').textContent = key === '__new__' ? 'Yeni Domain Profili' : `Düzenle: ${key}`;
  inp('d-host').value         = key === '__new__' ? '' : profile.match.host;
  inp('d-path-regex').value   = profile.match.pathRegex ?? '';
  sel('d-enabled').value      = String(profile.enabled);
  inp('d-tags').value         = (profile.tags ?? []).join(', ');
  inp('d-title-primary').value = profile.title.primarySelector;
  ta('d-title-fallbacks').value = (profile.title.fallbackSelectors ?? []).join('\n');
  sel('d-ss-mode').value      = profile.screenshot?.mode ?? '';
  inp('d-ss-toppx').value     = profile.screenshot?.topPx ? String(profile.screenshot.topPx) : '';
  $('title-preview').textContent = 'Henüz önizleme yok';
  $('domain-editor').style.display = 'block';
  $('domain-editor').scrollIntoView({ behavior: 'smooth' });
}

$('add-domain-btn').addEventListener('click', () => {
  openEditor('__new__', {
    enabled: true,
    match: { host: '' },
    title: { primarySelector: 'h1', fallbackSelectors: [] },
    tags: [],
  });
});

$('cancel-domain-btn').addEventListener('click', () => {
  $('domain-editor').style.display = 'none';
  activeEditKey = null;
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
      fallbackSelectors: ta('d-title-fallbacks').value
        .split('\n').map((s) => s.trim()).filter(Boolean),
    },
    tags: inp('d-tags').value.split(',').map((s) => s.trim()).filter(Boolean),
    screenshot: buildScreenshotOverride(),
  };

  try {
    await saveDomainProfile(host, profile);
    setStatus(`"${host}" profili kaydedildi.`);
    $('domain-editor').style.display = 'none';
    activeEditKey = null;
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
  activeEditKey = null;
  await loadDomainList();
});

$('test-domain-btn').addEventListener('click', async () => {
  const selector = inp('d-title-primary').value.trim();
  if (!selector) { setStatus('Önce bir selector gir.', true); return; }

  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) { setStatus('Aktif sekme bulunamadı.', true); return; }

  try {
    const results = await browser.scripting.executeScript({
      target: { tabId: tab.id },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      func: ((s: string) => {
        const el = document.querySelector(s);
        return el ? ((el as HTMLElement).innerText?.trim() || el.textContent?.trim() || 'Bulundu (metin yok)') : null;
      }) as (...args: any[]) => any,
      args: [selector],
    });
    const text = (results[0] as { result: string | null }).result;
    $('title-preview').textContent = text
      ? `✓ "${text.slice(0, 200)}"`
      : '✗ Mevcut sayfada element bulunamadı';
  } catch (err) {
    $('title-preview').textContent = `Hata: ${err}`;
  }
});

// ── Element picker ────────────────────────────────────────────────────────────

$('pick-title-btn').addEventListener('click', async () => {
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) { setStatus('Aktif sekme bulunamadı.', true); return; }

  const host = inp('d-host').value.trim()
    || (tab.url ? new URL(tab.url).hostname : 'unknown');

  setStatus('Sayfada bir öğeye tıkla…');
  try {
    await browser.tabs.sendMessage(tab.id, {
      type: 'PICK_ELEMENT_START',
      tabId: tab.id,
      domain: host,
    });
  } catch {
    setStatus('Picker enjekte edilemedi. Sayfayı yenile ve tekrar dene.', true);
  }
});

browser.runtime.onMessage.addListener((msg: ExtMessage) => {
  if (msg.type === 'PICK_ELEMENT_RESULT') {
    const m = msg as PickElementResultMessage;
    inp('d-title-primary').value = m.selector;
    $('title-preview').textContent = `✓ "${m.previewText.slice(0, 200)}"`;
    setStatus(`Selector: ${m.selector}`);
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

// ── Backup ────────────────────────────────────────────────────────────────────

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
  a.download = `penche-config-${Date.now()}.json`;
  a.click();
  URL.revokeObjectURL(url);
  setStatus('Config dışa aktarıldı.');
});

$('import-btn').addEventListener('click', async () => {
  const json = ta('config-json').value.trim();
  if (!json) { setStatus('Önce JSON yapıştır.', true); return; }
  try {
    await importConfig(json);
    setStatus('Config içe aktarıldı.');
    await loadGlobalSection();
    await loadDomainList();
  } catch (err) {
    setStatus(`İçe aktarma başarısız: ${err}`, true);
  }
});

// ── Helpers ───────────────────────────────────────────────────────────────────

function escHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Init ──────────────────────────────────────────────────────────────────────

(async () => {
  await loadGlobalSection();
  await loadDomainList();
  // suppress unused warning — variable is used for editor open/close state tracking
  void activeEditKey;
})();

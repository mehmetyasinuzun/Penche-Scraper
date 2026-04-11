import {
  loadConfig,
  saveConfig,
  saveDomainProfile,
  deleteDomainProfile,
  exportConfig,
  importConfig,
  resolveProfile,
} from '../shared/config';
import { checkHealth } from '../background/router-client';
import {
  AppConfig,
  DomainProfile,
  ScreenshotMode,
  ExtMessage,
  PickElementResultMessage,
} from '../shared/types';

const $ = (id: string) => document.getElementById(id) as HTMLElement;
const inp = (id: string) => document.getElementById(id) as HTMLInputElement;
const sel = (id: string) => document.getElementById(id) as HTMLSelectElement;
const ta = (id: string) => document.getElementById(id) as HTMLTextAreaElement;

// ── Nav ──────────────────────────────────────────────────────────────────────

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

// ── Status ───────────────────────────────────────────────────────────────────

let statusTimer: ReturnType<typeof setTimeout>;
function setStatus(msg: string, isError = false): void {
  const el = $('status-msg');
  el.textContent = msg;
  el.className = isError ? 'err' : 'msg';
  clearTimeout(statusTimer);
  statusTimer = setTimeout(() => { el.textContent = ''; }, 4000);
}

// ── Global Section ────────────────────────────────────────────────────────────

async function loadGlobalSection(): Promise<void> {
  const cfg = await loadConfig();
  inp('router-url').value = cfg.global.router.baseUrl;
  inp('router-secret').value = cfg.global.router.sharedSecret;
  inp('router-timeout').value = String(cfg.global.router.timeoutMs);
  sel('screenshot-mode').value = cfg.global.defaultScreenshot.mode;
  inp('screenshot-toppx').value = String(cfg.global.defaultScreenshot.topPx ?? 2200);
  inp('screenshot-maxh').value = String(cfg.global.defaultScreenshot.maxFullPageHeightPx ?? 16000);
  sel('screenshot-format').value = cfg.global.defaultScreenshot.imageType ?? 'jpeg';
  inp('screenshot-quality').value = String(cfg.global.defaultScreenshot.imageQuality ?? 0.82);
  updateTopPxVisibility();
}

sel('screenshot-mode').addEventListener('change', updateTopPxVisibility);
function updateTopPxVisibility(): void {
  const show = sel('screenshot-mode').value === 'top_px';
  ($('row-top-px') as HTMLElement).style.display = show ? 'block' : 'none';
}

$('save-global-btn').addEventListener('click', async () => {
  try {
    const cfg = await loadConfig();
    cfg.global.router.baseUrl = inp('router-url').value.trim();
    cfg.global.router.sharedSecret = inp('router-secret').value.trim();
    cfg.global.router.timeoutMs = parseInt(inp('router-timeout').value) || 7000;
    cfg.global.defaultScreenshot.mode = sel('screenshot-mode').value as ScreenshotMode;
    cfg.global.defaultScreenshot.topPx = parseInt(inp('screenshot-toppx').value) || 2200;
    cfg.global.defaultScreenshot.maxFullPageHeightPx = parseInt(inp('screenshot-maxh').value) || 16000;
    cfg.global.defaultScreenshot.imageType = sel('screenshot-format').value as any;
    cfg.global.defaultScreenshot.imageQuality = parseFloat(inp('screenshot-quality').value) || 0.82;
    await saveConfig(cfg);
    setStatus('Global settings saved.');
  } catch (err) {
    setStatus(`Save failed: ${err}`, true);
  }
});

$('test-router-btn').addEventListener('click', async () => {
  setStatus('Testing router…');
  const cfg = await loadConfig();
  const ok = await checkHealth(cfg.global.router);
  setStatus(ok ? '✓ Router is online.' : '✗ Router not reachable.', !ok);
});

// ── Domain Section ────────────────────────────────────────────────────────────

let editingDomain: string | null = null;

async function loadDomainList(): Promise<void> {
  const cfg = await loadConfig();
  const list = $('domain-list') as HTMLUListElement;
  list.innerHTML = '';

  if (Object.keys(cfg.domains).length === 0) {
    list.innerHTML = '<li style="color:#4b5563; padding:10px 0;">No profiles yet.</li>';
    return;
  }

  for (const [domain, profile] of Object.entries(cfg.domains)) {
    const li = document.createElement('li');
    li.innerHTML = `
      <span class="d-enabled ${profile.enabled ? '' : 'off'}"></span>
      <span class="d-name">${escHtml(domain)}</span>
      <span class="d-mode">${profile.screenshot?.mode ?? 'default'} · ${(profile.tags ?? []).join(', ')}</span>
      <button class="btn secondary" style="padding:4px 10px; font-size:12px;">Edit</button>
    `;
    li.querySelector('button')!.addEventListener('click', () => openEditor(domain, profile));
    list.appendChild(li);
  }
}

function openEditor(domain: string, profile: DomainProfile): void {
  editingDomain = domain;
  $('editor-title').textContent = domain === '__new__' ? 'New Domain Profile' : `Edit: ${domain}`;
  inp('d-host').value = domain === '__new__' ? '' : profile.match.host;
  inp('d-path-regex').value = profile.match.pathRegex ?? '';
  sel('d-enabled').value = String(profile.enabled);
  inp('d-tags').value = (profile.tags ?? []).join(', ');
  inp('d-title-primary').value = profile.title.primarySelector;
  ta('d-title-fallbacks').value = (profile.title.fallbackSelectors ?? []).join('\n');
  sel('d-ss-mode').value = profile.screenshot?.mode ?? '';
  inp('d-ss-toppx').value = profile.screenshot?.topPx ? String(profile.screenshot.topPx) : '';
  $('title-preview').textContent = 'No preview yet';
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
  editingDomain = null;
});

$('save-domain-btn').addEventListener('click', async () => {
  const host = inp('d-host').value.trim();
  if (!host) { setStatus('Domain host is required.', true); return; }

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
    setStatus(`Profile "${host}" saved.`);
    $('domain-editor').style.display = 'none';
    editingDomain = null;
    await loadDomainList();
  } catch (err) {
    setStatus(`Save failed: ${err}`, true);
  }
});

$('delete-domain-btn').addEventListener('click', async () => {
  const host = inp('d-host').value.trim();
  if (!host) return;
  if (!confirm(`Delete profile for "${host}"?`)) return;
  await deleteDomainProfile(host);
  setStatus(`Profile "${host}" deleted.`);
  $('domain-editor').style.display = 'none';
  editingDomain = null;
  await loadDomainList();
});

$('test-domain-btn').addEventListener('click', async () => {
  const selector = inp('d-title-primary').value.trim();
  if (!selector) { setStatus('Enter a selector first.', true); return; }

  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) { setStatus('No active tab to test against.', true); return; }

  try {
    const result = await browser.scripting.executeScript({
      target: { tabId: tab.id },
      func: (sel: string) => {
        const el = document.querySelector(sel);
        return el ? (el as HTMLElement).innerText?.trim() || el.textContent?.trim() || 'Found (no text)' : null;
      },
      args: [selector],
    });
    const text = (result[0] as any).result;
    $('title-preview').textContent = text ? `✓ "${text.slice(0, 200)}"` : '✗ Element not found on current page';
  } catch (err) {
    $('title-preview').textContent = `Error: ${err}`;
  }
});

// ── Element Picker ────────────────────────────────────────────────────────────

$('pick-title-btn').addEventListener('click', async () => {
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) { setStatus('No active tab.', true); return; }

  const host = inp('d-host').value.trim() || tab.url ? new URL(tab.url!).hostname : 'unknown';

  setStatus('Click an element on the page to select it…');

  try {
    await browser.tabs.sendMessage(tab.id, {
      type: 'PICK_ELEMENT_START',
      tabId: tab.id,
      domain: host,
    });
  } catch {
    setStatus('Could not inject picker. Try reloading the page.', true);
  }
});

// Listen for picker result from content script.
browser.runtime.onMessage.addListener((msg: ExtMessage) => {
  if (msg.type === 'PICK_ELEMENT_RESULT') {
    const m = msg as PickElementResultMessage;
    inp('d-title-primary').value = m.selector;
    $('title-preview').textContent = `✓ "${m.previewText.slice(0, 200)}"`;
    setStatus(`Selector picked: ${m.selector}`);
  }
  if (msg.type === 'PICK_ELEMENT_CANCEL') {
    setStatus('Picker cancelled.');
  }
});

function buildScreenshotOverride(): DomainProfile['screenshot'] {
  const mode = sel('d-ss-mode').value as ScreenshotMode | '';
  if (!mode) return undefined;
  const override: DomainProfile['screenshot'] = { mode: mode as ScreenshotMode };
  const topPx = parseInt(inp('d-ss-toppx').value);
  if (!isNaN(topPx) && topPx > 0) override.topPx = topPx;
  return override;
}

// ── Backup Section ────────────────────────────────────────────────────────────

async function loadBackupSection(): Promise<void> {
  const json = await exportConfig();
  ta('config-json').value = json;
}

$('export-btn').addEventListener('click', async () => {
  const json = await exportConfig();
  ta('config-json').value = json;
  // Also trigger file download.
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `penche-config-${Date.now()}.json`;
  a.click();
  URL.revokeObjectURL(url);
  setStatus('Config exported.');
});

$('import-btn').addEventListener('click', async () => {
  const json = ta('config-json').value.trim();
  if (!json) { setStatus('Paste JSON config first.', true); return; }
  try {
    await importConfig(json);
    setStatus('Config imported successfully.');
    await loadGlobalSection();
    await loadDomainList();
  } catch (err) {
    setStatus(`Import failed: ${err}`, true);
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
})();

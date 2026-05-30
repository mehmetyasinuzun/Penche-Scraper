import 'webextension-polyfill';
import { loadConfig, resolveProfile } from '../shared/config';
import { checkHealth } from '../background/router-client';
import { ExtMessage, CaptureResultMessage } from '../shared/types';

const $ = (id: string) => document.getElementById(id)!;

let currentHost = '';
let galleryBase = 'http://127.0.0.1:8787';

async function init(): Promise<void> {
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  const url = tab?.url ? safeUrl(tab.url) : null;
  currentHost = url?.hostname ?? '';

  ($('domain-val') as HTMLElement).textContent = currentHost || '—';

  const cfg = await loadConfig();
  galleryBase = cfg.global.router.baseUrl.replace(/\/+$/, '');

  const match = url ? resolveProfile(cfg, url.hostname, url.pathname) : null;
  const tag = $('profile-tag');
  const addLink = $('add-profile-link');
  if (match) {
    tag.textContent = match.profileId;
    ($('profile-row') as HTMLElement).className = 'profile ok';
    addLink.hidden = true;
  } else {
    tag.textContent = currentHost ? 'Profil yok' : '—';
    ($('profile-row') as HTMLElement).className = 'profile warn';
    addLink.hidden = !currentHost;
  }

  browser.runtime.sendMessage({ type: 'OUTBOX_STATUS' })
    .then((r: any) => { ($('outbox-val') as HTMLElement).textContent = String(r?.pendingCount ?? 0); })
    .catch(() => {});

  setRouterPill(await checkHealth(cfg.global.router));

  const stored = await browser.storage.local.get('penche_last_capture');
  if (stored.penche_last_capture) {
    const last = stored.penche_last_capture as { ts: string; status: string };
    ($('last-capture-val') as HTMLElement).textContent =
      `${statusLabel(last.status)} · ${new Date(last.ts).toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' })}`;
  }

  showShortcut();
}

function setRouterPill(online: boolean): void {
  const pill = $('router-pill');
  pill.className = `pill ${online ? 'online' : 'offline'}`;
  ($('router-text') as HTMLElement).textContent = online ? 'Çevrimiçi' : 'Çevrimdışı';
}

function statusLabel(s: string): string {
  return s === 'sent' ? 'Gönderildi' : s === 'queued' ? 'Kuyrukta' : s;
}

function safeUrl(u: string): URL | null {
  try { return new URL(u); } catch { return null; }
}

async function showShortcut(): Promise<void> {
  try {
    const cmds = await browser.commands.getAll();
    const cap = cmds.find((c) => c.name === 'capture');
    if (cap?.shortcut) ($('shortcut-hint') as HTMLElement).textContent = cap.shortcut;
  } catch { /* commands.getAll unsupported — keep default */ }
}

$('capture-btn').addEventListener('click', async () => {
  const btn = $('capture-btn') as HTMLButtonElement;
  const statusMsg = $('status-msg');
  btn.disabled = true;
  btn.textContent = 'Yakalanıyor…';
  statusMsg.textContent = '';

  try {
    const resp = await browser.runtime.sendMessage({ type: 'CAPTURE_REQUEST' });
    const r = resp as any;

    if (r?.status === 'success') {
      setMsg(`✓ Gönderildi (${r.eventId?.slice(0, 8)}…)`, 'var(--green)');
      await browser.storage.local.set({ penche_last_capture: { ts: new Date().toISOString(), status: 'sent' } });
      ($('last-capture-val') as HTMLElement).textContent =
        `Gönderildi · ${new Date().toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' })}`;
    } else if (r?.status === 'queued') {
      setMsg('⚠ Router çevrimdışı — kuyruğa alındı', 'var(--amber)');
    } else if (r?.status === 'no_profile') {
      setMsg(`Bu site için profil yok: "${r.domain}"`, 'var(--red)');
      $('add-profile-link').hidden = false;
    } else {
      setMsg(`✗ ${r?.message ?? 'Bilinmeyen hata'}`, 'var(--red)');
    }
  } catch (err) {
    setMsg(`✗ ${err}`, 'var(--red)');
  } finally {
    btn.disabled = false;
    btn.innerHTML = btn.dataset.label || 'Şimdi Yakala';
  }
});

function setMsg(text: string, color: string): void {
  const el = $('status-msg');
  el.textContent = text;
  el.style.color = color;
}

$('gallery-link').addEventListener('click', (e) => {
  e.preventDefault();
  browser.tabs.create({ url: `${galleryBase}/ui` });
  window.close();
});

$('options-link').addEventListener('click', (e) => {
  e.preventDefault();
  browser.runtime.openOptionsPage();
  window.close();
});

$('add-profile-link').addEventListener('click', async (e) => {
  e.preventDefault();
  if (currentHost) await browser.storage.local.set({ penche_add_domain: currentHost });
  browser.runtime.openOptionsPage();
  window.close();
});

browser.runtime.onMessage.addListener((msg: ExtMessage) => {
  if (msg.type === 'CAPTURE_RESULT') {
    const m = msg as CaptureResultMessage;
    if (m.success) setMsg(`✓ Yakalandı (${m.eventId?.slice(0, 8)}…)`, 'var(--green)');
    else setMsg(`✗ ${m.error}`, 'var(--red)');
  }
});

// Cache the button's original markup so we can restore it after capture.
const capBtn = $('capture-btn') as HTMLButtonElement;
capBtn.dataset.label = capBtn.innerHTML;

init().catch(console.error);

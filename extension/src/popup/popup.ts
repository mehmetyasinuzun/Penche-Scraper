import { loadConfig, resolveProfile } from '../shared/config';
import { checkHealth } from '../background/router-client';
import { ExtMessage, CaptureResultMessage } from '../shared/types';

const $ = (id: string) => document.getElementById(id)!;

async function init(): Promise<void> {
  // Get active tab info.
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  const url = tab?.url ? new URL(tab.url) : null;
  const host = url?.hostname ?? '—';

  ($('domain-val') as HTMLElement).textContent = host;

  // Check profile match.
  const cfg = await loadConfig();
  const match = url ? resolveProfile(cfg, url.hostname, url.pathname) : null;

  const profileEl = $('profile-val');
  if (match) {
    profileEl.textContent = match.profileId;
    profileEl.className = 'value ok';
  } else {
    profileEl.textContent = 'No profile — click Options to create one';
    profileEl.className = 'value warn';
  }

  // Outbox count.
  const outboxResp = await browser.runtime.sendMessage({ type: 'OUTBOX_STATUS' });
  ($('outbox-val') as HTMLElement).textContent = String(outboxResp?.pendingCount ?? 0);

  // Router health check.
  const dot = $('router-dot');
  const healthy = await checkHealth(cfg.global.router);
  dot.className = `dot ${healthy ? 'online' : 'offline'}`;
  dot.title = healthy ? 'Router online' : 'Router offline';

  // Load last capture from storage.
  const stored = await browser.storage.local.get('penche_last_capture');
  if (stored.penche_last_capture) {
    const last = stored.penche_last_capture as { ts: string; status: string };
    ($('last-capture-val') as HTMLElement).textContent = `${last.status} · ${new Date(last.ts).toLocaleTimeString()}`;
  }
}

// Capture button.
$('capture-btn').addEventListener('click', async () => {
  const btn = $('capture-btn') as HTMLButtonElement;
  const statusMsg = $('status-msg');
  btn.disabled = true;
  btn.textContent = 'Capturing…';
  statusMsg.textContent = '';

  try {
    const resp = await browser.runtime.sendMessage({ type: 'CAPTURE_REQUEST' });
    const r = resp as any;

    if (r?.status === 'success') {
      statusMsg.textContent = `✓ Sent (${r.eventId?.slice(0, 8)}…)`;
      statusMsg.style.color = '#22c55e';
      await browser.storage.local.set({ penche_last_capture: { ts: new Date().toISOString(), status: 'sent' } });
    } else if (r?.status === 'queued') {
      statusMsg.textContent = '⚠ Router offline — queued for retry';
      statusMsg.style.color = '#f59e0b';
    } else if (r?.status === 'no_profile') {
      statusMsg.textContent = `No profile for "${r.domain}"`;
      statusMsg.style.color = '#ef4444';
    } else {
      statusMsg.textContent = `✗ ${r?.message ?? 'Unknown error'}`;
      statusMsg.style.color = '#ef4444';
    }
  } catch (err) {
    statusMsg.textContent = `✗ ${err}`;
    statusMsg.style.color = '#ef4444';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Capture Now';
  }
});

// Options link.
$('options-link').addEventListener('click', (e) => {
  e.preventDefault();
  browser.runtime.openOptionsPage();
});

// Listen for result pushed from background.
browser.runtime.onMessage.addListener((msg: ExtMessage) => {
  if (msg.type === 'CAPTURE_RESULT') {
    const m = msg as CaptureResultMessage;
    const statusMsg = $('status-msg');
    if (m.success) {
      statusMsg.textContent = `✓ Captured (${m.eventId?.slice(0, 8)}…)`;
      statusMsg.style.color = '#22c55e';
    } else {
      statusMsg.textContent = `✗ ${m.error}`;
      statusMsg.style.color = '#ef4444';
    }
  }
});

init().catch(console.error);

/**
 * Content script.
 * Responsibilities:
 *   - Display toast notifications triggered by the background script.
 *   - Handle element picker activation from the options page.
 */

import { ExtMessage, ToastMessage, PickElementStartMessage } from '../shared/types';
import { startPicker, cancelPicker } from './picker';

// ── Toast notification ──────────────────────────────────────────────────────

function showToast(msg: ToastMessage): void {
  const existing = document.getElementById('__penche_toast__');
  if (existing) existing.remove();

  const el = document.createElement('div');
  el.id = '__penche_toast__';

  const colors: Record<string, string> = {
    success: '#22c55e',
    warning: '#f59e0b',
    error: '#ef4444',
    info: '#3b82f6',
  };

  Object.assign(el.style, {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    background: '#1a1a2e',
    color: '#fff',
    borderLeft: `4px solid ${colors[msg.level] ?? '#3b82f6'}`,
    padding: '12px 18px',
    borderRadius: '6px',
    fontSize: '14px',
    fontFamily: 'system-ui, sans-serif',
    zIndex: '2147483647',
    maxWidth: '360px',
    boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
    animation: 'penche_fadein 0.2s ease',
    lineHeight: '1.4',
  });

  // Inject keyframe once.
  if (!document.getElementById('__penche_style__')) {
    const style = document.createElement('style');
    style.id = '__penche_style__';
    style.textContent = `
      @keyframes penche_fadein { from { opacity:0; transform: translateY(8px); } to { opacity:1; transform: none; } }
    `;
    document.head.appendChild(style);
  }

  const icon = { success: '✓', warning: '⚠', error: '✗', info: 'ℹ' }[msg.level] ?? 'ℹ';
  el.textContent = `${icon}  ${msg.text}`;

  document.body.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

// ── Message listener ─────────────────────────────────────────────────────────

browser.runtime.onMessage.addListener(async (msg: ExtMessage, _sender) => {
  switch (msg.type) {
    case 'TOAST':
      showToast(msg as ToastMessage);
      return { ok: true };

    case 'PICK_ELEMENT_START': {
      const m = msg as PickElementStartMessage;
      try {
        const result = await startPicker();
        // Report back to background / options page.
        await browser.runtime.sendMessage({
          type: 'PICK_ELEMENT_RESULT',
          domain: m.domain,
          selector: result.selector,
          previewText: result.previewText,
        });
      } catch {
        await browser.runtime.sendMessage({ type: 'PICK_ELEMENT_CANCEL' });
      }
      return { ok: true };
    }

    case 'PICK_ELEMENT_CANCEL':
      cancelPicker();
      return { ok: true };

    default:
      return { ok: false, error: 'unknown message' };
  }
});

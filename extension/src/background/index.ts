/**
 * Background service worker (MV3) / background script (MV2).
 * Handles:
 *   - Keyboard shortcut → capture flow
 *   - Outbox retry loop
 *   - Message routing from popup / options
 */

import 'webextension-polyfill';
import { loadConfig } from '../shared/config';
import { logger } from '../shared/logger';
import { runCapture } from './capture';
import { outboxGetDue, outboxDequeue, outboxRecordAttemptFailure, outboxCount } from './outbox';
import { sendEvent } from './router-client';
import {
  ExtMessage,
  CaptureResultMessage,
  ToastMessage,
  OutboxStatusMessage,
  ProfileSaveMessage,
  ProfileDeleteMessage,
} from '../shared/types';
import { saveDomainProfile, deleteDomainProfile } from '../shared/config';

// ── Keyboard command listener ──────────────────────────────────────────────

browser.commands.onCommand.addListener(async (command) => {
  if (command !== 'capture') return;

  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) {
    logger.warn('command: no active tab found');
    return;
  }

  logger.info('capture command received', { tabId: tab.id, url: tab.url });
  const outcome = await runCapture(tab.id);

  let toast: ToastMessage;
  let result: CaptureResultMessage;

  switch (outcome.status) {
    case 'success':
      toast = { type: 'TOAST', level: 'success', text: 'Captured and sent!' };
      result = { type: 'CAPTURE_RESULT', success: true, eventId: outcome.eventId };
      break;

    case 'queued':
      toast = { type: 'TOAST', level: 'warning', text: 'Router offline — saved to outbox.' };
      result = { type: 'CAPTURE_RESULT', success: true, eventId: outcome.eventId };
      break;

    case 'no_profile':
      toast = {
        type: 'TOAST',
        level: 'warning',
        text: `No profile for "${outcome.domain}". Create one in Options.`,
      };
      result = {
        type: 'CAPTURE_RESULT',
        success: false,
        error: `no_profile:${outcome.domain}`,
      };
      break;

    case 'error':
      toast = { type: 'TOAST', level: 'error', text: `Capture failed: ${outcome.message}` };
      result = { type: 'CAPTURE_RESULT', success: false, error: outcome.message };
      break;
  }

  // Send toast to content script for in-page notification.
  try {
    await browser.tabs.sendMessage(tab.id, toast);
  } catch {
    // Content script may not be injected (e.g., on chrome:// pages). Ignore.
  }

  // Notify popup if open.
  broadcastToPopup(result);
});

// ── Message handler ────────────────────────────────────────────────────────

browser.runtime.onMessage.addListener((msg: ExtMessage, _sender, sendResponse) => {
  handleMessage(msg).then(sendResponse).catch((err) => {
    logger.error('message handler error', { error: String(err) });
    sendResponse({ error: String(err) });
  });
  return true; // async response
});

async function handleMessage(msg: ExtMessage): Promise<unknown> {
  switch (msg.type) {
    case 'CAPTURE_REQUEST': {
      const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
      if (!tab?.id) return { error: 'no active tab' };
      const outcome = await runCapture(tab.id);
      return outcome;
    }

    case 'PROFILE_SAVE': {
      const m = msg as ProfileSaveMessage;
      await saveDomainProfile(m.domain, m.profile);
      return { ok: true };
    }

    case 'PROFILE_DELETE': {
      const m = msg as ProfileDeleteMessage;
      await deleteDomainProfile(m.domain);
      return { ok: true };
    }

    case 'OUTBOX_STATUS': {
      const count = await outboxCount();
      const reply: OutboxStatusMessage = { type: 'OUTBOX_STATUS', pendingCount: count };
      return reply;
    }

    default:
      return { error: `unknown message type: ${(msg as any).type}` };
  }
}

// ── Outbox retry loop ──────────────────────────────────────────────────────

async function drainOutbox(): Promise<void> {
  const due = await outboxGetDue();
  if (due.length === 0) return;

  const cfg = await loadConfig();
  logger.info('outbox: draining', { count: due.length });

  for (const item of due) {
    const result = await sendEvent(cfg.global.router, item.payload);
    if (result.ok) {
      logger.info('outbox: delivery succeeded', { event_id: item.id });
      await outboxDequeue(item.id);
    } else {
      logger.warn('outbox: delivery failed', { event_id: item.id, error: result.error });
      await outboxRecordAttemptFailure(item.id);
    }
  }
}

// Run drain every 30 seconds.
setInterval(drainOutbox, 30_000);
// Also run once on startup.
drainOutbox().catch((err) => logger.error('initial outbox drain failed', { error: String(err) }));

// ── Helpers ────────────────────────────────────────────────────────────────

function broadcastToPopup(msg: ExtMessage): void {
  browser.runtime.sendMessage(msg).catch(() => {
    // Popup not open — ignore.
  });
}

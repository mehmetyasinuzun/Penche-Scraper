/**
 * Capture orchestrator.
 * Called when the keyboard shortcut fires.
 * Resolves the domain profile, extracts title, takes screenshot, sends to router.
 */

import { v4 as uuidv4 } from 'uuid';
import { loadConfig, resolveProfile } from '../shared/config';
import { RouterEventPayload, ScreenshotConfig } from '../shared/types';
import { logger } from '../shared/logger';
import { takeScreenshot } from './screenshot';
import { sendEvent } from './router-client';
import { outboxEnqueue } from './outbox';

export type CaptureOutcome =
  | { status: 'success'; eventId: string }
  | { status: 'no_profile'; domain: string }
  | { status: 'queued'; eventId: string }
  | { status: 'error'; message: string };

/** Main entry point called by the background service worker. */
export async function runCapture(tabId: number): Promise<CaptureOutcome> {
  const cfg = await loadConfig();

  // Get tab info
  const tab = await browser.tabs.get(tabId);
  if (!tab.url) {
    return { status: 'error', message: 'Tab has no URL' };
  }

  let url: URL;
  try {
    url = new URL(tab.url);
  } catch {
    return { status: 'error', message: `Invalid URL: ${tab.url}` };
  }

  const host = url.hostname;
  const path = url.pathname;

  // Resolve domain profile.
  const match = resolveProfile(cfg, host, path);
  if (!match) {
    logger.info('no profile for domain', { host });
    return { status: 'no_profile', domain: host };
  }

  const { profileId, profile } = match;
  logger.info('profile matched', { profileId, host });

  // Extract page title.
  const pageTitle = await extractTitle(tabId, profile.title);
  logger.info('title extracted', { pageTitle: pageTitle.slice(0, 80) });

  // Resolve screenshot config (profile override > global default).
  const screenshotCfg: ScreenshotConfig = {
    ...cfg.global.defaultScreenshot,
    ...(profile.screenshot ?? {}),
  };

  // Take screenshot.
  let screenshot: { mime: string; base64: string };
  try {
    screenshot = await takeScreenshot(tabId, screenshotCfg);
    logger.info('screenshot captured', { mode: screenshotCfg.mode, mime: screenshot.mime });
  } catch (err) {
    logger.error('screenshot failed', { error: String(err) });
    return { status: 'error', message: `Screenshot failed: ${err}` };
  }

  // Detect browser name.
  const browserName = detectBrowser();

  const payload: RouterEventPayload = {
    event_id: uuidv4(),
    captured_at: new Date().toISOString(),
    domain: host,
    page_title: pageTitle,
    page_url: tab.url,
    screenshot,
    meta: {
      browser: browserName,
      profile_id: profileId,
      tags: profile.tags ?? [],
    },
  };

  // Try to send immediately.
  const result = await sendEvent(cfg.global.router, payload);

  if (result.ok) {
    return { status: 'success', eventId: result.eventId };
  }

  // If network error, persist to outbox for retry.
  if (result.retryable) {
    await outboxEnqueue(payload);
    return { status: 'queued', eventId: payload.event_id };
  }

  return { status: 'error', message: result.error };
}

// ────────────────────────────────────────────────────────────────────────────

async function extractTitle(tabId: number, titleCfg: { primarySelector: string; fallbackSelectors?: string[] }): Promise<string> {
  const selectors = [titleCfg.primarySelector, ...(titleCfg.fallbackSelectors ?? [])];

  const result = await execScript<string>(tabId, (sels: string[]) => {
    for (const sel of sels) {
      try {
        if (sel.startsWith('meta[')) {
          const el = document.querySelector(sel) as HTMLMetaElement | null;
          if (el?.content) return el.content.trim();
        } else {
          const el = document.querySelector(sel);
          if (el?.textContent?.trim()) return el.textContent.trim();
        }
      } catch {
        // invalid selector — continue
      }
    }
    // og:title fallback
    const og = document.querySelector("meta[property='og:title']") as HTMLMetaElement | null;
    if (og?.content) return og.content.trim();
    return document.title.trim();
  }, [selectors]);

  return result ?? tab_title_fallback();
}

function tab_title_fallback(): string {
  return 'Untitled';
}

async function execScript<T>(tabId: number, fn: (...args: any[]) => T, args: any[]): Promise<T> {
  if (typeof browser.scripting !== 'undefined') {
    const results = await browser.scripting.executeScript({
      target: { tabId },
      func: fn,
      args,
    });
    return (results[0] as any).result;
  } else {
    const fnStr = `(${fn.toString()})(${args.map((a) => JSON.stringify(a)).join(',')})`;
    const results = await browser.tabs.executeScript(tabId, { code: fnStr });
    return results[0] as T;
  }
}

function detectBrowser(): string {
  // In service workers, navigator.userAgent is available.
  if (typeof navigator === 'undefined') return 'unknown';
  const ua = navigator.userAgent;
  if (ua.includes('Firefox')) return 'firefox';
  if (ua.includes('Chrome')) return 'chrome';
  if (ua.includes('Edg/')) return 'edge';
  return 'chromium';
}

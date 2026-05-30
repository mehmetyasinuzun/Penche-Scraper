/**
 * Capture orchestrator.
 * Resolves the domain profile, extracts the page title,
 * takes a screenshot, and sends everything to the router.
 */

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

/** Entry point called by the background service worker on shortcut press. */
export async function runCapture(tabId: number): Promise<CaptureOutcome> {
  const cfg = await loadConfig();

  const tab = await browser.tabs.get(tabId);
  if (!tab.url) return { status: 'error', message: 'Tab has no URL' };

  let url: URL;
  try {
    url = new URL(tab.url);
  } catch {
    return { status: 'error', message: `Invalid URL: ${tab.url}` };
  }

  const match = resolveProfile(cfg, url.hostname, url.pathname);
  if (!match) {
    logger.info('no profile for domain', { host: url.hostname });
    return { status: 'no_profile', domain: url.hostname };
  }

  const { profileId, profile } = match;
  logger.info('profile matched', { profileId, host: url.hostname });

  const pageTitle = await extractTitle(tabId, profile.title);
  logger.info('title extracted', { pageTitle: pageTitle.slice(0, 80) });

  const screenshotCfg: ScreenshotConfig = {
    ...cfg.global.defaultScreenshot,
    ...(profile.screenshot ?? {}),
  };

  let screenshot: { mime: string; base64: string };
  try {
    screenshot = await takeScreenshot(tabId, screenshotCfg);
    logger.info('screenshot captured', { mode: screenshotCfg.mode });
  } catch (err) {
    logger.error('screenshot failed', { error: String(err) });
    return { status: 'error', message: `Screenshot failed: ${err}` };
  }

  const payload: RouterEventPayload = {
    event_id: crypto.randomUUID(),
    captured_at: new Date().toISOString(),
    domain: url.hostname,
    page_title: pageTitle,
    page_url: tab.url,
    screenshot,
    meta: {
      browser: detectBrowser(),
      profile_id: profileId,
      tags: profile.tags ?? [],
    },
  };

  const result = await sendEvent(cfg.global.router, payload);

  if (result.ok) return { status: 'success', eventId: result.eventId };

  if (result.retryable) {
    await outboxEnqueue(payload);
    return { status: 'queued', eventId: payload.event_id };
  }

  return { status: 'error', message: result.error };
}

// ─────────────────────────────────────────────────────────────────────────────

async function extractTitle(
  tabId: number,
  titleCfg: { primarySelector: string; fallbackSelectors?: string[] }
): Promise<string> {
  const selectors = [titleCfg.primarySelector, ...(titleCfg.fallbackSelectors ?? [])];

  const result = await execInTab<string>(tabId, (sels: unknown[]) => {
    for (const sel of sels as string[]) {
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
    const og = document.querySelector("meta[property='og:title']") as HTMLMetaElement | null;
    if (og?.content) return og.content.trim();
    return document.title.trim();
  }, [selectors]);

  return result || 'Untitled';
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
async function execInTab<T>(tabId: number, fn: (...args: any[]) => T, args: unknown[]): Promise<T> {
  if (typeof browser.scripting !== 'undefined') {
    const results = await browser.scripting.executeScript({
      target: { tabId },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      func: fn as (...args: unknown[]) => any,
      args,
    });
    return (results[0] as { result: T }).result;
  }
  const fnStr = `(${fn.toString()})(${args.map((a) => JSON.stringify(a)).join(',')})`;
  const results = await browser.tabs.executeScript(tabId, { code: fnStr });
  return results[0] as T;
}

function detectBrowser(): string {
  if (typeof navigator === 'undefined') return 'unknown';
  const ua = navigator.userAgent;
  if (ua.includes('Firefox')) return 'firefox';
  if (ua.includes('Edg/')) return 'edge';
  if (ua.includes('Chrome')) return 'chrome';
  return 'chromium';
}

/**
 * Screenshot engine.
 *
 * Modes:
 *   viewport   – single captureVisibleTab (fastest)
 *   full_page  – scroll + stitch via OffscreenCanvas
 *   top_px     – stitch only down to N pixels from the top
 *   element    – crop to a DOM element bounding box
 */

import { ScreenshotConfig } from '../shared/types';
import { logger } from '../shared/logger';

export interface ScreenshotResult {
  mime: string;
  base64: string;
}

type ImageFormat = 'jpeg' | 'png' | 'webp';

/** Take a screenshot according to the given config. */
export async function takeScreenshot(
  tabId: number,
  cfg: ScreenshotConfig
): Promise<ScreenshotResult> {
  const fmt: ImageFormat = cfg.imageType ?? 'jpeg';
  const quality = cfg.imageQuality ?? 0.82;

  switch (cfg.mode) {
    case 'viewport':
      return captureViewport(fmt, quality);

    case 'full_page':
      return captureTopPx(tabId, cfg.maxFullPageHeightPx ?? 16000, fmt, quality);

    case 'top_px':
      return captureTopPx(tabId, cfg.topPx ?? 2200, fmt, quality);

    case 'element':
      if (cfg.elementSelector) {
        return captureElement(tabId, cfg.elementSelector, fmt, quality).catch((err) => {
          logger.warn('element capture failed, falling back to viewport', { error: String(err) });
          return captureViewport(fmt, quality);
        });
      }
      return captureViewport(fmt, quality);

    default:
      return captureViewport(fmt, quality);
  }
}

// ─────────────────────────────────────────────────────────────────────────────

async function captureViewport(fmt: ImageFormat, quality: number): Promise<ScreenshotResult> {
  const dataUrl = await browser.tabs.captureVisibleTab(
    // Firefox requires windowId; passing undefined uses the current window.
    undefined as unknown as number,
    { format: fmt === 'png' ? 'png' : 'jpeg', quality: Math.round(quality * 100) }
  );
  return dataUrlToResult(dataUrl, fmt);
}

async function captureTopPx(
  tabId: number,
  targetPx: number,
  fmt: ImageFormat,
  quality: number
): Promise<ScreenshotResult> {
  // Retrieve page metrics from the page context.
  const metrics = await execInTab<{
    scrollX: number;
    scrollY: number;
    viewportH: number;
    pageH: number;
    dpr: number;
  }>(tabId, () => ({
    scrollX: window.scrollX,
    scrollY: window.scrollY,
    viewportH: window.innerHeight,
    pageH: Math.max(document.body.scrollHeight, document.documentElement.scrollHeight),
    dpr: window.devicePixelRatio || 1,
  }), []);

  const viewportH = metrics.viewportH;
  const dpr = metrics.dpr;
  const captureH = Math.min(metrics.pageH, targetPx);

  // Reset to top before starting.
  await scrollTab(tabId, 0, 0);
  await delay(80);

  const strips: string[] = [];
  let capturedPx = 0;

  while (capturedPx < captureH) {
    const strip = await browser.tabs.captureVisibleTab(
      undefined as unknown as number,
      { format: fmt === 'png' ? 'png' : 'jpeg', quality: Math.round(quality * 100) }
    );
    strips.push(strip);
    capturedPx += viewportH;

    if (capturedPx < captureH) {
      await scrollTab(tabId, 0, capturedPx);
      await delay(120);
    }
  }

  // Restore original scroll position.
  await scrollTab(tabId, metrics.scrollX, metrics.scrollY);

  if (strips.length === 1) {
    const cropped = await cropTopPx(strips[0], targetPx * dpr, fmt, quality);
    return dataUrlToResult(cropped, fmt);
  }

  const stitched = await stitchStrips(strips, captureH, viewportH, dpr, fmt, quality);
  return dataUrlToResult(stitched, fmt);
}

async function captureElement(
  tabId: number,
  selector: string,
  fmt: ImageFormat,
  quality: number
): Promise<ScreenshotResult> {
  const rect = await execInTab<{
    top: number;
    left: number;
    width: number;
    height: number;
    dpr: number;
  } | null>(tabId, (sel: string) => {
    const el = document.querySelector(sel);
    if (!el) return null;
    el.scrollIntoView({ block: 'nearest' });
    const r = el.getBoundingClientRect();
    return { top: r.top, left: r.left, width: r.width, height: r.height, dpr: window.devicePixelRatio || 1 };
  }, [selector]);

  if (!rect) throw new Error(`element not found: ${selector}`);

  const dataUrl = await browser.tabs.captureVisibleTab(
    undefined as unknown as number,
    { format: fmt === 'png' ? 'png' : 'jpeg', quality: Math.round(quality * 100) }
  );

  const cropped = await cropRect(
    dataUrl,
    rect.left * rect.dpr, rect.top * rect.dpr,
    rect.width * rect.dpr, rect.height * rect.dpr,
    fmt, quality
  );
  return dataUrlToResult(cropped, fmt);
}

// ─────────────────────────────────────────────────────────────────────────────
// Canvas helpers (OffscreenCanvas — available in service workers)
// ─────────────────────────────────────────────────────────────────────────────

async function stitchStrips(
  strips: string[],
  totalH: number,
  viewportH: number,
  dpr: number,
  fmt: ImageFormat,
  quality: number
): Promise<string> {
  // viewportH is used indirectly via totalH/strips.length; keep param for clarity.
  void viewportH;
  const images = await Promise.all(strips.map(loadImageBitmap));
  const w = images[0].width;
  const h = Math.min(Math.round(totalH * dpr), images.reduce((s, img) => s + img.height, 0));

  const canvas = new OffscreenCanvas(w, h);
  const ctx = canvas.getContext('2d')!;

  let y = 0;
  for (const img of images) {
    const remaining = h - y;
    if (remaining <= 0) break;
    const drawH = Math.min(img.height, remaining);
    ctx.drawImage(img, 0, 0, w, drawH, 0, y, w, drawH);
    y += drawH;
  }

  const blob = await canvas.convertToBlob({ type: `image/${fmt}`, quality });
  return blobToDataUrl(blob);
}

async function cropTopPx(
  dataUrl: string,
  heightPx: number,
  fmt: ImageFormat,
  quality: number
): Promise<string> {
  const img = await loadImageBitmap(dataUrl);
  const h = Math.min(Math.round(heightPx), img.height);
  const canvas = new OffscreenCanvas(img.width, h);
  canvas.getContext('2d')!.drawImage(img, 0, 0);
  const blob = await canvas.convertToBlob({ type: `image/${fmt}`, quality });
  return blobToDataUrl(blob);
}

async function cropRect(
  dataUrl: string,
  x: number, y: number, w: number, h: number,
  fmt: ImageFormat,
  quality: number
): Promise<string> {
  const img = await loadImageBitmap(dataUrl);
  const cw = Math.round(w), ch = Math.round(h);
  const canvas = new OffscreenCanvas(cw, ch);
  canvas.getContext('2d')!.drawImage(img, Math.round(x), Math.round(y), cw, ch, 0, 0, cw, ch);
  const blob = await canvas.convertToBlob({ type: `image/${fmt}`, quality });
  return blobToDataUrl(blob);
}

function loadImageBitmap(dataUrl: string): Promise<ImageBitmap> {
  return fetch(dataUrl).then((r) => r.blob()).then((b) => createImageBitmap(b));
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

function dataUrlToResult(dataUrl: string, fmt: ImageFormat): ScreenshotResult {
  return {
    mime: `image/${fmt}`,
    base64: dataUrl.split(',')[1] ?? '',
  };
}

async function scrollTab(tabId: number, x: number, y: number): Promise<void> {
  await execInTab<void>(tabId, (sx: number, sy: number) => { window.scrollTo(sx, sy); }, [x, y]);
}

/** Execute a function in the tab context, returning its result. */
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
  // MV2 fallback
  const fnStr = `(${fn.toString()})(${args.map((a) => JSON.stringify(a)).join(',')})`;
  const results = await browser.tabs.executeScript(tabId, { code: fnStr });
  return results[0] as T;
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

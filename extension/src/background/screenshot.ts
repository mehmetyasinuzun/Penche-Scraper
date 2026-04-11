/**
 * Screenshot engine.
 *
 * Modes:
 *   viewport   - single captureVisibleTab (fastest)
 *   full_page  - scroll + stitch via Offscreen canvas
 *   top_px     - stitch only down to N pixels from the top
 *   element    - crop to a DOM element bounding box
 *
 * All stitching happens inside an Offscreen document to avoid
 * blocking the service worker.
 */

import { ScreenshotConfig, ImageType } from '../shared/types';
import { logger } from '../shared/logger';

export interface ScreenshotResult {
  mime: string;
  base64: string;
}

/** Take a screenshot according to the given config. */
export async function takeScreenshot(
  tabId: number,
  cfg: ScreenshotConfig
): Promise<ScreenshotResult> {
  const imageType = cfg.imageType ?? 'jpeg';
  const quality = cfg.imageQuality ?? 0.82;
  const mime = `image/${imageType}`;

  switch (cfg.mode) {
    case 'viewport':
      return captureViewport(tabId, imageType, quality);

    case 'full_page':
      return captureFullPage(tabId, cfg.maxFullPageHeightPx ?? 16000, imageType, quality);

    case 'top_px':
      return captureTopPx(tabId, cfg.topPx ?? 2200, cfg.maxFullPageHeightPx ?? 16000, imageType, quality);

    case 'element':
      if (cfg.elementSelector) {
        return captureElement(tabId, cfg.elementSelector, imageType, quality).catch((err) => {
          logger.warn('element capture failed, falling back to viewport', { error: String(err) });
          return captureViewport(tabId, imageType, quality);
        });
      }
      return captureViewport(tabId, imageType, quality);

    default:
      return captureViewport(tabId, imageType, quality);
  }
}

// ────────────────────────────────────────────────────────────────────────────

async function captureViewport(
  tabId: number,
  imageType: ImageType,
  quality: number
): Promise<ScreenshotResult> {
  const dataUrl = await browser.tabs.captureVisibleTab(undefined as unknown as number, {
    format: imageType === 'png' ? 'png' : 'jpeg',
    quality: Math.round(quality * 100),
  });
  return dataUrlToResult(dataUrl, imageType);
}

async function captureFullPage(
  tabId: number,
  maxHeight: number,
  imageType: ImageType,
  quality: number
): Promise<ScreenshotResult> {
  return captureTopPx(tabId, maxHeight, maxHeight, imageType, quality);
}

async function captureTopPx(
  tabId: number,
  targetPx: number,
  maxPx: number,
  imageType: ImageType,
  quality: number
): Promise<ScreenshotResult> {
  // Get page metrics from the content script.
  const [metricsResult] = await browser.tabs.executeScript
    ? // MV2 path
      await browser.tabs.executeScript(tabId, {
        code: `({
          scrollX: window.scrollX,
          scrollY: window.scrollY,
          viewportH: window.innerHeight,
          pageH: Math.max(document.body.scrollHeight, document.documentElement.scrollHeight),
          devicePR: window.devicePixelRatio || 1
        })`,
      })
    : // MV3 path
      await browser.scripting.executeScript({
        target: { tabId },
        func: () => ({
          scrollX: window.scrollX,
          scrollY: window.scrollY,
          viewportH: window.innerHeight,
          pageH: Math.max(document.body.scrollHeight, document.documentElement.scrollHeight),
          devicePR: window.devicePixelRatio || 1,
        }),
      });

  const metrics = (metricsResult as any)?.result ?? metricsResult;
  const viewportH: number = metrics.viewportH;
  const pageH: number = Math.min(metrics.pageH, Math.min(targetPx, maxPx));
  const dpr: number = metrics.devicePR;

  // Save original scroll position.
  await scrollTab(tabId, 0, 0);
  await delay(80);

  const strips: string[] = [];
  let capturedPx = 0;

  while (capturedPx < pageH) {
    const strip = await browser.tabs.captureVisibleTab(undefined as unknown as number, {
      format: imageType === 'png' ? 'png' : 'jpeg',
      quality: Math.round(quality * 100),
    });
    strips.push(strip);
    capturedPx += viewportH;

    if (capturedPx < pageH) {
      await scrollTab(tabId, 0, capturedPx);
      await delay(120);
    }
  }

  // Restore scroll.
  await scrollTab(tabId, metrics.scrollX, metrics.scrollY);

  if (strips.length === 1) {
    // Nothing to stitch — crop if needed.
    const dataUrl = await cropTopPx(strips[0], targetPx * dpr, imageType, quality);
    return dataUrlToResult(dataUrl, imageType);
  }

  const stitched = await stitchStrips(strips, pageH, viewportH, dpr, imageType, quality);
  return dataUrlToResult(stitched, imageType);
}

async function captureElement(
  tabId: number,
  selector: string,
  imageType: ImageType,
  quality: number
): Promise<ScreenshotResult> {
  const execResult = await execInTab(tabId, (sel: string) => {
    const el = document.querySelector(sel);
    if (!el) return null;
    el.scrollIntoView({ block: 'nearest' });
    const rect = el.getBoundingClientRect();
    return {
      top: rect.top,
      left: rect.left,
      width: rect.width,
      height: rect.height,
      dpr: window.devicePixelRatio || 1,
    };
  }, [selector]);

  if (!execResult) throw new Error(`element not found: ${selector}`);

  const dataUrl = await browser.tabs.captureVisibleTab(undefined as unknown as number, {
    format: imageType === 'png' ? 'png' : 'jpeg',
    quality: Math.round(quality * 100),
  });

  const cropped = await cropRect(
    dataUrl,
    execResult.left * execResult.dpr,
    execResult.top * execResult.dpr,
    execResult.width * execResult.dpr,
    execResult.height * execResult.dpr,
    imageType,
    quality
  );
  return dataUrlToResult(cropped, imageType);
}

// ────────────────────────────────────────────────────────────────────────────
// Canvas helpers — run in service worker context using OffscreenCanvas.
// ────────────────────────────────────────────────────────────────────────────

async function stitchStrips(
  strips: string[],
  totalH: number,
  viewportH: number,
  dpr: number,
  imageType: ImageType,
  quality: number
): Promise<string> {
  const images = await Promise.all(strips.map(loadImage));
  const w = images[0].width;
  const h = Math.min(Math.round(totalH * dpr), images.reduce((s, img) => s + img.height, 0));

  const canvas = new OffscreenCanvas(w, h);
  const ctx = canvas.getContext('2d')!;

  let y = 0;
  for (let i = 0; i < images.length; i++) {
    const remaining = h - y;
    const drawH = Math.min(images[i].height, remaining);
    ctx.drawImage(images[i], 0, 0, w, drawH, 0, y, w, drawH);
    y += drawH;
    if (y >= h) break;
  }

  const blob = await canvas.convertToBlob({
    type: `image/${imageType}`,
    quality: quality,
  });
  return blobToDataUrl(blob, imageType);
}

async function cropTopPx(
  dataUrl: string,
  heightPx: number,
  imageType: ImageType,
  quality: number
): Promise<string> {
  const img = await loadImage(dataUrl);
  const h = Math.min(Math.round(heightPx), img.height);
  const canvas = new OffscreenCanvas(img.width, h);
  const ctx = canvas.getContext('2d')!;
  ctx.drawImage(img, 0, 0);
  const blob = await canvas.convertToBlob({ type: `image/${imageType}`, quality });
  return blobToDataUrl(blob, imageType);
}

async function cropRect(
  dataUrl: string,
  x: number,
  y: number,
  w: number,
  h: number,
  imageType: ImageType,
  quality: number
): Promise<string> {
  const img = await loadImage(dataUrl);
  const canvas = new OffscreenCanvas(Math.round(w), Math.round(h));
  const ctx = canvas.getContext('2d')!;
  ctx.drawImage(img, Math.round(x), Math.round(y), Math.round(w), Math.round(h), 0, 0, Math.round(w), Math.round(h));
  const blob = await canvas.convertToBlob({ type: `image/${imageType}`, quality });
  return blobToDataUrl(blob, imageType);
}

function loadImage(dataUrl: string): Promise<ImageBitmap> {
  return fetch(dataUrl)
    .then((r) => r.blob())
    .then((b) => createImageBitmap(b));
}

function blobToDataUrl(blob: Blob, imageType: ImageType): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

function dataUrlToResult(dataUrl: string, imageType: ImageType): ScreenshotResult {
  const base64 = dataUrl.split(',')[1] ?? '';
  return { mime: `image/${imageType}`, base64 };
}

async function scrollTab(tabId: number, x: number, y: number): Promise<void> {
  await execInTab(tabId, (sx: number, sy: number) => window.scrollTo(sx, sy), [x, y]);
}

async function execInTab<T>(tabId: number, fn: (...args: any[]) => T, args: any[]): Promise<T> {
  if (typeof browser.scripting !== 'undefined') {
    // MV3
    const results = await browser.scripting.executeScript({
      target: { tabId },
      func: fn,
      args,
    });
    return (results[0] as any).result;
  } else {
    // MV2 fallback
    const fnStr = `(${fn.toString()})(${args.map((a) => JSON.stringify(a)).join(',')})`;
    const results = await browser.tabs.executeScript(tabId, { code: fnStr });
    return results[0] as T;
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

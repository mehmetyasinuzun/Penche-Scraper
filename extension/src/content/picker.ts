/**
 * Element picker overlay.
 * Injected into the page; user hovers to highlight elements, clicks to select.
 * Generates a stable CSS selector and reports it back to the options page.
 */

export interface PickerResult {
  selector: string;
  previewText: string;
}

let highlightEl: HTMLElement | null = null;
let resolvePromise: ((r: PickerResult) => void) | null = null;
let rejectPromise: (() => void) | null = null;

/** Activate the element picker. Returns a promise that resolves on selection. */
export function startPicker(): Promise<PickerResult> {
  cleanup();
  return new Promise<PickerResult>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
    createOverlay();
    document.addEventListener('mouseover', onMouseOver, true);
    document.addEventListener('click', onClick, true);
    document.addEventListener('keydown', onKeyDown, true);
  });
}

/** Programmatically cancel the picker. */
export function cancelPicker(): void {
  rejectPromise?.();
  cleanup();
}

// ─────────────────────────────────────────────────────────────────────────────

function createOverlay(): void {
  highlightEl = document.createElement('div');
  highlightEl.id = '__penche_highlight__';
  Object.assign(highlightEl.style, {
    position: 'fixed',
    border: '2px solid #ff4444',
    background: 'rgba(255,68,68,0.1)',
    borderRadius: '3px',
    pointerEvents: 'none',
    zIndex: '2147483647',
    transition: 'all 0.05s ease',
    display: 'none',
    boxSizing: 'border-box',
  });

  const badge = document.createElement('div');
  badge.id = '__penche_badge__';
  badge.textContent = 'Penche: öğeye tıkla · ESC iptal';
  Object.assign(badge.style, {
    position: 'fixed',
    top: '12px',
    left: '50%',
    transform: 'translateX(-50%)',
    background: '#1a1a2e',
    color: '#eee',
    padding: '6px 16px',
    borderRadius: '20px',
    fontSize: '13px',
    fontFamily: 'monospace',
    zIndex: '2147483647',
    pointerEvents: 'none',
    boxShadow: '0 2px 12px rgba(0,0,0,0.5)',
    whiteSpace: 'nowrap',
  });

  document.body.appendChild(highlightEl);
  document.body.appendChild(badge);
}

function onMouseOver(e: MouseEvent): void {
  const target = e.target as Element;
  if (!target || isPickerEl(target)) return;
  const rect = target.getBoundingClientRect();
  if (highlightEl) {
    Object.assign(highlightEl.style, {
      display: 'block',
      top: `${rect.top}px`,
      left: `${rect.left}px`,
      width: `${rect.width}px`,
      height: `${rect.height}px`,
    });
  }
}

function onClick(e: MouseEvent): void {
  e.preventDefault();
  e.stopPropagation();
  e.stopImmediatePropagation();
  const target = e.target as Element;
  if (!target || isPickerEl(target)) return;
  resolvePromise?.({ selector: buildSelector(target), previewText: extractText(target) });
  cleanup();
}

function onKeyDown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault();
    rejectPromise?.();
    cleanup();
  }
}

function cleanup(): void {
  document.removeEventListener('mouseover', onMouseOver, true);
  document.removeEventListener('click', onClick, true);
  document.removeEventListener('keydown', onKeyDown, true);
  document.getElementById('__penche_highlight__')?.remove();
  document.getElementById('__penche_badge__')?.remove();
  highlightEl = null;
  resolvePromise = null;
  rejectPromise = null;
}

function isPickerEl(el: Element): boolean {
  return el.id === '__penche_highlight__' || el.id === '__penche_badge__';
}

// ─────────────────────────────────────────────────────────────────────────────
// Selector generation
// ─────────────────────────────────────────────────────────────────────────────

function buildSelector(el: Element): string {
  // 1. Stable ID
  if (el.id && isStableId(el.id)) {
    const s = `#${CSS.escape(el.id)}`;
    if (isUnique(s)) return s;
  }
  // 2. Tag + stable class combination
  const cls = buildClassSelector(el);
  if (cls && isUnique(cls)) return cls;
  // 3. Structural path (max 4 levels)
  return buildStructural(el, 4);
}

function buildClassSelector(el: Element): string | null {
  const tag = el.tagName.toLowerCase();
  const stable = Array.from(el.classList).filter(isStableClass);
  if (!stable.length) return null;
  return `${tag}.${stable.map(CSS.escape).join('.')}`;
}

function buildStructural(el: Element, maxDepth: number): string {
  const parts: string[] = [];
  let cur: Element | null = el;
  let depth = 0;
  while (cur && cur !== document.body && depth < maxDepth) {
    const tag = cur.tagName.toLowerCase();
    const parent = cur.parentElement;
    if (parent) {
      const siblings = Array.from(parent.children).filter((c) => c.tagName === cur!.tagName);
      parts.unshift(siblings.length > 1 ? `${tag}:nth-of-type(${siblings.indexOf(cur) + 1})` : tag);
    } else {
      parts.unshift(tag);
    }
    cur = cur.parentElement;
    depth++;
  }
  return parts.join(' > ');
}

function isUnique(sel: string): boolean {
  try { return document.querySelectorAll(sel).length === 1; } catch { return false; }
}

function isStableId(id: string): boolean {
  return !/^\d|[a-f0-9]{8,}/.test(id);
}

function isStableClass(cls: string): boolean {
  if (cls.length > 60 || /[a-f0-9]{8,}/.test(cls)) return false;
  return /^[a-z_-][a-z0-9_-]*$/i.test(cls);
}

function extractText(el: Element): string {
  return ((el as HTMLElement).innerText ?? el.textContent ?? '').trim().slice(0, 200);
}

/**
 * Element picker overlay injected into the page.
 * User hovers to highlight elements, clicks to select.
 * Generates a robust CSS selector and reports back to options page.
 */

export interface PickerResult {
  selector: string;
  previewText: string;
}

let overlayEl: HTMLElement | null = null;
let highlightEl: HTMLElement | null = null;
let currentTarget: Element | null = null;
let resolvePromise: ((r: PickerResult) => void) | null = null;
let rejectPromise: ((r: void) => void) | null = null;

/** Start the element picker. Returns a promise that resolves on selection. */
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
  if (rejectPromise) rejectPromise();
  cleanup();
}

// ────────────────────────────────────────────────────────────────────────────

function createOverlay(): void {
  overlayEl = document.createElement('div');
  overlayEl.id = '__penche_picker_overlay__';
  Object.assign(overlayEl.style, {
    position: 'fixed',
    top: '0',
    left: '0',
    width: '100vw',
    height: '100vh',
    zIndex: '2147483646',
    cursor: 'crosshair',
    pointerEvents: 'none',
  });

  highlightEl = document.createElement('div');
  highlightEl.id = '__penche_picker_highlight__';
  Object.assign(highlightEl.style, {
    position: 'fixed',
    border: '2px solid #ff4444',
    background: 'rgba(255,68,68,0.12)',
    borderRadius: '3px',
    pointerEvents: 'none',
    zIndex: '2147483647',
    transition: 'all 0.05s ease',
    display: 'none',
    boxSizing: 'border-box',
  });

  const badge = document.createElement('div');
  badge.id = '__penche_picker_badge__';
  badge.textContent = 'Penche: Click to select | Esc to cancel';
  Object.assign(badge.style, {
    position: 'fixed',
    top: '10px',
    left: '50%',
    transform: 'translateX(-50%)',
    background: '#1a1a2e',
    color: '#eee',
    padding: '6px 14px',
    borderRadius: '6px',
    fontSize: '13px',
    fontFamily: 'monospace',
    zIndex: '2147483647',
    pointerEvents: 'none',
    boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
  });

  document.body.appendChild(overlayEl);
  document.body.appendChild(highlightEl);
  document.body.appendChild(badge);
}

function onMouseOver(e: MouseEvent): void {
  const target = e.target as Element;
  if (!target || isPickerElement(target)) return;

  currentTarget = target;
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
  if (!target || isPickerElement(target)) return;

  const selector = buildSelector(target);
  const previewText = extractText(target);

  if (resolvePromise) {
    resolvePromise({ selector, previewText });
  }
  cleanup();
}

function onKeyDown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault();
    if (rejectPromise) rejectPromise();
    cleanup();
  }
}

function cleanup(): void {
  document.removeEventListener('mouseover', onMouseOver, true);
  document.removeEventListener('click', onClick, true);
  document.removeEventListener('keydown', onKeyDown, true);

  ['__penche_picker_overlay__', '__penche_picker_highlight__', '__penche_picker_badge__'].forEach(
    (id) => document.getElementById(id)?.remove()
  );

  overlayEl = null;
  highlightEl = null;
  currentTarget = null;
  resolvePromise = null;
  rejectPromise = null;
}

function isPickerElement(el: Element): boolean {
  return (
    el.id === '__penche_picker_overlay__' ||
    el.id === '__penche_picker_highlight__' ||
    el.id === '__penche_picker_badge__'
  );
}

// ────────────────────────────────────────────────────────────────────────────
// Selector generation — prefers stable class-based selectors.
// ────────────────────────────────────────────────────────────────────────────

function buildSelector(el: Element): string {
  // Strategy 1: element has a useful id.
  if (el.id && isStableId(el.id)) {
    const sel = `#${CSS.escape(el.id)}`;
    if (isUnique(sel)) return sel;
  }

  // Strategy 2: tag + class combination.
  const classSelector = buildClassSelector(el);
  if (classSelector && isUnique(classSelector)) return classSelector;

  // Strategy 3: structural path (limited depth).
  return buildStructuralSelector(el, 4);
}

function buildClassSelector(el: Element): string | null {
  const tag = el.tagName.toLowerCase();
  const stable = Array.from(el.classList).filter(isStableClass);
  if (stable.length === 0) return null;
  return `${tag}.${stable.map(CSS.escape).join('.')}`;
}

function buildStructuralSelector(el: Element, maxDepth: number): string {
  const parts: string[] = [];
  let current: Element | null = el;
  let depth = 0;

  while (current && current !== document.body && depth < maxDepth) {
    const tag = current.tagName.toLowerCase();
    const parent = current.parentElement;

    if (parent) {
      const siblings = Array.from(parent.children).filter((c) => c.tagName === current!.tagName);
      if (siblings.length > 1) {
        const idx = siblings.indexOf(current) + 1;
        parts.unshift(`${tag}:nth-of-type(${idx})`);
      } else {
        parts.unshift(tag);
      }
    } else {
      parts.unshift(tag);
    }

    current = current.parentElement;
    depth++;
  }

  return parts.join(' > ');
}

function isUnique(selector: string): boolean {
  try {
    return document.querySelectorAll(selector).length === 1;
  } catch {
    return false;
  }
}

function isStableId(id: string): boolean {
  // Avoid auto-generated IDs that look random.
  return !/^\d|[a-f0-9]{8,}/.test(id);
}

function isStableClass(cls: string): boolean {
  // Filter out utility classes with random hashes.
  if (cls.length > 60) return false;
  if (/[a-f0-9]{8,}/.test(cls)) return false;
  // Keep semantic class names.
  return /^[a-z_-][a-z0-9_-]*$/i.test(cls);
}

function extractText(el: Element): string {
  const text = (el as HTMLElement).innerText ?? el.textContent ?? '';
  return text.trim().slice(0, 200);
}

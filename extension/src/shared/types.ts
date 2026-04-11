// ────────────────────────────────────────────────────────────────────────────
// Shared types used across background, content, popup, and options.
// ────────────────────────────────────────────────────────────────────────────

export type ScreenshotMode = 'viewport' | 'full_page' | 'top_px' | 'element';
export type ImageType = 'jpeg' | 'png' | 'webp';

export interface ScreenshotConfig {
  mode: ScreenshotMode;
  /** Used when mode === 'top_px' */
  topPx?: number;
  /** Used when mode === 'element' */
  elementSelector?: string;
  /** Safety cap for full_page stitching */
  maxFullPageHeightPx?: number;
  imageType?: ImageType;
  /** 0.0–1.0 */
  imageQuality?: number;
}

export interface TitleConfig {
  primarySelector: string;
  fallbackSelectors?: string[];
}

export interface DomainMatch {
  host: string;
  /** Optional path regex string */
  pathRegex?: string;
}

export interface DomainProfile {
  enabled: boolean;
  match: DomainMatch;
  title: TitleConfig;
  screenshot?: Partial<ScreenshotConfig>;
  tags?: string[];
}

export interface RouterConfig {
  baseUrl: string;
  timeoutMs: number;
  sharedSecret: string;
}

export interface GlobalConfig {
  shortcut: string;
  defaultScreenshot: ScreenshotConfig;
  router: RouterConfig;
}

export interface AppConfig {
  version: number;
  global: GlobalConfig;
  domains: Record<string, DomainProfile>;
}

// ────────────────────────────────────────────────────────────────────────────
// Message types passed between extension contexts.
// ────────────────────────────────────────────────────────────────────────────

export type MessageType =
  | 'CAPTURE_REQUEST'
  | 'CAPTURE_RESULT'
  | 'PICK_ELEMENT_START'
  | 'PICK_ELEMENT_RESULT'
  | 'PICK_ELEMENT_CANCEL'
  | 'TOAST'
  | 'PROFILE_SAVE'
  | 'PROFILE_DELETE'
  | 'OUTBOX_STATUS';

export interface BaseMessage {
  type: MessageType;
}

export interface CaptureRequestMessage extends BaseMessage {
  type: 'CAPTURE_REQUEST';
}

export interface CaptureResultMessage extends BaseMessage {
  type: 'CAPTURE_RESULT';
  success: boolean;
  eventId?: string;
  error?: string;
}

export interface PickElementStartMessage extends BaseMessage {
  type: 'PICK_ELEMENT_START';
  /** Tab ID to inject into */
  tabId: number;
  /** The domain we're configuring */
  domain: string;
}

export interface PickElementResultMessage extends BaseMessage {
  type: 'PICK_ELEMENT_RESULT';
  domain: string;
  selector: string;
  previewText: string;
}

export interface PickElementCancelMessage extends BaseMessage {
  type: 'PICK_ELEMENT_CANCEL';
}

export interface ToastMessage extends BaseMessage {
  type: 'TOAST';
  level: 'success' | 'warning' | 'error' | 'info';
  text: string;
}

export interface ProfileSaveMessage extends BaseMessage {
  type: 'PROFILE_SAVE';
  domain: string;
  profile: DomainProfile;
}

export interface ProfileDeleteMessage extends BaseMessage {
  type: 'PROFILE_DELETE';
  domain: string;
}

export interface OutboxStatusMessage extends BaseMessage {
  type: 'OUTBOX_STATUS';
  pendingCount: number;
}

export type ExtMessage =
  | CaptureRequestMessage
  | CaptureResultMessage
  | PickElementStartMessage
  | PickElementResultMessage
  | PickElementCancelMessage
  | ToastMessage
  | ProfileSaveMessage
  | ProfileDeleteMessage
  | OutboxStatusMessage;

// ────────────────────────────────────────────────────────────────────────────
// Event payload sent to the Router API.
// ────────────────────────────────────────────────────────────────────────────

export interface ScreenshotPayload {
  mime: string;
  /** Base64 encoded image data */
  base64: string;
}

export interface EventMeta {
  browser: string;
  profile_id: string;
  tags: string[];
}

export interface RouterEventPayload {
  event_id: string;
  captured_at: string; // ISO8601
  domain: string;
  page_title: string;
  page_url: string;
  screenshot: ScreenshotPayload;
  meta: EventMeta;
}

// ────────────────────────────────────────────────────────────────────────────
// Outbox item stored in browser.storage.local during router unavailability.
// ────────────────────────────────────────────────────────────────────────────

export interface OutboxItem {
  id: string;
  payload: RouterEventPayload;
  createdAt: number; // unix ms
  attemptCount: number;
  nextRetryAt: number; // unix ms
}

// Default application configuration factory.
export function defaultConfig(): AppConfig {
  return {
    version: 1,
    global: {
      shortcut: 'Ctrl+Shift+X',
      defaultScreenshot: {
        mode: 'top_px',
        topPx: 2200,
        maxFullPageHeightPx: 16000,
        imageType: 'jpeg',
        imageQuality: 0.82,
      },
      router: {
        baseUrl: 'http://127.0.0.1:8787',
        timeoutMs: 7000,
        sharedSecret: 'CHANGE_ME',
      },
    },
    domains: {
      'xss.is': {
        enabled: true,
        match: {
          host: 'xss.is',
          pathRegex: '/threads?/.*',
        },
        title: {
          primarySelector: 'h1.p-title-value',
          fallbackSelectors: ['h1', "meta[property='og:title']"],
        },
        screenshot: {
          mode: 'top_px',
          topPx: 2600,
        },
        tags: ['cti', 'forum', 'xss'],
      },
    },
  };
}

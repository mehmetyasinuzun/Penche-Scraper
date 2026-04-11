/**
 * HTTP client that POSTs events to the local Go Router API.
 * Handles HMAC signing and error classification.
 */

import { RouterEventPayload, RouterConfig } from '../shared/types';
import { buildAuthHeaders } from '../shared/hmac';
import { logger } from '../shared/logger';

export type SendResult =
  | { ok: true; eventId: string }
  | { ok: false; retryable: boolean; error: string };

export async function sendEvent(
  cfg: RouterConfig,
  payload: RouterEventPayload
): Promise<SendResult> {
  const body = JSON.stringify(payload);
  let headers: Record<string, string>;

  try {
    headers = await buildAuthHeaders(cfg.sharedSecret, body);
  } catch (err) {
    return { ok: false, retryable: false, error: `HMAC signing failed: ${err}` };
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), cfg.timeoutMs);

  try {
    const resp = await fetch(`${cfg.baseUrl}/v1/events`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...headers,
      },
      body,
      signal: controller.signal,
    });
    clearTimeout(timeoutId);

    if (resp.status === 200 || resp.status === 202) {
      const data = await resp.json();
      // 'duplicate' status is still a success — already exists.
      logger.info('router: event accepted', { event_id: payload.event_id, status: data.status });
      return { ok: true, eventId: payload.event_id };
    }

    const errorBody = await resp.text().catch(() => '');
    const retryable = resp.status >= 500;
    logger.warn('router: non-success response', {
      status: resp.status,
      event_id: payload.event_id,
      body: errorBody.slice(0, 200),
    });
    return {
      ok: false,
      retryable,
      error: `HTTP ${resp.status}: ${errorBody.slice(0, 200)}`,
    };
  } catch (err: any) {
    clearTimeout(timeoutId);
    const isNetwork = err?.name === 'AbortError' || err?.name === 'TypeError';
    logger.warn('router: send failed', { event_id: payload.event_id, error: String(err) });
    return {
      ok: false,
      retryable: isNetwork,
      error: String(err),
    };
  }
}

/** Simple health check — returns true if router is reachable. */
export async function checkHealth(cfg: RouterConfig): Promise<boolean> {
  try {
    const resp = await fetch(`${cfg.baseUrl}/v1/health`, {
      signal: AbortSignal.timeout(3000),
    });
    return resp.ok;
  } catch {
    return false;
  }
}

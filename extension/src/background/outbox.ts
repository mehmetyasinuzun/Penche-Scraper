/**
 * Outbox — a persistent queue in browser.storage.local.
 * Events are stored here when the Router API is unreachable.
 * A background retry loop drains the queue.
 */

import { OutboxItem, RouterEventPayload } from '../shared/types';
import { logger } from '../shared/logger';

const STORAGE_KEY = 'penche_outbox';
const MAX_ATTEMPTS = 10;
const BASE_BACKOFF_MS = 5_000;
const MAX_BACKOFF_MS = 5 * 60_000; // 5 min

export async function outboxEnqueue(payload: RouterEventPayload): Promise<void> {
  const items = await loadOutbox();
  const item: OutboxItem = {
    id: payload.event_id,
    payload,
    createdAt: Date.now(),
    attemptCount: 0,
    nextRetryAt: Date.now(),
  };
  items.push(item);
  await saveOutbox(items);
  logger.info('outbox: enqueued', { event_id: payload.event_id, queue_size: items.length });
}

export async function outboxDequeue(id: string): Promise<void> {
  const items = await loadOutbox();
  const filtered = items.filter((i) => i.id !== id);
  await saveOutbox(filtered);
}

export async function outboxGetDue(): Promise<OutboxItem[]> {
  const items = await loadOutbox();
  const now = Date.now();
  return items.filter((i) => i.attemptCount < MAX_ATTEMPTS && i.nextRetryAt <= now);
}

export async function outboxRecordAttemptFailure(id: string): Promise<void> {
  const items = await loadOutbox();
  const item = items.find((i) => i.id === id);
  if (!item) return;

  item.attemptCount += 1;
  item.nextRetryAt = Date.now() + backoffMs(item.attemptCount);

  if (item.attemptCount >= MAX_ATTEMPTS) {
    logger.warn('outbox: item reached max attempts, removing', { event_id: id });
    await saveOutbox(items.filter((i) => i.id !== id));
    return;
  }
  await saveOutbox(items);
}

export async function outboxCount(): Promise<number> {
  const items = await loadOutbox();
  return items.length;
}

async function loadOutbox(): Promise<OutboxItem[]> {
  const result = await browser.storage.local.get(STORAGE_KEY);
  return (result[STORAGE_KEY] as OutboxItem[]) ?? [];
}

async function saveOutbox(items: OutboxItem[]): Promise<void> {
  await browser.storage.local.set({ [STORAGE_KEY]: items });
}

function backoffMs(attempt: number): number {
  const delay = BASE_BACKOFF_MS * Math.pow(2, attempt - 1);
  return Math.min(delay, MAX_BACKOFF_MS);
}

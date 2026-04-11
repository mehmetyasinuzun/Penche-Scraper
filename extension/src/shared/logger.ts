/** Structured logger for the extension. Never logs screenshot base64. */

type Level = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
  ts: string;
  level: Level;
  msg: string;
  [key: string]: unknown;
}

function log(level: Level, msg: string, ctx?: Record<string, unknown>): void {
  const entry: LogEntry = {
    ts: new Date().toISOString(),
    level,
    msg,
    ...sanitize(ctx),
  };
  const fn = level === 'error' ? console.error : level === 'warn' ? console.warn : console.log;
  fn('[penche]', JSON.stringify(entry));
}

/** Strip screenshot base64 from log context to avoid data leakage. */
function sanitize(ctx?: Record<string, unknown>): Record<string, unknown> {
  if (!ctx) return {};
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(ctx)) {
    if (k === 'base64' || k === 'screenshot_data') {
      out[k] = '[REDACTED]';
    } else if (typeof v === 'string' && v.length > 2000) {
      out[k] = v.slice(0, 80) + '…[truncated]';
    } else {
      out[k] = v;
    }
  }
  return out;
}

export const logger = {
  debug: (msg: string, ctx?: Record<string, unknown>) => log('debug', msg, ctx),
  info: (msg: string, ctx?: Record<string, unknown>) => log('info', msg, ctx),
  warn: (msg: string, ctx?: Record<string, unknown>) => log('warn', msg, ctx),
  error: (msg: string, ctx?: Record<string, unknown>) => log('error', msg, ctx),
};

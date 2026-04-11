/**
 * HMAC-SHA256 signing for the extension → Router API requests.
 * Uses the Web Crypto API available in both MV3 service workers and content scripts.
 */

const encoder = new TextEncoder();

async function importKey(secret: string): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    'raw',
    encoder.encode(secret),
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  );
}

/** Compute HMAC-SHA256 over `timestamp + "." + body` and return hex string. */
export async function sign(secret: string, timestampUnix: number, body: string): Promise<string> {
  const key = await importKey(secret);
  const message = `${timestampUnix}.${body}`;
  const signature = await crypto.subtle.sign('HMAC', key, encoder.encode(message));
  return bufferToHex(signature);
}

function bufferToHex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

/** Returns headers needed for authenticated Router API requests. */
export async function buildAuthHeaders(
  secret: string,
  body: string
): Promise<Record<string, string>> {
  const ts = Math.floor(Date.now() / 1000);
  const sig = await sign(secret, ts, body);
  return {
    'X-Penche-Timestamp': String(ts),
    'X-Penche-Signature': sig,
  };
}

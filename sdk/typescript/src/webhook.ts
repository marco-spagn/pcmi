import { createHmac, timingSafeEqual } from 'node:crypto';

export const DEFAULT_MAX_AGE_SECS = 300;
export const CLOCK_SKEW_SECS = 60;

export function signWebhook(secret: string, timestamp: string, body: Buffer | Uint8Array): string {
  const mac = createHmac('sha256', secret);
  mac.update(timestamp);
  mac.update('.');
  mac.update(body);
  return `sha256=${mac.digest('hex')}`;
}

export function verifySignature(
  secret: string,
  signature: string,
  timestamp: string,
  body: Buffer | Uint8Array,
  options?: { now?: number; maxAgeSecs?: number },
): boolean {
  if (!secret || !signature || !timestamp) {
    return false;
  }
  if (!signature.startsWith('sha256=')) {
    return false;
  }
  const ts = Number.parseInt(timestamp, 10);
  if (!Number.isFinite(ts)) {
    return false;
  }
  const now = options?.now ?? Date.now() / 1000;
  const maxAge = options?.maxAgeSecs ?? DEFAULT_MAX_AGE_SECS;
  const age = now - ts;
  if (age > maxAge || ts - now > CLOCK_SKEW_SECS) {
    return false;
  }
  const expected = signWebhook(secret, timestamp, body);
  try {
    const got = Buffer.from(signature.slice(7), 'hex');
    const want = Buffer.from(expected.slice(7), 'hex');
    if (got.length !== want.length) {
      return false;
    }
    return timingSafeEqual(got, want);
  } catch {
    return false;
  }
}

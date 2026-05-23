"""Webhook signature verification for PCMI HTTP deliveries."""

from __future__ import annotations

import hashlib
import hmac
import time
from typing import Union

DEFAULT_MAX_AGE_SECS = 300
CLOCK_SKEW_SECS = 60


def verify_signature(
    secret: str,
    signature: str,
    timestamp: str,
    body: bytes,
    *,
    now: float | None = None,
    max_age_secs: int = DEFAULT_MAX_AGE_SECS,
) -> bool:
    """Verify X-PCMI-Signature for a webhook POST body.

    Signature format: sha256={hex(HMAC-SHA256(secret, timestamp + "." + body))}
    """
    if not secret or not signature or not timestamp:
        return False
    if not signature.startswith("sha256="):
        return False
    try:
        ts = int(timestamp)
    except ValueError:
        return False
    now_ts = time.time() if now is None else now
    age = now_ts - ts
    if age > max_age_secs or ts - now_ts > CLOCK_SKEW_SECS:
        return False
    expected = _sign(secret, timestamp, body)
    got_hex = signature[7:]
    try:
        got = bytes.fromhex(got_hex)
        want = bytes.fromhex(expected[7:])
    except ValueError:
        return False
    return hmac.compare_digest(got, want)


def _sign(secret: str, timestamp: str, body: bytes) -> str:
    msg = timestamp.encode() + b"." + body
    digest = hmac.new(secret.encode(), msg, hashlib.sha256).hexdigest()
    return f"sha256={digest}"

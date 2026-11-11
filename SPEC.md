# Project: Claude Code Local Router

## 1. Overview
A lightweight, local-first API gateway written in Go. It acts as a transparent proxy between **Claude Code** and multiple Anthropic-compatible upstream providers. 

The primary goal is to eliminate rate-limit interruptions by pooling API keys, automatically handling `429 Too Many Requests` errors, and failing over to secondary providers based on a tiered priority system. Claude Code only ever communicates with this local router, which handles the complexity of credential swapping and model mapping.

## 2. Core Architecture
```text
[ Claude Code ] 
      │  (Anthropic API format, custom router token)
      ▼
┌──────────────────────────────────────────────┐
│ Local Router (127.0.0.1:8787)                │
│                                              │
│  1. Auth Check (x-api-key or Bearer)         │
│  2. Model Override (e.g., map Claude->Flint) │
│  3. Key Selection (Tiered + Round Robin)     │
│  4. Stream Passthrough (Unbuffered)          │
│  5. Health Tracking (Cooldown on 429)        │
└──────────────────────────────────────────────┘
      │
      ├──▶ OpenRouter (Tier 0, Key Pool)
      ├──▶ TokenHarbor (Tier 1, Key Pool)
      └──▶ Other Providers (Tier 2, Key Pool)
```

## 3. Key Features
*   **Native Anthropic Passthrough:** No schema translation. The router accepts Anthropic Messages format and streams it back exactly as received. 
*   **Zero-Buffer Streaming:** To prevent Claude Code from stalling, the proxy writes and flushes chunks to the client immediately. 
*   **Tiered Failover:** If all keys in Tier 0 are exhausted or cooling down, the router automatically falls back to Tier 1, then Tier 2.
*   **Model Override:** Allows using Claude Code with non-Claude models (e.g., OpenRouter's `recursive/flint-v1.0:free`). The router intercepts the model name from Claude Code and swaps it before sending upstream.
*   **Intelligent Cooldowns:** Reads `Retry-After` headers or applies per-provider defaults to temporarily disable keys that hit rate limits.
*   **Comment-Supporting Config:** Uses `.jsonc` so providers and keys can be annotated with notes (e.g., "Key expires in 30 days").

## 4. API Contract

### `POST /v1/messages`
The primary inference endpoint.
*   **Auth:** Requires the custom router token passed via `x-api-key: <ROUTER_TOKEN>` or `Authorization: Bearer <ROUTER_TOKEN>`.
*   **Headers Forwarded:** `anthropic-version`, `anthropic-beta`, `content-type`.
*   **Response:** Standard Anthropic SSE (Server-Sent Events) stream.
*   **Failover Window:** The router will only retry another provider/key if the failure happens *before* the first byte of the response is sent to the client.

### `POST /v1/messages/count_tokens`
Optional endpoint. If implemented, the router proxies this to the highest-priority available provider. If omitted, Claude Code falls back to character-based estimation.

### `HEAD /api/hello`
Claude Code's connection warmup probe. The router should return `200 OK` to keep logs clean.

## 5. Configuration Schema (`config.jsonc`)

```jsonc
{
  "listen": "127.0.0.1:8787",
  "default_cooldown_seconds": 60,
  "providers": [
    {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api",
      "auth_style": "bearer", // "bearer" or "x-api-key"
      "tier": 0,
      "override_model": "recursive/flint-v1.0:free",
      "extra_headers": {
        "HTTP-Referer": "http://localhost:8787",
        "X-Title": "Claude Local Router"
      },
      "keys": ["sk-or-v1-1", "sk-or-v1-2"]
    },
    {
      "name": "tokenharbor",
      "base_url": "https://api.tokenharbor.com",
      "auth_style": "x-api-key",
      "tier": 1,
      "cooldown_seconds": 120,
      "keys": ["th-abc123"]
    }
  ]
}
```

## 6. Routing & Health Logic

### Selection Algorithm
On every request, the router builds a list of `Candidate Keys`:
1.  Filter out any keys marked as `disabled` (e.g., 401 Unauthorized).
2.  Filter out any keys where `CooldownUntil > time.Now()`.
3.  Sort the remaining keys by `Provider.Tier` (ascending), then by `LastUsed` (ascending, to ensure round-robin).

### Failure Handling
When iterating through candidates, if a request fails, the router evaluates the HTTP status:
*   **`429 Too Many Requests`:** Set key status to `cooling`. Set `CooldownUntil` based on the `Retry-After` header, or fallback to the provider's default cooldown. Proceed to the next key.
*   **`401 Unauthorized / 403 Forbidden`:** Set key status to `disabled`. Log a warning (key requires human intervention). Proceed to the next key.
*   **`500+ Server Errors / Timeouts`:** Increment a failure counter. If failures exceed 3, set to `cooling`. Proceed to the next key.
*   **`400 Bad Request`:** Do *not* failover. Return the error immediately to Claude Code, as the payload itself is malformed.

## 7. Constraints & Non-Goals

*   **Localhost Only:** The router must bind exclusively to `127.0.0.1`. It holds sensitive upstream API keys and should never be exposed to the public internet. The `ROUTER_TOKEN` acts only as a local authorization check.
*   **No Translation:** This router explicitly does *not* translate between OpenAI and Anthropic formats. Upstreams must natively support `/v1/messages`.
*   **No OAuth:** The router does not handle OAuth refresh flows (e.g., logging in with a Claude Max subscription). It is designed for static API keys.

## 8. Tech Stack
*   **Language:** Go (Standard library `net/http` and `httputil`).
*   **Config Parsing:** `github.com/tidwall/jsonc` for stripping comments from `.jsonc` files.
*   **State:** In-memory key pool state (thread-safe via `sync.Mutex`).

---

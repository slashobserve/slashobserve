---
title: Webhooks
description: Send workspace activity to an outside address as signed HTTPS requests that retry in order, set up by an Admin, with what is sent, how to verify the signature and how deliveries fail and recover.
---

A webhook posts the events of the [activity log](activity.md) to an address you choose, so another system can react to a dashboard save, a failed agent run or a mention without polling.
Each event becomes one signed HTTPS request.
A request that fails is retried, and a webhook never skips an event or sends a later one first.

Only an Admin can create, change, test or delete a webhook, through the HTTP API below or the [`observe webhooks`](cli.md#webhooks) commands.
A service-account key with the Admin role can do the same.
No MCP tool or agent tool reads or changes a webhook, because a webhook sends workspace activity to an outside address and that is an Admin's decision.

## What is sent and to whom

A webhook has a name, an address, the event types it wants and an owner, the Admin who created it or the one an Admin moved it to.
It sends an event only when its owner could read that event in the activity log at the moment of sending, by the rules in [Who sees an event](activity.md#who-sees-an-event).
So an event in a folder the owner cannot view is passed over, and an event in a person's personal folder reaches only a webhook that person owns.
When the owner is deleted or stops being an Admin, the webhook is disabled with the reason `owner`.

A webhook starts at the end of the log: it sends events that happen after it was created, not older ones.
Use `GET /api/v1/activity` to read what came before.

The server keeps at most 20 webhooks.
The 21st answers `422 too_many_webhooks`.

## Before you start

The server seals each signing secret with `SLASHOBSERVE_SECRET_KEY`, so creating a webhook without it answers `409 secret_key_missing`; see [Configuration](../reference/configuration.md#stored-secrets).
Changing that key makes every stored secret unreadable: each webhook is disabled with the reason `secret_unreadable`, and you rotate its secret and enable it again.

The address must be `https` on port 443 with a domain name, with no user name, password or fragment, and at most 2048 bytes.
The request goes through the same [outbound access](../operate/outbound-access.md) rules as every other call the server makes: a public address only, and the domain must be on the allowlist.
A webhook never adds its domain to the allowlist.
If the domain is not allowed, creating the webhook answers `403 outbound_domain_not_allowed`, and an Admin has to allow the domain first.
If an Admin removes the domain later, the attempts fail with `domain_not_allowed` and the webhook retries until the domain is back.

The receiver must answer within 10 seconds, and a request body is never larger than 64 KiB.

## Set up a webhook in Settings

Admins see a Webhooks section in Settings; nobody else does.

1. Open Settings, then Webhooks.
2. Under Add a webhook, enter a name, the https address and the events to send.
   The events are grouped as Dashboards, Annotations, Findings, Saved searches, Observations, Proposals, Comment threads and Agent runs, and a group's box chooses all of its events.
3. Select Add webhook.
   If the address's domain is not on the outbound allowlist, the form says so and links to the Outbound access section, where an Admin can allow it.
4. Copy the signing secret that appears, then select I have copied it.
   The page keeps the secret only until then, and the server cannot show it again.

Each webhook in the table shows its owner, address, events and status: `Sending`, `Retrying` after failed attempts, or `Disabled` with the reason.
Over HTTP the same answer is the webhook's `state`, `sending`, `retrying` or `disabled`; see [States](../reference/states.md).
Test sends a `webhook.test` event and reports the outcome, status and duration.
Attempts lists the recent attempts of that webhook.
The menu next to them edits the webhook, enables or disables it, rotates its secret and deletes it, and rotating and deleting ask for confirmation first.
Saving an edit or rotating the secret makes you the webhook's owner.

## Manage webhooks over HTTP

All of these need the Admin role.

| Request | What it does |
| --- | --- |
| `GET /api/v1/webhooks` | Lists the webhooks. |
| `POST /api/v1/webhooks` | Creates one from `name`, `url` and `event_types`, and returns it with its `secret`. |
| `PATCH /api/v1/webhooks/{id}` | Changes `name`, `url`, `event_types` or `enabled`. |
| `DELETE /api/v1/webhooks/{id}` | Deletes the webhook and its attempts. |
| `POST /api/v1/webhooks/{id}/secret` | Makes a new signing secret and returns it. |
| `POST /api/v1/webhooks/{id}/test` | Sends one `webhook.test` event now and returns the attempt. |
| `GET /api/v1/webhooks/{id}/deliveries` | Lists the recent attempts, newest first, with `limit` and `cursor`. |

```sh
curl -fsS -X POST -H "Authorization: Bearer $OBSERVE_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Failed runs","url":"https://hooks.example.com/slashobserve","event_types":["run.failed"]}' \
  "$OBSERVE_SERVER/api/v1/webhooks"
```

The signing secret, `whsec_` and 43 characters, appears in this response and in the one from `/secret`, and nowhere else.
Copy it then; the server cannot show it again, and the way to get a new one is to rotate it.
Rotating takes effect at once: the next request is signed with the new secret and the old one stops working.

`PATCH` with `enabled: false` disables the webhook with the reason `manual`.
`PATCH` with `enabled: true` turns it on again, clears the failure count and makes it send at once.
Changing the address keeps the webhook's place in the log, clears the failure count and sends at once, so it carries on from the event it had reached.
Whoever changes a webhook, with a `PATCH` or by rotating its secret, becomes its owner, so from then on it sends what that Admin can read.

`event_types` takes the type names from [What is recorded](activity.md#what-is-recorded), at least one and each at most once.

## The request

Each request is a `POST` with a JSON body and three headers:

| Header | Value |
| --- | --- |
| `X-SlashObserve-Event` | The event type, such as `dashboard.version_created`, or `webhook.test`. |
| `X-SlashObserve-Delivery` | The delivery id, the same for every attempt at one event. |
| `X-SlashObserve-Signature` | `t=<unix seconds>,v1=<hex>`, described below. |

The body has the delivery id, the webhook id, the type and the event as the activity API returns it:

```json
{
  "delivery_id": "6c1f0f1e-6a53-4c8e-9c63-2f4d5f0b8a11.412",
  "webhook_id": "6c1f0f1e-6a53-4c8e-9c63-2f4d5f0b8a11",
  "type": "dashboard.version_created",
  "event": {
    "seq": 412,
    "type": "dashboard.version_created",
    "target": {"kind": "dashboard", "id": "3d2e9a5c-1b0f-4c7a-8d41-0e6f2b9a7c10"},
    "dashboard_id": "3d2e9a5c-1b0f-4c7a-8d41-0e6f2b9a7c10",
    "folder_id": "b7a4d0e2-5c19-4f6a-9d3e-8a21c4f7e605",
    "actor": {"kind": "user", "id": "9f1c2d3e-4a5b-4c6d-8e7f-0a1b2c3d4e5f", "name": "Ada", "origin": null, "client": null, "run_id": null, "label": "Ada"},
    "data": {"version": 12, "parent_version": 11, "title": "Checkout", "summary": "Add a latency panel"},
    "created_at": "2026-11-11T09:30:00Z"
  }
}
```

`event` follows the rules of [What data holds](activity.md#what-data-holds): ids, numbers, titles and enums, never a prompt, a comment or a query.
A `webhook.test` body has `"event": null`.
Treat unknown fields as allowed, because a later version may add some: the `run.succeeded` event of a health repair that saved a version carries `dashboard_title` and `panels_repaired` in `data` beside the fields every run event has.

An event whose body would pass 64 KiB is never sent.
It is recorded as a `skipped` attempt with the error `too_large`, and the webhook moves on to the next event.

## Verify the signature

Check every request before you trust it.
The signature is an HMAC-SHA256, keyed with the signing secret, of the timestamp `t`, a dot, and the raw request body.
The header carries `t` and the lowercase hex of the result as `v1`.

1. Read the raw body before you parse it.
2. Split the header at the commas and read `t` and `v1`.
3. Reject the request when `t` is more than 300 seconds from your clock, which stops a recorded request from being replayed later.
4. Compute the HMAC over `t`, `.` and the body, and compare it with `v1` in constant time.

For the secret `whsec_test`, the timestamp `1700000000` and the body `{"ok":true}`, the signature is `85876387ad9d6be57a04653bc0729da757049f58afb10ba6cac3bedaecf4fda3`.
Use that to check your code.

A Go receiver, using only the standard library:

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func verify(secret, header string, body []byte, now time.Time) bool {
	var stamp, sum string
	for _, part := range strings.Split(header, ",") {
		key, value, _ := strings.Cut(part, "=")
		switch key {
		case "t":
			stamp = value
		case "v1":
			sum = value
		}
	}
	sent, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || now.Sub(time.Unix(sent, 0)).Abs() > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "."))
	mac.Write(body)
	want, err := hex.DecodeString(sum)
	return err == nil && hmac.Equal(mac.Sum(nil), want)
}

func handler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil || !verify(secret, r.Header.Get("X-SlashObserve-Signature"), body, time.Now()) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

The same check in Python:

```python
import hashlib
import hmac
import time


def verify(secret, header, body, now=None):
    parts = dict(p.split("=", 1) for p in header.split(","))
    stamp = int(parts["t"])
    if abs((now or time.time()) - stamp) > 300:
        return False
    signed = str(stamp).encode() + b"." + body
    expected = hmac.new(secret.encode(), signed, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, parts["v1"])
```

## Delivery rules

- **At least once.**
  A request can reach you twice, for example when your answer was lost on the way back.
  Keep the delivery ids you handled and ignore a repeat.
- **In order for one webhook.**
  A webhook sends its next event only after the previous one was delivered, so a failing event holds back the events behind it.
  Different webhooks do not wait for each other.
- **`seq` is not the order.**
  Events are ordered by their position in the log, which is not always the order of `seq`; see [Order and resuming](activity.md#order-and-resuming).
  Rely on the order requests arrive in, not on `seq` growing.
- **A `2xx` answer delivers the event.**
  Any other answer, a timeout, a refused connection or a blocked address is a failed attempt.
- **Retries back off.**
  The wait before attempt `n + 1` is `min(5 seconds * 2^(n-1), 15 minutes)`, so 5 seconds, 10, 20, and 15 minutes from the ninth failure on.
  When the receiver answers `429` or `503` with a `Retry-After` header, the server waits that long instead, at least 5 seconds and at most 1 hour.
- **An event expires after 24 hours.**
  An event that is more than 24 hours old when its turn comes is passed over, so a webhook that was down for days does not replay all of it and one bad event cannot hold the webhook forever.
- **Some answers stop the webhook.**
  A `410 Gone` disables it with the reason `gone`, and 50 failed attempts in a row disable it with `failing`.
  The server does not follow a redirect, so a `3xx` is a failed attempt.

A disabled webhook sends nothing.
Its `disabled_reason` says why: `manual`, `gone`, `failing`, `owner` or `secret_unreadable`.
Fix the cause and `PATCH` it with `enabled: true`; it continues from the event it stopped at.
For `secret_unreadable`, rotate the secret first.
Events that happened while it was disabled are still in the log, so after a long outage read `GET /api/v1/activity` with `since` to see what you may have missed.

## Attempts

Each attempt is kept for 7 days, with its time, duration, the receiver's status, an outcome (`delivered`, `failed` or `skipped`) and an error code: `status`, `unreachable`, `blocked_address`, `domain_not_allowed` or `too_large`.
The server never stores or logs the address's path, a header, the request body or what the receiver answered, only that status and code.
Read them with `GET /api/v1/webhooks/{id}/deliveries` or `observe webhooks deliveries`.

`POST /api/v1/webhooks/{id}/test` sends a `webhook.test` event signed like any other and returns its attempt without waiting for the log, so you can check an address and a signature check right after creating the webhook.

---
title: Outbound access
description: Control which domains the server may fetch from, including the feeds in news panels.
---

SlashObserve can show content that dashboards fetch from other sites, such as the feeds in news panels.
The server fetches it only from domains that are on the allowlist.
The list starts empty, so a new deployment makes no such request until someone allows a domain.
Datasource queries, the identity provider and the model provider are not part of this list.

An entry is `example.com`, which matches that host only, or `*.example.com`, which matches its subdomains but not `example.com` itself.
Write the domain without a scheme, port or path; an IP address is refused, and a name with non-ASCII letters is stored in its ASCII form.
At most 100 domains can be allowed.

Every fetch follows the same rules, whatever the domain:

- Only `https` on port 443.
- Only public addresses.
  The server checks the address it connects to, so a name that resolves to a private, loopback, link-local or cloud metadata address is refused.
- A redirect is followed up to 5 times, and each hop must be an allowed domain on `https`.
- No cookies, credentials or `Referer` are sent.
- Each fetch has a size cap (1 MiB unless the feature asks for more, 8 MiB at most), a 10 second time limit and a content type check.
- Results are cached for 15 minutes, and removing a domain stops its cached results from being served.

Any signed-in person can see the list; only an Admin can allow or remove a domain in Settings or through `/api/v1/outbound-domains`, and service-account keys cannot.
Every change is logged at INFO as `allowed an outbound domain` or `removed an outbound domain` with the person's id (`actor_id`) and the domain.
The table also keeps who added each entry and when, but a removed entry leaves only the log line.

To take the decision away from the Admins, set `SLASHOBSERVE_OUTBOUND_DOMAINS`.
`none` allows nothing.
A comma-separated list such as `example.com,*.example.org` allows exactly those domains.
The list is then read-only: Settings shows it, the API answers changes with `409 managed_by_config`, and the database table is neither read nor written.
The setting never seeds the Settings list; to switch back, unset it and restart, and the Settings list is what it was before.

## News feeds

News panels read their feed through the server, never from the browser, with `GET /api/v1/news?url=<feed address>`; only signed-in people can call it, not service-account keys.
The feed must be an RSS or Atom feed on an https address, up to 1 MiB, and its domain must be on the outbound allowlist.
The server caches a feed for 15 minutes and returns at most 50 items as plain text.
A feed whose domain is not allowed shows a message naming the domain.
Item links open in a new tab and are not limited by the allowlist: the allowlist controls what the server fetches, not where a person may click.

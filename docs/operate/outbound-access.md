---
title: Outbound access
description: Control which domains the server may fetch from or call, including the feeds in news panels, the images and GeoJSON files that panels show and the requests that canvas actions send.
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

## Images and GeoJSON

The browser never loads an image or a file from another site.
A page's Content-Security-Policy allows only the server's own origin, so panels that show a remote image, such as an image in a text panel's Markdown, ask the server for it with `GET /api/v1/assets/image?url=<image address>`.
Panels that draw a GeoJSON layer use `GET /api/v1/assets/geojson?url=<file address>`.
Geomap panels that draw map tiles load each tile through the image route, and they draw tiles only when every host the tile address can produce is allowed (CARTO uses `a.basemaps.cartocdn.com` to `d.basemaps.cartocdn.com`); otherwise the map shows country outlines and a note names the host to allow.
Only signed-in people can call these routes, not service-account keys, and the address must be an https address on an allowed domain.

To show a basemap, allow the host of its tiles:

| Basemap | Domain to allow |
| --- | --- |
| `carto` | `*.basemaps.cartocdn.com` |
| `esri` | `services.arcgisonline.com` |
| `osm` | `tile.openstreetmap.org` |
| `xyz` | The host in the panel's tile address. |

The OpenStreetMap servers are volunteer-run and limit heavy use; read their [tile usage policy](https://operations.osmfoundation.org/policies/tiles/) before you allow them.
For dashboards many people open, prefer CARTO or your own tile server (an `xyz` basemap).

- An image can be a PNG, JPEG, GIF, WebP, AVIF or SVG, up to 5 MiB, and its bytes must match the type the site declared.
  The server sends it with a policy that forbids scripts, so an SVG opened on its own never runs script, and the browser keeps it for 1 hour.
- A GeoJSON file can be served as `application/geo+json`, `application/json` or `text/plain`, up to 5 MiB, and must be a JSON object whose `type` is a GeoJSON type.
- Both are cached on the server for 1 hour, in a cache separate from the news feeds'.
- A domain that is not allowed answers `403 outbound_domain_not_allowed`, and the panel shows the image's alt text instead.
  A file that cannot be fetched, is too large or is not what it claims to be answers `502 asset_unavailable`.
- A site that redirects to a content network needs that network's domain on the allowlist too, because each redirect hop is checked.

Each refusal is logged at INFO as `asset fetch failed` with the host, the code and the person's id (`actor_id`), never the address's path or query.

## Canvas actions

A canvas element can carry actions, which are HTTP requests that a person runs from the panel (see [Maps, canvases and actions](../concepts/observespec.md#maps-canvases-and-actions)).
The browser never calls the other site.
It asks the server with `POST /api/v1/dashboards/{id}/actions`, and the server sends the request under the same rules as every other outbound call: an allowed domain, `https` on port 443 and a public address.

- Only a person with Edit on the dashboard can run an action, because it changes something outside SlashObserve.
  Viewers, Commenters and service-account keys are refused.
- The request names the action by its place in the dashboard's latest saved version (panel, element and index), plus the values of the variables, the time range and the clicked value.
  The method, address, headers and body template always come from the saved version, and a request made against an older version answers `409 stale_version`.
- `__user.login`, `__user.email`, `__user.name` and `__user.id` come from the signed-in person.
- A value is escaped for the place it lands in: the path, the query, a form body or a JSON string.
  An action that would change its own host, put a line break in a header or leave a JSON body invalid answers `400 action_invalid`.
- No credentials are sent, so an action can call only endpoints that need none.
  The server does not follow a redirect: a `3xx` is returned as the status.
- A request has a 10 second limit and a body of at most 64 KiB, and the response body is read up to 64 KiB and thrown away.
  The answer is `{"status": 204, "ok": true}`, where `ok` is true for a 2xx status.
- Each person can have 2 actions running at once (`429 too_many_actions`).
- Actions are never cached or merged, so running one twice sends two requests.
- A domain that is not allowed answers `403 outbound_domain_not_allowed` with the `host`, and a connection error, a timeout or a private address answers `502 action_failed` with the `host`.

Every attempt that reaches the network is logged at INFO as `canvas action run` with the person (`actor_id`, `actor_role`), `dashboard_id`, `version`, `panel`, `element`, `action`, `method`, `host`, `status` (0 when no answer came), `outcome` (`ok`, `remote_error`, `refused` or `failed`) and `duration_ms`.
The line never holds the address, a header, the body or a variable value.

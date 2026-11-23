---
title: API behavior
description: How the HTTP API treats list queries, unknown query parameters, rejected panels, caching and compression, for scripts, agents and proxies.
---

This page covers behaviors of `/api/v1` that are easy to trip over from a script.
Read it before you write a client that lists items, filters them or caches reads.

## Catch a misspelt parameter

A list route is a `GET` that returns a page of items.
It refuses a query parameter it does not document, so a misspelt filter never returns an unfiltered list.

1. Send a request with a parameter the route does not take:

   ```sh
   curl -sS -H "Authorization: Bearer $OBSERVE_TOKEN" \
     "$OBSERVE_SERVER/api/v1/dashboards?health=duplicate_title"
   ```

2. Read the `400` answer.
   It has the code `invalid_request` and names the parameters the route does take:

   ```json
   {"error": "unknown query parameter health; this route takes ...", "code": "invalid_request"}
   ```

3. Fix the name and send the request again.

A value the parameter does not accept is refused the same way.
`?order=oldest` answers `query parameter "order" must be one of updated, folder, views, name`.
A query string that cannot be parsed, such as an unescaped `;`, is refused as well.

## Detect an ignored parameter on other routes

Every other route reads the parameters it documents and ignores the rest.
So that you notice, the server names the ignored parameters in a response header:

```http
X-Unknown-Query-Parameters: health
```

The status and body do not change.
A script that must not act on an ignored parameter should fail when the header is present.

## Read every page of a list

1. Request the list with a `limit`:

   ```sh
   curl -sS -H "Authorization: Bearer $OBSERVE_TOKEN" \
     "$OBSERVE_SERVER/api/v1/activity?limit=50"
   ```

2. Read `next_cursor` from the answer.
   It is `null` on the last page.
3. Send the value back unchanged as `cursor` to read the next page.

The `observe` command line follows cursors with `--all`; see [the command line](../use/cli.md#lists-and-paging).

## Save a panel as a service account

A person's session may save a panel block that returns no data, after the web app warns.
A service-account key may not.
When a key adds or changes a panel block of an observation, the server runs the same panel check as the `add_block` and `update_block` MCP tools.
A rejected panel answers `422 invalid_spec` with `issues` that name the failing paths.

## Reference

### Query parameters

- A list route refuses an undocumented parameter, a value the parameter does not accept and an unparseable query string, with `400 invalid_request`.
- The refusal names up to 10 unknown parameters, separated by commas, and ends with `...` when more were unknown.
- Every other route ignores undocumented parameters and sets `X-Unknown-Query-Parameters` in every deployment.
  The header lists up to 10 names, separated by commas, and ends with `...` when more were ignored.
  A name that is not 1 to 64 letters, digits or `_.[]-` is left out of the list, and `...` ends the list.
- Parameters that start with `var-` are dashboard variable selections.
  The document does not list them by name, so the server never reports them.
- Routes that read a free-form query by design, such as the query proxy and the log tail, already answer `400 invalid_request` for a parameter they do not know.
- With `SLASHOBSERVE_API_CONTRACT_CHECK=true`, which is meant for test stacks, a request with an unknown parameter makes the response a `500 contract_violation`.
  An integration test of a client then finds the mistake before it ships.
  See [Configuration](configuration.md#testing-only).

### Paging

| Item | Rule |
| --- | --- |
| `limit` | An integer from 1 up to a maximum the route documents. |
| `cursor` | The `next_cursor` of the previous page, unchanged. |
| `next_cursor` | `null` on the last page. |
| A `limit` out of range | `400 invalid_request`. |
| A cursor the server does not recognize | `400 invalid_request`. |

### Caching

Every `/api/v1` response is `Cache-Control: no-store` unless this table says otherwise.

| Route | Header |
|---|---|
| `GET /api/v1/folders` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |
| `GET /api/v1/dashboards/{id}/versions/{version}` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |
| `GET /api/v1/assets/image` | `private, max-age=3600` |
| `GET /api/v1/assets/geojson` | `private, max-age=3600` |

### Details of `no-cache` and `ETag`

`no-cache` means a browser or proxy keeps a copy but asks the server before every use.
The server still checks who is asking and what they may see.
It answers `304 Not Modified` without a body when the `ETag` sent in `If-None-Match` is still current.
The tag is a weak hash of the body, so it changes when the dashboard, a folder or the caller's permissions on it change.

Query proxy data, such as `/dashboards/{id}/versions/{version}/panels/{panel_id}/data`, is never cached.
It comes from a live data source and depends on who asks.

### Compression

- A `GET` or `HEAD` response of 1024 bytes or more is gzip-compressed when the client sends `Accept-Encoding: gzip`.
- Event streams and responses to other methods are never compressed.

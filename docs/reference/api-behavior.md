---
title: API behavior
description: How the API pages lists, treats query parameters it does not know, refuses a rejected panel, caches and compresses reads, for scripts, agents and proxies.
---

This page covers behaviors of `/api/v1` that are easy to trip over from a script.

## Query parameters

A list route, which is a `GET` that returns a page of items, refuses a query parameter it does not document and a value its parameter does not accept.
`GET /api/v1/dashboards?health=duplicate_title` answers `400 invalid_request` with `unknown query parameter health; this route takes ...` and the parameters the route does take, and `?order=oldest` answers `query parameter "order" must be one of updated, folder, views, name`.
A query string that cannot be parsed, such as an unescaped `;`, is refused the same way.
A script never gets an unfiltered list because of a misspelt filter.

Every other route reads the parameters it documents and ignores the rest.
So that this does not go unnoticed, the server names the parameters it did not use in a response header:

```http
X-Unknown-Query-Parameters: health
```

The header lists up to 10 names, separated by commas, and ends with `...` when more were ignored.
It is sent on every route that does not refuse, in every deployment, and the status and body do not change.
A script that must not act on an ignored parameter should fail when the header is present.

Parameters that start with `var-` are dashboard variable selections.
They are not listed in the document by name, so the server never reports them.

Routes that read a free-form query by design, such as the query proxy and the log tail, already answer `400 invalid_request` for a parameter they do not know.

The server can also reject unknown parameters.
With `SLASHOBSERVE_API_CONTRACT_CHECK=true`, which is meant for test stacks, a request with an unknown parameter makes the response a `500 contract_violation`, so an integration test of a client finds the mistake before it ships.
See [Configuration](configuration.md#testing-only).

## Paging

A list route takes `limit`, an integer from 1 up to a maximum the route documents, and `cursor`.
A `limit` outside its range answers `400 invalid_request`.
The response carries `next_cursor`, which is `null` on the last page; pass it back as `cursor` unchanged to read the next page.
A cursor the server does not recognise answers `400 invalid_request`.
The `observe` command line follows cursors with `--all`, see [the command line](../use/cli.md#lists-and-paging).

## Rejected panels

A person's session may save a panel block that returns no data, after the web app warns.
A service-account key may not: when it adds or changes a panel block of an observation, the server runs the same panel check as the `add_block` and `update_block` MCP tools, and a rejected panel answers `422 invalid_spec` with `issues` that name the failing paths.

## Caching

Every `/api/v1` response is `Cache-Control: no-store` unless this page says otherwise.
Four reads are the exception:

| Route | Header |
|---|---|
| `GET /api/v1/folders` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |
| `GET /api/v1/dashboards/{id}/versions/{version}` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |
| `GET /api/v1/assets/image` | `private, max-age=3600` |
| `GET /api/v1/assets/geojson` | `private, max-age=3600` |

`no-cache` means a browser or proxy keeps a copy but asks the server before every use.
The server still checks who is asking and what they may see, then answers `304 Not Modified` without a body when the `ETag` sent in `If-None-Match` is still current.
The tag is a hash of the body, so it changes when the dashboard, a folder or the caller's permissions on it change.

Query proxy data, such as `/dashboards/{id}/versions/{version}/panels/{panel_id}/data`, is never cached, because it comes from a live data source and depends on who asks.

## Compression

A `GET` or `HEAD` response of 1024 bytes or more is gzip-compressed when the client sends `Accept-Encoding: gzip`.
Event streams and responses to other methods are never compressed.

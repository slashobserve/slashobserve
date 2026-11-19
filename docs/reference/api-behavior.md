---
title: API behavior
description: How the API treats query parameters it does not know and which reads can be cached, for scripts, agents and proxies.
---

This page covers two behaviors of `/api/v1` that are easy to trip over from a script.

## Query parameters

The API reads the query parameters its routes document and ignores any other.
A misspelt filter such as `?health=duplicate_title` for `?problem=duplicate_title` is therefore not an error, and the list comes back unfiltered.

So that this does not go unnoticed, the server names the parameters it did not use in a response header:

```http
X-Unknown-Query-Parameters: health
```

The header lists up to 10 names, separated by commas, and ends with `...` when more were ignored.
It is sent on every route, in every deployment, and the status and body do not change.
A script that must not act on an unfiltered list should fail when the header is present.

Parameters that start with `var-` are dashboard variable selections.
They are not listed in the document by name, so the server never reports them.

Routes that read a free-form query by design, such as the query proxy and the log tail, already answer `400 invalid_request` for a parameter they do not know.

The server can also reject unknown parameters.
With `SLASHOBSERVE_API_CONTRACT_CHECK=true`, which is meant for test stacks, a request with an unknown parameter makes the response a `500 contract_violation`, so an integration test of a client finds the mistake before it ships.
See [Configuration](configuration.md#testing-only).

## Caching

Every `/api/v1` response is `Cache-Control: no-store` unless this page says otherwise.
Two reads are the exception, because they are large and rarely change:

| Route | Header |
|---|---|
| `GET /api/v1/folders` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |
| `GET /api/v1/dashboards/{id}/versions/{version}` | `private, no-cache`, `ETag`, `Vary: Authorization, Cookie` |

`no-cache` means a browser or proxy keeps a copy but asks the server before every use.
The server still checks who is asking and what they may see, then answers `304 Not Modified` without a body when the `ETag` sent in `If-None-Match` is still current.
The tag is a hash of the body, so it changes when the dashboard, a folder or the caller's permissions on it change.

Query proxy data, such as `/dashboards/{id}/versions/{version}/panels/{panel_id}/data`, is never cached, because it comes from a live data source and depends on who asks.

---
title: Search
description: Find dashboards, findings, annotations, comments and saved searches by their words, ranked by relevance, showing each person only what they can open.
---

Search finds past work by its words.
One query covers dashboards, findings, annotations, comments and saved logs and trace searches, and ranks the matches together.
You call it with `GET /api/v1/search`, which needs the Viewer role or higher and works with a session or a service-account key.

## What each kind matches

| Kind | Title and tags | Text | Query text |
| --- | --- | --- | --- |
| `dashboard` | Title and tags. | Description, and the titles and descriptions of panels, rows included. | The query of every panel, rows included. |
| `finding` | Title. | The text of the finding, and the titles and descriptions of its evidence panels. | The queries of its evidence panels. |
| `annotation` | The first line of its text, and its tags. | The text. | None. |
| `comment` | None. | One message, with each mention read as `@Name`. | None. |
| `saved_search` | Title. | The service and span filters of a trace search. | The query of a logs search. |

Only the latest version of a dashboard is searched, so a word that was removed in a later save finds nothing.
A comment is one message, not a whole thread, and a result for it names the object it is on.
Folders, text panel content and prompts are not searched.
A match in a title or a tag counts for more than a match in the text, and a match in query text counts for less.
Query text is searched as plain words, so `http_requests_total` is found by `http_requests_total` and by `requests`.

## Who finds what

A result appears only when you can open the object it points to.
A dashboard is found through its folder, and so are its annotations, findings and comments.
A finding filed in a folder and a saved search are found through that folder.
A global annotation, one on no dashboard, and its comments are found by everyone who can read General.
A comment is found by whoever can read what it is on.
An unfiled finding, one on no dashboard and in no folder, is found only by the person or key that saved it and by Admins.
Personal folders stay private, Admins included.
A result never shows a title or a snippet of something you cannot open, and a move, a delete or an edit changes the results at once.

## Writing a query

Every word has to match, either exactly or by its English stem, so `errors` finds `error`.
Case and punctuation do not matter.

| You write | It finds |
| --- | --- |
| `checkout latency` | Objects with both words. |
| `"checkout latency"` | Objects with the two words next to each other, in that order. |
| `checkout or payments` | Objects with either word. |
| `checkout -staging` | Objects with `checkout` and without `staging`. |
| `chec` | Objects with a word that starts with `chec`. |

The last word also matches as a prefix, so a search box can find results while you type.
A query that ends in a space, an open quote or a `-` term has no prefix.
A query is 1 to 200 characters with at least one letter or digit outside a `-` term, and holds no control characters.

## Ranking and limits

Results are ranked by how well the words match, and among near-equals the newer object comes first.
A title match always beats a text match of the same frequency.
A query returns at most 200 results, so refine the words to see more.

## Search over HTTP

`GET /api/v1/search` takes these query parameters.

| Parameter | Meaning |
| --- | --- |
| `q` | The words to search for. Required. |
| `kind` | Only results of this kind: `dashboard`, `finding`, `annotation`, `comment` or `saved_search`. Repeat it for several. |
| `limit` | 1 to 50 results, 20 by default. |
| `cursor` | The `next_cursor` of the previous page. |

The answer is `{"results": [...], "next_cursor": ...}`, and `next_cursor` is null on the last page.
Each result has these fields.

| Field | Meaning |
| --- | --- |
| `kind` | One of the five kinds. |
| `id` | The id of the object, or of the message for a comment. |
| `title` | The title of the object, or for a comment, of what it is on. |
| `snippet` | Up to two fragments of the text around the matched words, as `{"text", "match"}` segments of at most 300 characters in all. A segment with `match` set is a matched word. Render it as text, never as HTML. |
| `url` | The page that shows the object, or null for an annotation on no dashboard. |
| `folder` | The `id` and `title` of the folder the result is read through, or null for an unfiled finding. |
| `target` | For a comment, the `kind`, `id` and `title` of what it is on, else null. |
| `thread_id` | For a comment, its thread, else null. |
| `updated_at` | When the object last changed, or for a comment, when it was written or last edited. |

Recency is measured once for a whole search, so paging does not reorder results as time passes, but an edit made while you page can still move a result.
A malformed query or cursor answers 400 `invalid_request` with the reason.

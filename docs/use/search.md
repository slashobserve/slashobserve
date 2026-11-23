---
title: Search
description: Find dashboards, observations, annotations, comments and saved searches by their words, ranked by relevance, showing each person only what they can open.
---

Search finds past work by its words.
One query covers dashboards, annotations, observations, comments, and saved logs, trace and metrics searches, and ranks the matches together.
Use it to find the dashboard that had a panel on `checkout`, the observation that answered a latency question, or the comment where someone explained a spike.

You see only what you can open.
[Who finds what](#who-finds-what) has the rules.

## Find something

1. Press Cmd+K (Ctrl+K on Windows and Linux), or choose **Search or jump to** in the sidebar.
2. Type at least two characters, such as `checkout latency`.
   The palette lists matching pages, dashboards and sources, then adds what the server finds in the workspace under a heading for each kind, with the matched words marked.
3. Press Enter to open a result.
   A comment opens the page of what it is on.
4. Choose **See all results**, or open `/search?q=checkout`, to see every match on the Search page, 20 at a time with **Show more**.
   One button per kind narrows the list.

[Search in the web app](web.md#search) describes the palette and the page.
From a terminal, run:

```sh
observe search checkout latency --kind observation
```

You see one line per result with its kind, title, folder and page, and a snippet of the matched text under it:

```text
observation  Checkout latency after the 14:00 deploy  Payments  https://observe.example.com/o/6c1f...
    ... p99 latency on checkout rose after ...
```

`observe search` prints `No results.` when nothing matches.

## Write a query

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
A prefix needs at least two letters or digits.
A query that ends in a space, an open quote or a `-` term has no prefix.

A hyphen between two words searches both parts, so `demo-shop overview` finds a title written `Demo Shop Overview` or `demo-shop overview`.
A hyphen before a digit, as in `node-1`, matches a title written the same way.
A hyphen at the start of a word still excludes it, and `-demo-shop` excludes the words `demo shop` written next to each other, whether the title has a hyphen or a space.

A query is 1 to 200 characters and holds no control characters.
It needs at least one word that is not a `-` term, so `-staging` alone answers 400.

## Search only one kind of object

Pass the kind you want, and repeat it for several.
In the web app, the buttons on the Search page do this.
Over HTTP, add `kind` once for each kind:

```sh
curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
  "$OBSERVE_SERVER/api/v1/search?q=checkout&kind=dashboard&kind=observation&limit=10"
```

The answer is `{"results": [...], "next_cursor": ...}`, best match first.
`next_cursor` is `null` on the last page.
Pass it back as `cursor` to read the next page.

## Read every page from a script

The HTTP endpoint needs the Viewer role or higher and accepts a session or a service-account key.
The CLI and MCP accept a key.

In the CLI, `--cursor` continues from a `next_cursor`, and `--all` reads every page as one list.
`--limit` takes 1 to 50 and is 20 by default.
Put a word that starts with `-` after `--`, so the CLI does not read it as a flag:

```sh
observe search checkout --kind comment -- -staging
```

The MCP tool `search_workspace` takes `q`, `kinds`, `limit` and `cursor` and returns the same results for the person the agent acts as.
Its `snippet` is plain text, and it may return fewer results than `limit` when the page would be too large, so keep reading with `next_cursor`.

## Reference

### What each kind matches

| Kind | Title and tags | Text | Query text |
| --- | --- | --- | --- |
| `dashboard` | Title and tags. | Description, and the titles and descriptions of panels, rows included. | The query of every panel, rows included. |
| `annotation` | The first line of its text, and its tags. | The text. | None. |
| `observation` | Title. | The summary, and the Markdown of its live text blocks, which hold an investigation's answer. | None. |
| `comment` | None. | One message, with each mention read as `@Name`. | None. |
| `saved_search` | Title. | The service and span filters of a trace search. | The query of a logs, trace or metrics search. |

Details of what is and is not searched:

- Only the latest version of a dashboard is searched, so a word that was removed in a later save finds nothing.
- A comment is one message, not a whole thread, and a result for it names the object it is on.
- Folders, text panel content and prompts are not searched.
- A finding's answer lives in its observation, so it is found as that observation.
- Query text is searched as plain words, so `http_requests_total` is found by `http_requests_total` and by `requests`.

### Who finds what

A result appears only when you can open the object it points to.

- A dashboard is found through its folder, and so are its annotations and comments.
- An [observation](observations.md), its comments and a saved search are found through the folder each is in.
- A global annotation, one on no dashboard, and its comments are found by everyone who can read General.
- A comment is found by whoever can read what it is on.
- Personal folders stay private, Admins included.

A result never shows a title or a snippet of something you cannot open, and a move, a delete or an edit changes the results at once.

### Ranking and limits

- Results are ranked by how well the words match, and among near-equals the newer object comes first.
- A match in a title or a tag counts for more than a match in the text, and a match in query text counts for less.
- A title match always beats a text match of the same frequency.
- A query returns at most 200 results, so refine the words to see more.
- Recency is measured once for a whole search, so paging does not reorder results as time passes, but an edit made while you page can still move a result.

### Request parameters

`GET /api/v1/search` takes these query parameters.

| Parameter | Meaning |
| --- | --- |
| `q` | The words to search for. Required. |
| `kind` | Only results of this kind: `dashboard`, `annotation`, `observation`, `comment` or `saved_search`. Repeat it for several. |
| `limit` | 1 to 50 results, 20 by default. |
| `cursor` | The `next_cursor` of the previous page. |

### Result fields

Each result has these fields.

| Field | Meaning |
| --- | --- |
| `kind` | One of the five kinds. |
| `id` | The id of the object, or of the message for a comment. |
| `title` | The title of the object, or for a comment, of what it is on. |
| `snippet` | Up to two fragments of the matched text, as `{"text", "match"}` segments of at most 300 characters in all. |
| `url` | The page that shows the object, or null for an annotation on no dashboard. |
| `folder` | The `id` and `title` of the folder the result is read through, or null for an annotation on no dashboard. |
| `target` | For a comment, the `kind`, `id` and `title` of what it is on, else null. |
| `thread_id` | For a comment, its thread, else null. |
| `updated_at` | When the object last changed, or for a comment, when it was written or last edited. |

A segment of `snippet` with `match` set is a matched word.
Render a snippet as text, never as HTML.

### Errors

- A malformed query or cursor answers 400 `invalid_request` with the reason, such as `q must be 1 to 200 characters`, `q needs a word to search for` or `cursor is not valid`.
- A `kind` that is not one of the five answers 400 `invalid_request`.

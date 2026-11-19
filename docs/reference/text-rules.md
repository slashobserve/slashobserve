---
title: Text rules
description: What the server accepts in a title, a name, a comment, a prompt or a note, how it trims them and counts their length, and the reason code it returns when text is refused.
---

Every text a person or an agent types goes through one rule on the server, and the web app applies the same rule before it sends, so a form shows the problem as you type.
The rule is the same on the web, over the API, through the CLI and over MCP.

## What is checked

Text is checked in this order, and the first failure is reported.

1. It must be valid UTF-8.
2. Spaces and zero-width characters at its start and end are removed, so `  QA lead  ` is stored as `QA lead`.
3. What is left must hold at least one visible character.
4. It must not contain control characters.
5. It must not contain bidirectional override or isolate characters (U+202A to U+202E and U+2066 to U+2069), which can make a name read as another name.
6. A title or name must not contain zero-width or hidden characters inside it, because two names that differ only by such a character would look identical.
7. Its length, counted in Unicode characters and not bytes, must be within the limit.

A title, name or note is one line: line breaks and tabs count as control characters.
A comment, a prompt and an observation text block may hold line breaks and tabs.
A zero-width joiner is allowed inside text because emoji sequences and some scripts need it.
Tag characters (U+E0000 to U+E007F) are allowed only in the sequence that follows a black flag (U+1F3F4), which is how the England, Scotland and Wales flags are written, and are refused anywhere else.

A dashboard or panel title that an earlier version already holds is kept as it is when you save, duplicate or build on that dashboard, even when it breaks the rules above, so an old dashboard never blocks a save over a title nobody touched.
A title you type or change is checked in full.

## Where it applies

| Text | Limit | One line |
| --- | --- | --- |
| Dashboard title and panel titles | 200 | yes |
| Folder name | 100 | yes |
| Saved search title | 120 | yes |
| Observation title | 200 | yes |
| Observation summary | 500 | yes |
| Observation text block | 8000 | no |
| Chat title | 120 | yes |
| Prompt, continue note, saved prompt body, comment and thread message | 4000 | no |
| Annotation text | 2000 | no |

## Reason codes

A refused write answers with the endpoint's usual code and a `reason` from this table where the endpoint reports one; a dashboard or panel title reports the reason as the `code` of its issue at `/title` or `/panels/{i}/title`.
The `error` message names the field and says what to change.

| Reason | What to change |
| --- | --- |
| `text_empty` | Enter some text. |
| `text_invisible_only` | The text has only spaces or invisible characters; enter visible text. |
| `text_invisible_characters` | Remove the zero-width or hidden characters. |
| `text_control_characters` | Remove the control characters, and the line breaks and tabs in a one-line field. |
| `text_bidi_override` | Remove the bidirectional override characters. |
| `text_too_long` | Shorten the text to the limit. |
| `text_invalid_utf8` | Send valid UTF-8. |

---
title: Text rules
description: What the server accepts in a title, a name, a comment, a prompt or a note, how it trims text and counts its length, and the reason code it returns when text is refused.
---

Titles, names, comments, prompts and notes follow one text rule.
The server applies it on the web, over the API, through the CLI and over MCP, and the web app applies the same rule before it sends, so a form shows a problem as you type.

## Fix text the server refused

A refused write answers with the endpoint's usual error code and a `reason` from the [reason table](#reason-codes).
A dashboard or panel title reports the reason as the `code` of its issue at `/title` or `/panels/{i}/title`.

1. Read the `error` message.
   It names the field and says what to change, for example `title must be at most 200 characters; shorten it`.
2. Find the `reason` in the [reason table](#reason-codes) to see what to remove or shorten.
3. Send the text again.

## Know what the server trims

The server removes spaces and zero-width characters at the start and end of a field, so `  QA lead  ` is stored as `QA lead`.
Spaces and hidden characters inside the text stay, apart from the cases refused in [What is checked](#what-is-checked).
An observation text block is checked as it is sent and is not trimmed.

## Keep an old title that breaks the rules

A dashboard or panel title that an earlier version already holds is kept as it is when you save, duplicate or build on that dashboard, even when it breaks the rules.
An old dashboard never blocks a save over a title nobody touched.
A title you type or change is checked in full.

In the same way, an observation text block whose text is sent back unchanged is not checked again.
A block saved earlier with control characters can still be moved or resized.

## Reference

### What is checked

The server checks the characters of a field in this order and reports the first failure.

1. It must be valid UTF-8.
2. Spaces and zero-width characters at its start and end are removed.
3. What is left must hold at least one visible character.
4. Each character is checked in turn, and the first bad one decides the reason:
   - a control character is refused, and a one-line field counts line breaks and tabs as control characters;
   - a bidirectional override or isolate character (U+202A to U+202E and U+2066 to U+2069) is refused, because it can make a name read as another name;
   - a tag character (U+E0000 to U+E007F) is refused unless it follows a black flag (U+1F3F4);
   - in a one-line field, a zero-width or hidden character is refused, because two names that differ only by such a character would look identical.
5. Its length, counted in Unicode characters and not bytes, must be within the limit.

### Details of the characters

- A comment, a prompt and an observation text block may hold line breaks and tabs.
  A title, name or note is one line.
- A zero-width joiner (U+200D) and a zero-width non-joiner (U+200C) are allowed inside text, because emoji sequences and some scripts need them.
- A multi-line field allows other zero-width characters inside it.
- A one-line field refuses these hidden characters inside it: U+200B, U+FEFF, U+00AD, U+180E, U+2060 to U+2064, U+206A to U+206F, and the fillers U+034F, U+115F, U+1160, U+2800, U+3164 and U+FFA0.
- Tag characters are allowed only after a black flag, which is how the England, Scotland and Wales flags are written.
  Anywhere else, in any field, they are refused with `text_invisible_characters`.

### Where it applies

| Text | Limit | One line |
| --- | --- | --- |
| Dashboard title and panel titles | 200 | yes |
| Folder name | 100 | yes |
| Saved search title | 120 | yes |
| Observation title | 200 | yes |
| Observation summary | 500 | yes |
| Observation text block | 8000 | no |
| Chat title | 120 | yes |
| Public link label | 80 | yes |
| Prompt, continue note, saved prompt body, comment and thread message | 4000 | no |
| Annotation text | 2000 | no |

The limits count characters.
A public link label may be empty, and the other fields need at least one visible character.
A refused public link label answers `label must be at most 80 characters without control characters` and no `reason`.

A saved prompt's name, a comment access request's note, a service account's name and a team name follow their own rules, described with them.
See [Saved prompts](../teams/saved-prompts.md), [Folders and access](../teams/folders.md#ask-for-comment-access), [People](../teams/people.md#teams) and [Service-account keys](../operate/service-account-keys.md).

### Reason codes

| Reason | What to change |
| --- | --- |
| `text_empty` | Enter some text. |
| `text_invisible_only` | The text has only spaces or invisible characters; enter visible text. |
| `text_invisible_characters` | Remove the zero-width or hidden characters. |
| `text_control_characters` | Remove the control characters, and the line breaks and tabs in a one-line field. |
| `text_bidi_override` | Remove the bidirectional override characters. |
| `text_too_long` | Shorten the text to the limit. |
| `text_invalid_utf8` | Send valid UTF-8. |

---
title: Saved prompts
description: Save a prompt once, keep it private or share it, and run it again from the chat home, the picker or the CLI.
---

A saved prompt is a name and the text of a request to the agent.
Save the prompts you send often, such as "build a latency dashboard for checkout", and run them again with one click.
Each prompt belongs to the person who saved it and is either private or shared with everyone on the server.

Running a prompt sends its text to the agent as a new request, exactly as if you had typed it.
The agent must be on; see [LLM providers](../connect/llm-providers.md).

## Save and run a prompt

You need the Editor or Admin role to save a prompt.

1. Open the chat home or any conversation and choose **History**.
2. Point at one of your chats and choose the bookmark button (**Save prompt**).
   On a device without hover the button is always visible.
3. Check the name and the text, choose **Only me** or **Everyone on this server**, and choose **Save prompt**.
   The panel starts with the first three words of the prompt as the name and the whole prompt as the text.
4. Open the chat page: go to `/ask`, or open Home and choose **Ask Observe Agent**.
   Your prompt is listed under the example prompts.
5. Choose **Run** on its row.
   The prompt is sent at once and the conversation opens.

The bookmark button appears on a chat whose request came from the chat box and was not about an existing dashboard.
A chat you started by running a saved prompt has it too.
A comment revision, an investigation and a follow-up message on an existing dashboard have none.

## Run a prompt from the keyboard

Type `/` as the only line in the prompt box of Home or of the chat page.
The picker lists the prompts that match what follows the slash.

- The arrow keys move through the list.
- **Enter** fills the box with the prompt, and does not send it.
  Edit the text if you want, then press **Enter** again to send.
- **Esc** closes the picker.

## Run a prompt from the CLI

`observe run` lists the prompts shared with everyone, and `observe run <name>` runs one.
The name matches without regard to case, and its words may be unquoted.
A key sees only shared prompts, so share a prompt in the web app first.
A key can run one when its service account is an Editor or Admin.
See [CLI](../use/cli.md).

## Change, share or delete a prompt

A prompt you can edit has a menu (the three dots) on its row in the chat page list, with **Edit**, **Share with everyone** (or **Make private**) and **Delete**.
Deleting a shared prompt removes it for everyone, and `observe run` stops finding it.

Only the owner can edit a prompt.
An Admin can also edit and delete any shared prompt, but cannot see another person's private prompts.

## See who changed a shared prompt

The [audit trail](../reference/audit-queries.md#event-data) records a shared prompt being saved, changed, made private and deleted, and a prompt being shared.
Each event names who did it and which of the name, text and sharing changed, and never holds the text.
A successful change to a prompt that is private before and after is not recorded.
A refused or failed request on a private prompt is, with the route and the prompt's id but never its name or text.

## Reference

### Who can use saved prompts

| Role | Saved prompts |
| --- | --- |
| Admin | Create, run and change their own, and change or delete any shared prompt. |
| Editor | Create, run and change their own, and run shared prompts. |
| Commenter | See and run shared prompts. The API refuses a change. |
| Viewer | See and run shared prompts. The API refuses a change. |

Running a prompt starts a new dashboard, which needs Edit on some folder, and every person has Edit on their own My Dashboards (see [Roles](roles.md) and [Folders and access](folders.md)).
A Commenter or Viewer therefore sees and runs every shared prompt, which starts a dashboard in their own My Dashboards, and keeps no prompts of their own.
Creating and editing prompts stays with Editors and Admins.

### Limits

| Item | Limit |
| --- | --- |
| Name | 1 to 64 characters. |
| Text | 1 to 4000 characters. |
| Prompts per person | 100. |
| Prompts in a list | 500, your own first. |

A name holds letters, numbers, spaces, `-` and `_`, and starts and ends with a letter or number.
Runs of spaces collapse to one.
The text needs at least one visible character.
It cannot contain control characters other than newlines and tabs, or the bidirectional override characters; see [Text rules](../reference/text-rules.md).
A name is unique among your own prompts and among shared prompts, ignoring case, so you cannot share a prompt whose name another shared prompt already has.

### What the chat page shows

The list sits under the example prompts and the dashboards.
Each row shows the name, a preview of the text, and who can use it.

- **Only me** is your private prompt.
- **Everyone** is your prompt shared with everyone.
- **By** and the owner's name is someone else's shared prompt.

The list shows 5 rows and a **Show all** button, labelled with the total, for the rest.
It is hidden when there are no prompts to show or the agent is off.
The Home page (`/`) has no list, only the picker.

### What the picker shows

The picker orders matches as follows: names that start with the text after the slash, names that contain it, then prompts whose text contains it.
It shows the first 50.

### HTTP API

| Route | Role | Use |
| --- | --- | --- |
| `GET /api/v1/prompts` | Viewer | List your prompts and the shared ones. |
| `POST /api/v1/prompts` | Editor, signed in as a person | Create one from `name`, `body` and `shared`. |
| `PATCH /api/v1/prompts/{id}` | Editor, signed in as a person | Change at least one of `name`, `body` and `shared`. |
| `DELETE /api/v1/prompts/{id}` | Editor, signed in as a person | Delete one. |

A service-account key cannot create, change or delete a prompt.

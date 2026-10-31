---
title: Saved prompts
description: Save a prompt once, keep it private or share it, and run it again from the chat home, the picker or the CLI.
---

A saved prompt is a name and the text of a prompt.
Running one sends that text to the agent as a new request, exactly as if you had typed it, so it needs the agent to be on (see [LLM providers](../connect/llm-providers.md)).
Each prompt belongs to the person who saved it and is either private or shared with everyone on the server.

## Save a prompt

The web app has one place to save a prompt, the History panel.

1. Open the chat home or any conversation and choose **History**.
2. On the **Chats** tab, point at a chat and choose the bookmark button (**Save prompt**).
   On a device without hover the button is always visible.
3. Check the name and the text, choose **Only me** or **Everyone on this server**, and choose **Save prompt**.

The button appears only on a chat that started a new dashboard.
A follow-up message on an existing dashboard has none.
The panel starts with the first three words of the prompt as the name and the whole prompt as the text, and you can change both.

A name is 1 to 64 characters: letters, numbers, spaces, `-` and `_`, starting and ending with a letter or number.
Runs of spaces collapse to one.
The text is 1 to 4000 characters and cannot contain control characters other than newlines and tabs.
A name is unique among your own prompts and among shared prompts, ignoring case, so you cannot share a prompt whose name another shared prompt already has.
A person can keep 100 prompts.

Over the API, `POST /api/v1/prompts` takes `name`, `body` and `shared`.
It needs a signed-in person.
A service-account key cannot create, change or delete a prompt.

## Run a prompt

On the chat home, saved prompts are listed under the example prompts: name, a preview of the text, and who can use it (**Only me**, **Everyone**, or **By** the owner's name for someone else's shared prompt).
The list shows 5 and has a **Show all** button for the rest.
**Run** sends the prompt at once and opens the conversation.
The list is hidden when you have no prompts or the agent is off.

To pick one without leaving the keyboard, type `/` as the only line in the home prompt box.
The picker lists the prompts that match what follows the slash, in this order: names that start with it, names that contain it, then prompts whose text contains it.
It shows the first 50.
The arrow keys move, **Enter** fills the box and **Esc** closes the picker.
Filling the box does not send it, so you can edit the text first and press **Enter** again to send.

With the CLI, `observe run <name>` runs a prompt and `observe run` alone lists them.
A key sees only prompts that are shared, so share a prompt in the web app first.
See [CLI](../use/cli.md).

## Change, share or delete a prompt

On each prompt you can edit, the home list has a menu (the three dots) with **Edit**, **Share with everyone** (or **Make private**) and **Delete**.
Deleting a shared prompt removes it for everyone, and `observe run` stops finding it.

Only the owner can edit a prompt.
An Admin can also edit and delete any shared prompt, but cannot see another person's private prompts.

## Who can use them

| Role      | Saved prompts                                                                                                                           |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Admin     | Create, run and change their own, and change or delete any shared prompt.                                                               |
| Editor    | Create, run and change their own, and run shared prompts.                                                                               |
| Commenter | Run shared prompts only when a folder's list gives them Edit. Otherwise none, and the chat home shows a note. The API refuses a change. |
| Viewer    | Run shared prompts only when a folder's list gives them Edit. Otherwise none, and the chat home shows a note. The API refuses a change. |

Running a prompt starts a new dashboard, which needs Edit on some folder (see [Roles](roles.md) and [Folders and access](folders.md)).
A Commenter or Viewer without such an entry sees the chat-home note and no saved prompts.
Creating and editing prompts stays with Editors and Admins.
A key can run shared prompts with `observe run` when its service account is an Editor or Admin.

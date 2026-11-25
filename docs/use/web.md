---
title: Addresses and viewing tools in the web app
description: The short addresses of Home, the inbox, dashboards, folders, chats, builds, observations, proposals, agents, logs and traces, their title slugs and old forms, selecting a panel to act on it and arranging panels, the tools for viewing a dashboard at full size, in Inspect and on a phone, with the dashboards list and a dashboard's More menu, a pointer to TV mode, the sidebar, and the light and dark theme.
---

This page covers how to find your way around the web app and how to view a dashboard.
Every page that belongs to one dashboard, folder or agent request has a short address you can paste into an incident, a runbook or a chat.
The sections below follow what you want to do, and the Reference at the end lists every address and key.

## Share the address of a page

1. Open the page and copy the address from the browser.
2. Paste it where your team will read it.
   Anyone who can see the dashboard, folder or request opens the same page.

A dashboard is `/d/<dashboard id>/<slug>`, a folder is `/f/<folder id>`, an observation is `/o/<observation id>` and an agent request is `/c/<request id>`.
[Addresses](#addresses) lists them all.

[Logs and traces](logs-and-traces.md) describes the logs page, the trace search and the trace view, and [Metrics](metrics.md) describes the metrics page.

The lists keep their names: `/dashboards` lists dashboards, `/` opens Home, and `/` with a query string other than `tab` opens the chat at `/ask` with its query string kept.

An address that nothing answers opens **Page not found** inside the app, with the sidebar and the top bar, the address it could not find and links to Home and the dashboards.
A signed-out visitor signs in first and then sees it.
A folder's page names the folder in the breadcrumb and in the tab title, such as "Dashboards / Checkout" and "Checkout - SlashObserve".

`/logs`, `/traces` and `/explore/metrics` list their sources and saved searches.
An address there that carries a search opens the source you used last on this browser, even though the source lists keep the order they had when the app loaded.
An address with `?ds=<datasource uid>` redirects to the source's own page with the rest of its query string.

### Slugs

The slug is the dashboard title in lowercase, with every run of other characters turned into one dash, trimmed, and cut to 60 characters.
Only the letters `a` to `z` and the digits `0` to `9` are kept, so `Checkout health (prod)` becomes `checkout-health-prod` and a title with no such characters has no slug.
The slug is only for people: `/d/<dashboard id>` opens the same dashboard, and so does any address with an old slug.
When a dashboard opens, the address bar switches to the slug of its current title without adding a history entry, so a copied address always carries the title.
A title whose slug would be `versions` gets no slug, because `/d/<dashboard id>/versions` is the history page.

The server writes the same form wherever it returns an address: the `url` fields of the API, the MCP tool results and the `observe` CLI.
The address of a request read with `GET /api/v1/observe/{id}` has no slug, and the page adds it when it opens.

### Old addresses

The old forms keep working and redirect with `301 Moved Permanently`, keeping the query string, so bookmarks, curl and links in old runbooks reach the new address.
[Old address forms](#old-address-forms) lists them.

The server redirects only when the id is a UUID, and the web app does the same for links it follows itself.
`/dashboards/by-title?title=<title>` is not an id address and keeps working as described in [ObserveSpec](../concepts/observespec.md).

A Grafana dashboard's own address, `/d/<grafana uid>` or `/d/<grafana uid>/<slug>`, redirects with `302 Found` to `/d/<id>` of the dashboard imported from that uid, keeping the slug and the query string.
It resolves only for a signed-in caller who can see the dashboard.
A signed-out visitor signs in first and comes back to it.
A uid nobody imported, or whose dashboard is in a folder you cannot see, opens the app's not-found page.

### When your session ends

When an Admin uses Sign out everywhere, or the server drops your session, the next page or save sends you to the sign-in page, which says "Your session was ended, so you were signed out."
After you sign in you return to the page you were on.
Signing out yourself shows no such note.

### Public links

A dashboard's Share pane can create a public link, whose `/p/<token>` address opens the dashboard read-only for anyone who has it, without signing in.
The page reads only the time range, the refresh interval and, on a free link, the variables the viewer may choose from its address.
[Public links](public-links.md) describes who can create one, what the view shows, and how to revoke one.

## Find, organise and pin dashboards

### The dashboards list

`/dashboards` lists the dashboards you can see, folder by folder.

- **Create dashboard** is a menu with **Ask the agent**, which opens the chat at `/ask` where you describe the dashboard you want, and **Blank dashboard**; see [Start from a blank dashboard](edit-panel.md#start-from-a-blank-dashboard).
- **New folder**, offered to an Editor or an Admin, asks for a name and creates a folder you own.
  It appears in the list as soon as it is created.
- The box above the list filters by title, and the folder and tag menus narrow it further.
- Each row has a **More** menu, offered where you may edit the dashboard or duplicate it.
  [Details of a row's More menu](#details-of-a-rows-more-menu) lists its items.

### Move several dashboards at once

Where you may edit dashboards and there is a shared folder you can move into, every row you may move has a checkbox, and the checkbox in the header selects all of them on the page.

1. Tick the rows, or tick the header checkbox.
   A bar above the list shows how many are selected, a **Move to folder** menu of the shared folders you can edit, **Move** and **Clear selection**.
2. Pick a folder in **Move to folder**.
3. Choose **Move**.

**Move** moves each selected dashboard with one request, as a row's **Move to folder** does, and skips the ones already in that folder.
It says how many moved and which failed and why, and the ones that failed stay selected.
The selection is kept for the page and filters you are on and cleared when you change either.
An agent does the same with one `move_dashboard` call for each dashboard.

### Sort the list

The **Sort** menu on the list switches between **By folder**, **Last updated**, **Name** and **Most viewed**.
Every order except by folder drops the folder headers and shows the folder as a column, because the order crosses folders.
The choice is kept in the address as `sort=updated`, `sort=name` or `sort=views`, and the list asks the server for the same `order`.

### How often a dashboard is opened

The dashboards list has a **Views** column and the dashboard's header says how many views it had, such as **12 views in 30 days**.
A view is one person opening the dashboard on one UTC day, so three people who each open it twice today make three views.
The count covers the last 30 UTC days including today, and the daily records are kept for 90 days.
Opening a dashboard writes once for the page, never for each panel, and a service-account key records nothing.

`GET /api/v1/dashboards/<id>/views` returns the count as `view_count` with `window_days`, and each item of `GET /api/v1/dashboards` carries the same `view_count`.
`order=views` on the list sorts by it, most viewed first and then most recently updated, and it pages with `cursor` and `offset` like the other orders.
A cursor only works with the order it came from.
`order=name` sorts every visible dashboard by lower-case title and then by id, across folders, and pages the same way.

`list_dashboards` takes `order` (`updated`, `views` or `name`) and returns `view_count`, and `get_dashboard` returns `view_count` and `window_days`.
`observe dashboards list --order views` sorts the list and `observe dashboards show` prints the views.

### Find a dashboard from the Ask page

The chat page at `/ask` lists your dashboards below the chat composer and its example prompts, or above the model note when no model is connected.
It shows Recently viewed: the last five dashboards you opened in this browser.
When you have none, it lists the first eight dashboards you can see, folder by folder, so a fresh demo shows its provisioned dashboard.

The search box below them finds dashboards by title or description, and `tag:` words narrow it to tags: `tag:prod latency` finds dashboards tagged `prod` that mention `latency`.
Arrow keys move through the results and Enter opens one.
Recently viewed dashboards stay in this browser's local storage, at most 20, and never reach the server.

### Pin a dashboard to the sidebar

1. Open the dashboard and choose the pin button in its header, labelled **Pin to sidebar**.
2. The dashboard appears in the sidebar under **Dashboards**, indented, newest pin first.
3. To unpin it, press the button again, which now reads **Unpin from sidebar**.

The pinned dashboard you have open is highlighted, while **Dashboards** itself turns semibold instead of highlighted.
The sidebar lists ten pins and then **Show all pins**, which lists every pin in place until you choose **Show fewer pins**.
The phone menu lists them too.
On the icon rail, **Dashboards** opens a menu with a **Dashboards** link and your pins.

Pins belong to you and are stored on the server, so they follow you to every browser.
`GET /api/v1/stars` lists them, because the API keeps its original `stars` name.
Only people pin: a service-account key gets `403 user_session_required`.
A pinned dashboard in a folder you can no longer see leaves the sidebar, and deleting a dashboard removes its pins.

### Home dashboard

Choose **Open on sign-in** in a dashboard's **More** menu and that dashboard opens in place of Home when you sign in or open SlashObserve in a new tab.

- Home opens as usual for a link that names a page, such as a shared dashboard link or `/?tab=observe`, and when you click **Home** in the sidebar.
- Choosing another dashboard replaces the first.
- The same menu item, now **Stop opening on sign-in**, clears the choice.
  So does **Clear** under **Settings**, **Home dashboard**.
- If the dashboard was deleted or you can no longer view it, Home opens and says so with a **Clear it** button.
  The choice comes back to work if you can view the dashboard again.

The choice belongs to you and follows you to every browser.
`GET`, `PUT` and `DELETE /api/v1/me/home-dashboard` read, set and clear it.
`status` is `none`, `ok`, `gone` or `unreadable`, and setting a dashboard you cannot view answers `404 not_found`.
Over MCP, `get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard` do the same.
Only people have one, so a service-account key gets `403 user_session_required`, and the CLI has no command.

## Use a dashboard's More menu

The **More** button at the right of a dashboard's header opens the actions below.
[Details of the More menu](#details-of-the-more-menu) lists each item.
While you have unsaved layout moves, the items that would change the dashboard say "finish the layout first".

### Print a dashboard or download it as an image

**Print or save as PDF** in a dashboard's **More** menu lays the dashboard out for paper and opens the browser's print dialog, where **Save as PDF** is a destination.
The page shows only the dashboard: no sidebar, toolbar or side panes.
It has a header with the title, folder, version, time range, variables and the time of printing, and then the panels in one column in reading order.

- Every panel loads before the dialog opens, including the ones far down the page.
- A panel is moved whole to the next page instead of being cut when it does not fit.
- Rows you collapsed are expanded on the sheet so that no panel is left out.
- The variables and time range you chose are the ones printed.
- The sheet prints in the light theme whatever theme you use, so a dark theme does not print as a page of dark boxes.
  The image keeps your current theme.

**Back to dashboard** at the top of the page, or closing the print dialog, returns to the dashboard.

**Download as image** in the same menu opens the same sheet, draws it as one PNG named after the dashboard and its version, such as `checkout-service-health-v7.png`, and returns to the dashboard.
The image is drawn in your browser from the rendered panels, with no server involved.
It is drawn at twice the size unless the dashboard is so tall that the browser's canvas limit needs a smaller scale.
If the browser cannot draw it, a message says so and suggests **Print or save as PDF**.

Both items are off while you are rearranging the layout, and a single panel is still saved from its own menu.
Neither needs a server feature, so there is no API operation, CLI command or MCP tool for them.
An agent that needs the data reads the dashboard and its panel queries instead.

### Show a dashboard on a wall screen

TV mode shows a dashboard on a wall screen with nothing but its panels, and can cycle through its rows or its folder's dashboards.
[Show it on a wall](public-links.md#show-it-on-a-wall) describes how to start it, the address keys that open a TV straight into it, and how it fits with public links.

### Choose how hovering shows on other charts

**Chart hover** in the **More** menu, also Cmd/Ctrl+O, chooses how hovering one time chart shows on the others: **This chart only**, **Shared crosshair** or **Shared tooltip**.
A choice that differs from the dashboard's saved `cursorSync` goes into the address as `cursor` and is not saved to the dashboard.

## Act on a panel

### Select a panel

A dashboard is a canvas: select a panel, then act on it.

1. Click the panel's title bar, or Tab to its title and press Enter.
   The panel gets a ring and a small toolbar above its top right corner.
2. Choose a button on the toolbar.
3. Press `Esc` to clear the selection.

With the title focused, `↑`, `↓`, `Home` and `End` move the selection through the panels in reading order.
The toolbar shows only what you may do:

- **Comment**, when you can comment, opens the composer on the whole panel, as `c`, the panel menu's **Comment** and the **Comment** button in the panel's header do.
  In the full-size view and TV mode it starts the comment in the activity pane instead.
- **Ask @observe**, when you can comment on the dashboard and a model is connected, opens the [ask box](ask-and-comment.md#ask-mode) on the whole panel, on a data panel, as `a`, the panel menu's **Ask** and the **Ask** button in the panel's header do.
  In the full-size view and TV mode it opens the dashboard's Chat tab instead, with the cursor in its box and a chip naming the panel, so the question is about that panel until you remove the chip.
- **Edit query** opens the panel editor on its query when you can edit the dashboard, and reads **Suggest an edit** when you can only comment.
- **Add to observation** is described in [Add to an observation](#add-to-an-observation).
- **More actions** opens the panel's menu, which every panel also has in its own title bar without selecting it.

The toolbar moves below the panel's title bar when the panel is less than 48 pixels from the top of the dashboard area.
The arrow keys move between the toolbar's buttons, and `Home` and `End` go to the first and last.
While you have unsaved layout changes, Comment, Ask, Edit query and Add to observation are greyed.

At the bottom of the screen a tool bar offers **Select**, **Comment** and **Ask**.
It is hidden on a phone, in TV mode and when you cannot comment, as a Viewer cannot.
**Comment** and **Ask** turn on the pointing modes, where a click or a drag names a spot on a panel and opens a box there to leave a comment or to ask the agent.
[Ask and comment on a dashboard](ask-and-comment.md) describes the modes, what a click can name, the pins and private asks.

### Arrange panels

There is no edit mode.
You can move a panel when you can edit the dashboard, are on its latest version and the dashboard area is at least 768 pixels wide.

1. Press a panel's title bar and drag.
   The pointer turns into a hand over every title bar, and a faint grip with the tip "Drag to move" appears at its left end.
   The panel is selected and moves in one go.
2. To resize it, drag a handle that shows on its right edge, bottom edge or bottom right corner while the pointer is over the selected panel's grip.
   An edge changes the width or the height, and the corner changes both.
3. Let go.
   A dashed outline shows where the panel will land, the other panels slide out of its way, and it settles into place.
4. When the bar at the bottom of the screen says "Your layout changes are not saved", choose **Save version N** and give a summary.
   It saves the layout as the next version.
   **Discard** asks "Discard unsaved layout changes?" and then drops the moves.

With the grip focused, the arrow keys move the panel and `Shift` with an arrow resizes it.
The panel's buttons, such as its menu and the information icon, never start a move.
In Comment mode the hint, the grip and the handles are hidden and a click on a panel comments on that spot instead.
While a panel is selected for arranging, the grid shows the saved layout, so a repeated panel shows once and a row cannot be collapsed until you clear the selection.

When someone else saves a version, your moves stay on top of it, and a panel you did not touch takes the other version's size and place.
Leaving the dashboard, or opening an older version, with unsaved moves asks first.
While you have unsaved moves, the actions that would change the dashboard in another way, such as Edit details, Duplicate, Delete, Inspect and View, say "finish the layout first".

When the Activity or chat pane is open beside the grid, selecting a panel to arrange closes it first, so the saved layout gets the full width.
The press that selects still drags the panel.
The pane comes back as you left it once no panel is selected and no moves are unsaved, for example after you press `Esc` or click an empty part of the grid.
**Activity** in the controls brings it back sooner, and it then stays open while you select other panels.
A Viewer who cannot edit, or a press in Comment or Ask mode, leaves the pane open.
While you have unsaved moves the pane stays closed.

### Look at one panel at full size

**View** in a panel's menu, or `v` over a panel, shows that panel alone in the whole dashboard area.
The address carries `viewPanel=<panel id>`, so the view can be shared.
**Back to dashboard**, `Esc` or `v` again returns to the grid.

**View as table** in the menu of a chart panel replaces the chart with a table of its data in that panel until you choose **View as chart**.
Nothing is saved.

### Inspect a panel

**Inspect** in a panel's menu, or `i` over a panel, opens a pane in the place of the activity card.
The address carries `inspect=<panel id>` and, for the second and third tab, `inspectTab=query` or `inspectTab=stats`.
`Esc` or the close button closes it.
`viewPanel` and `inspect` are dropped while you have unsaved layout changes or the panel editor is open.

The pane has three tabs:

- **Data** shows the panel's results after its transforms as a table of up to 500 rows, with **Download CSV** and **Save as PNG**.
- **Query** shows the time range and step the server ran, and for each query its datasource, step, saved text, the text as run and the variables it uses with their selected values.
  The text as run is the query after variables and macros are expanded.
  It shows when it differs from the saved text and never on a public link.
- **Stats** shows how long the request took, whether the server answered from its cache, when it ran, and the number of frames, series, points and rows with the first and last time.
  **Run again** runs the panel's queries again.

**Download CSV** and **Save as PNG** are always in each panel's menu and are greyed until its data has loaded.
The CSV has every row, quoted as RFC 4180 describes, with times in RFC 3339 UTC.
When every series of the panel has its own time column with no repeated times, the CSV joins them into one row per time, with one column per series.
Otherwise each row starts with a `series` column naming the series it came from.
The PNG is the panel's body at twice the screen's pixel density, on the page's background colour, named after the panel title.

### See which series a panel left out

A time series or trend panel draws at most 50 series per query.
The server keeps the 50 whose highest value is largest, in their original order, and the panel header says "Top 50 of 412 series" with the full sentence on hover.
Narrow the query or add a filter to see the series that were left out.

In the data response `meta.series_shown` and `meta.series_total` carry the two numbers, `meta.partial` is true and `meta.note` says which series were kept.
They are absent when nothing was dropped.

### Open a panel in Logs or Traces

**Open in Logs** in a logs panel's menu, and **Open in Traces** in a traces panel's menu, open the explorer with the panel's query, its datasource and the dashboard's time range.
The item is greyed, with the reason after it, when the panel has several queries, has no datasource, uses variables in its query, or is a ClickHouse SQL trace search.

### Duplicate or remove a panel

An Editor's panel menu has **Duplicate** and **Remove** on the latest version, while no layout change is unsaved.

- **Duplicate** saves a new version with a copy titled "<title> copy" right under the panel, moving the panels below it down, and runs only the copy's queries.
  A copy of a panel whose query returns no data, such as one you saved with **Save anyway**, is kept as it is and marked "Unverified" like the original.
- **Remove** asks to confirm with the panel's title, then saves a new version without the panel and runs no query.
  Restoring the version before it from History brings a removed panel back.

Agents and scripts make the same changes with the `update_panels` MCP tool or `observe dashboards panels`, both on `POST /api/v1/dashboards/{id}/panels`.

### Add to an observation

**Add to observation** in a data panel's menu copies the panel into an [observation](observations.md), the open one you choose or a new one.
A panel that uses dashboard variables is copied with them and the values you have selected, see [Add to an observation](observations.md#add-to-an-observation-from-a-panel-or-from-logs).
The item is greyed in the toolbar and missing from the menu while you have unsaved layout changes.
[Observations](observations.md#add-to-an-observation-from-a-panel-or-from-logs) describes the side panel and the range the block takes.

## Check a dashboard's health

### Health on a dashboard

When the last health check found a problem in the dashboard, its header has a button that counts the problems, such as **2 problems**.
A dashboard that was checked and has no problem shows a quiet **Queries OK** button instead, whose popover says "No problems found" and when and on which version it was checked.
The button reports whether the queries ran, not whether the numbers look right, so read a panel's threshold state for that.

1. Choose the problem button.
   A popover lists every problem, when and on which version the dashboard was checked, and **Repair**.
2. Read the problems.
   The popover says "before the latest version" when the dashboard has been saved since the check.
3. As an Editor, choose **Repair**.

A panel that failed the check, or returned no data, shows a badge in its title bar with the kind of the problem.
Choose the badge to read the message of every problem in the panel.
The badges show only while you look at the version that was checked.
Another version, such as an older one or the latest one after a save, shows the header button but no badges.
A panel that is also **Unverified** shows only its Unverified badge.
**Not opened in 90 days** is never listed here, because you just opened the dashboard.

### Repair failing panels

An Editor sees **Repair** in the popover of the header button and in the popover of a badge.
It asks the agent to fix every failing panel and variable on the dashboard in one revision, not only the panel you chose.
The header says **Repair all** when more than one problem can be repaired.

Both places show the same states: **Repair queued**, **Repairing**, **Repaired** with **See changes**, and **Repair failed** with the reason and **Repair** again.
While you have unsaved layout changes, **Repair** is greyed and says "finish the layout first".
Without a configured agent it is greyed too.
The problems come from the last health check; see [Dashboard health](dashboard-health.md).

## Search and move around

### Search

Press `⌘K` or `Ctrl+K`, or choose **Search or jump to** in the sidebar (in the top bar on a phone), and type at least two characters.
The command palette lists the pages, your dashboards and the log and trace sources that match.
After a short pause it adds what the server finds in the workspace: dashboards, observations, comments, annotations and saved searches you can read, under their own headings with the matched words marked.

- Typing a source's name or type, such as Loki, lists the source under Sources and jumps to its page.
- Press Enter, or click, to open a result here, and hold `⌘` or `Ctrl` to open it in a new tab.
- **Keyboard shortcuts** in the palette opens the list of keys on any page.
- A comment opens the page of what it is on, and an annotation that is on no dashboard is left out of the palette, because it has no page.
- Snippets show plain text, with Markdown links and emphasis flattened.
- With nothing typed, the palette lists the observations you opened last under **Recent observations**.

**See all results** at the end of a search, or the address `/search?q=checkout`, opens the Search page.
It shows every match in pages of 20 with **Show more**, one button per kind to narrow them, and global annotations as rows with no link.
Search matches whole words, their English stems and the start of the last word, so a misspelling finds nothing, and the empty result says so.

**Help and docs** in the palette, and in the account menu, opens these docs in a new tab.
The empty Dashboards list and the Search page link to their docs page the same way.
[Search](search.md) describes what each kind matches and how to write a query.

### Use the sidebar

The sidebar starts with **Home**, then Inbox, Observations, Dashboards, Logs, Traces, Metrics and Agents.
A Workspace group follows with Datasources, API keys, People and Audit for Admins and Settings for everyone.
The Workspace group stays at the bottom of the sidebar, above your account, while the items above it scroll, so Datasources, People and Settings never sit below the fold.
Open observations are listed indented under **Observations** the same way.

**Audit** lists its three pages indented under it, **Log**, **Dashboards** and **Observations**.
On the audit datasource's page under Logs, which **Open in Logs** on the Audit log reaches, **Audit** is the parent that turns semibold and **Log** the highlighted page, not **Logs**.
On the page of a listed observation, pinned dashboard, or log, trace or metrics source only that row is highlighted, and its parent item is not highlighted but shown in semibold.
On any other page of the section, such as the list itself, the parent is highlighted as usual.

To fold the sidebar to a rail of icons, choose **Collapse sidebar**, beside your account at the bottom.
**Expand sidebar**, above your avatar, opens it again, and this browser remembers the choice.
Under 1200 pixels wide, and on dashboard pages, the sidebar is always a rail, and **Expand sidebar** opens the full sidebar over the page.
`⌘\` or `Ctrl+\` does the same as these buttons on every page that shows the sidebar, including while you type in a field, and does nothing on a phone.

Hovering or focusing an icon on the rail shows its name.
On the rail, Inbox shows its unread count and Observations shows a teal dot while one of your open observations has something unread.
Dashboards and Audit open a menu that Escape or a click outside closes.
With two or more sources, Logs, Traces and Metrics open a menu with a link to their list and the sources.
A pinned dashboard, in the sidebar and in the Dashboards menu, shows its problem count from the last health check, such as "1 problem", and nothing while it has none.

### Use the keyboard

Dashboards answer `v`, `e`, `i`, `a`, `c`, `Esc`, `t z`, `t ←`, `t →`, `d r`, `d k`, `f` and `?`.
[Keyboard shortcuts](keyboard-shortcuts.md) lists them all.

## Use the app on a phone or a narrow screen

Under 900 pixels wide, every page has a bar at the top with **Menu**, which opens the sidebar, the page's name, or the wordmark on Home, **Search or jump to** and **Inbox** with its unread count.
On a dashboard, a build and a dashboard's version history the bar shows the dashboard's name.
On a dashboard the dashboard's name, its version button and **Controls** sit under it, without the folder crumbs, which the sidebar and **Dashboards** page keep.
A page's own actions, such as **Share** on an observation or **History** on a chat, sit in a row under the bar.

Below 768 pixels wide a dashboard shows its panels in one column.
Each panel type keeps a minimum height there, so a chart or a table stays readable even when the dashboard made it short.

Up to 760 pixels wide, the time range, refresh, annotation filter and the dashboard's actions move behind one **Controls** button, which opens them in a sheet at the bottom of the screen.
Choosing **Activity**, **Share**, **Add panel** or **Edit Spec JSON** in the sheet closes the sheet first, and closing the pane puts focus back on **Controls**.
The activity pane starts closed at 900 pixels wide and below, whatever this browser saved on a wider screen, and **Activity** in the controls opens it.
The activity card and the Inspect pane show below the panels, and opening Inspect scrolls to it.
The panel editor opens as a sheet at the bottom of the screen, and the panel menu does not show keyboard shortcut letters.

### Wide and narrow windows

On the Dashboards page, the Datasources and Open comments columns fold away when the list is under 1000 pixels wide, between the phone layout and a full-width window, so the title, health and actions keep their room.
The Dashboards list fills the page: the title takes the spare width, keeps at least 240 pixels, and a narrower page scrolls the table sideways.
Home and the Ask page stop growing at 720 pixels, and at 960 pixels on a window 1800 pixels wide or more.
On a dashboard, the title and the controls share one row while they fit, and the controls drop to a second row under the title when they do not, for example with the Activity pane open on a 1440 pixel screen.
Browser zoom scales the whole app if the text is small on a large display.

## Manage the agents you connected

Settings groups its sections under You, Workspace and Integrations, and Admins also see Agent access and the Integrations group.
A section's address is `/settings#<section>`, such as `/settings#grafana`; `#grafana-connection` opens the same section.
An address whose section does not exist, or one that only Admins can open, shows Server with a note that says so.
The audit log is not a Settings section any more: `/settings#audit-log` redirects to the Audit page at `/audit`.
On a phone the section list is a **Settings section** menu at the top of the page.

1. Open Settings and choose **Connected agents** in its section list.
   **Your connected agents** lists the MCP clients you approved, each with the name it registered under, the host your browser returned to when you approved it, when it connected and when it last made a request.
2. To end a connection, choose **Disconnect** and confirm.
   The connection ends at once, and the client has to be connected again from the agent to work.
3. To add another, use the **Connect another** card, which shows the command that adds this server to Claude Code.
   It reads **Connect an agent** until you have connected one.

A note under the list explains the two modes: Direct saves change dashboards, and Propose saves become proposals an editor merges.
The API behind it also returns each connection's presence (working, listening or offline) and when it last asked for work; this section does not show them yet.
The **Agents** page in the sidebar lists the same connections next to the Observe Agent and the service accounts, and each can switch between direct and propose mode there.
An Admin finds **Everyone's connected agents** under **Agent access**, with the person each belongs to, and can disconnect any of them there.
**Agent access** also links to API keys.
[Connect your own agent](mcp.md#connect-your-own-agent) describes how a client is approved.

## Change the theme

The app follows your system's light or dark setting until you choose one.

1. Open Settings and choose **Appearance** in its section list.
2. Pick **Light**, **Dark** or **Match the system** under **Theme**.
   The choice applies at once.

The account menu offers the same three choices under **Theme**, and the toolbar's **Switch theme** button and the command palette's **Switch theme** command flip between light and dark.
The choice stays in this browser's local storage and never reaches the server, so it does not follow you to another browser.

## Change the time zone

**Appearance** in Settings also has **Time zone**.
It is the zone a dashboard draws its times in when the dashboard does not set one.
It is the choice the time zone box next to the time range picker starts on and returns to.
The zone a dashboard sets itself, and a zone you pick in that box (which goes into the address as `tz`), win over it.

It is also the zone the **From** and **To** boxes of every time range picker read and show a typed time such as `2026-10-17 09:00` in.
On a dashboard they follow the dashboard's own zone and use this one only when it sets none.
It changes only how times are drawn and typed: a relative range such as `now/d` still resolves on the server in the dashboard's own zone, and the Logs and Traces pages keep showing times in your browser's zone.
The choice stays in this browser's local storage and never reaches the server, like the theme, so it does not follow you to another browser.

## Pick a time range

The time range button opens the quick ranges and the **From** and **To** boxes.

1. Choose **Pick dates** to show a calendar.
2. Click a day for that whole day, then click a second day for the days between them.
   The calendar fills **From** with the start of the first day and **To** with the end of the last, in the zone the boxes read.
3. Choose **Apply**.

A time or range the picker refuses is explained under the box it comes from, for example when the range is longer than 400 days.
On a dashboard, someone who may edit it also sees **Save as default**, which applies the range and saves it as the dashboard's default range in a new version.
The button is missing on an older version and while a layout or panel is being edited, and it is disabled for a range that already is the default.

## Reference

### Addresses

| Address | Opens |
| --- | --- |
| `/` | [Home](home.md): ask Observe Agent, start an observation or import a dashboard. |
| `/inbox` | Your inbox: what happened on your observations, dashboards and requests. |
| `/ask` | The chat: ask the Observe Agent for a dashboard. |
| `/chats` | Your chats with the Observe Agent. |
| `/observations` | The list of [observations](observations.md). |
| `/o/<observation id>` | One observation, its blocks and its discussion. |
| `/o/<observation id>/rca/<revision>` | A stored RCA revision inside the app. |
| `/proposals/<proposal id>` | One [proposal](../concepts/proposals.md) with its changes, Merge and Discard. |
| `/agents` | The Observe Agent, your connected agents and the service accounts that take runs. |
| `/agents/<ref>` | One of them. |
| `/dashboards` | The dashboards list. |
| `/d/<dashboard id>/<slug>` | A dashboard. |
| `/d/<dashboard id>/versions` | Its version history, headed "Versions of <title>". |
| `/f/<folder id>` | A folder. |
| `/c/<request id>` | The conversation of an agent request. |
| `/b/<request id>` | The live view of an agent request's build. |
| `/logs` | The list of log sources and saved logs searches. |
| `/logs/<datasource uid>` | The logs page for one source. |
| `/traces` | The list of trace sources and saved trace searches. |
| `/traces/<datasource uid>` | The trace search on one source. |
| `/explore/metrics/<datasource uid>` | The metrics page on one Prometheus source. |
| `/t/<trace id>?ds=<datasource uid>` | One trace in a Tempo, Jaeger or ClickHouse datasource. |
| `/search?q=<words>` | The Search page. |
| `/settings` | Settings; `#<section>` opens a section. |
| `/audit` | The Audit pages, for Admins. |
| `/datasources`, `/keys`, `/people` | Datasources, API keys and People, for Admins. |
| `/p/<token>` | A dashboard opened through a public link, with no sign-in. |
| `/rca/<token>` | A published RCA revision opened through its public link, with no sign-in; see [Share a revision](observations.md#share-a-revision). |
| `/connect/<request id>` | Approve or deny an MCP client that asked to act as you. |

#### Details of query strings

- `/`: `tab=observe` or `tab=import` opens that tab.
- `/inbox`: `view` and `item` reopen a view and an open item.
- `/o/<observation id>`: `rca=1` opens the Publish as RCA panel, and `thread=<thread id>` opens the discussion with that thread marked.
- `/proposals/<proposal id>`: `thread=<thread id>` marks that thread in the discussion.
- `/d/<dashboard id>/<slug>`: `thread=<thread id>` opens the activity pane on that comment thread.
- `/f/<folder id>`: the folder's page shows its dashboards with the same search and filters as the Dashboards list, its observations and its saved searches, and **Folder settings** for its name, access and deletion where you can manage it.
- `/chats`: `q` and `state` reopen a search, and `scope=all` opens an Admin's Everyone view.
- `/logs/<datasource uid>`: `q`, `ask`, `from` and `to` reopen a search or a question.
- `/traces/<datasource uid>`: `service`, `span`, `min`, `max`, `errors`, `q`, `ask`, `from` and `to` reopen a search or a question.
- `/explore/metrics/<datasource uid>`: `q`, `from`, `to`, `mode` and `ask` reopen a query, and `saved` and `observation` name the saved search or observation it belongs to.

### Old address forms

| Old address | Redirects to |
| --- | --- |
| `/dashboards/<id>` and `/dashboards/<id>/versions` | `/d/<id>` and `/d/<id>/versions` |
| `/folders/<id>` | `/f/<id>` |
| `/observe/<id>` | `/c/<id>` |
| `/builds/<id>` | `/b/<id>` |

The server answers these with `301 Moved Permanently`.
The web app also sends `/health` to `/dashboards?kind=any` and `/import/repair` to `/dashboards?kind=imported`, keeping `kind` and `folder`; these are not server redirects.

### Details of a row's More menu

On the dashboards list, a row's **More** menu has:

- **Duplicate...** asks for a title and a folder, from a list of the folders you may move it to with the current one marked, and **New folder** for an Editor or an Admin.
- **Delete dashboard...** asks for confirmation and needs Edit on the folder.

### Details of the More menu

The **More** menu of a dashboard has:

- **Edit details**, when you can edit the dashboard, changes the title, description and tags and saves them as a new version.
  It has a **Folder** menu of the shared folders you can edit.
  Choosing another one moves the dashboard, after the new version when you changed both, and a move alone saves no version.
- **Add panel**, when a datasource is configured, adds a panel; see [Edit a panel](edit-panel.md).
- **Edit Spec JSON** edits the whole Spec and saves it as a version; see [Versions](../concepts/versions.md).
- **Open on sign-in** makes the dashboard open in place of Home; see [Home dashboard](#home-dashboard).
  It reads **Stop opening on sign-in** when it already does.
- **Export spec JSON** downloads the dashboard's Spec as `<title>-v<version>.json`.
- **Dashboards as code** opens [Dashboards as code](dashboards-as-code.md) in a new tab.
- **Print or save as PDF** and **Download as image** lay the dashboard out for paper or draw it as one PNG; see [Print a dashboard or download it as an image](#print-a-dashboard-or-download-it-as-an-image).
- **Duplicate...** copies the dashboard under a title and into a folder you choose.
  The folder starts as the dashboard's own folder when you may create there, else your My Dashboards folder, else General.
- **TV mode**, also `d` then `k`, starts [TV mode](public-links.md#show-it-on-a-wall).
  **TV mode for this folder** cycles through the folder's dashboards and is left out in a personal folder.
- **Chart hover**, also Cmd/Ctrl+O, chooses how hovering one time chart shows on the others.
- **Delete dashboard...**, when you can edit the dashboard, asks for confirmation.

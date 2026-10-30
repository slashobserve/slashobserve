---
title: Import Grafana dashboards
description: Import a Grafana dashboard from the web page, the CLI or MCP, and how datasources and folders are matched.
---

You can bring a Grafana dashboard in three ways: the web page Import from Grafana, `observe import`, and the `import_grafana_dashboard` MCP tool.
The page and `observe import` call `POST /api/v1/imports` with the dashboard JSON as pasted or uploaded, the tool runs the same import, and `POST /api/v1/imports/preview` shows the result first without saving anything.
The JSON can be the classic dashboard model, an export that shares a dashboard externally, or the response of Grafana's dashboard API.

Each datasource the dashboard uses is matched to one of yours, in the order described under `grafanaUids` in the [datasources file](../reference/datasources-file.md).
An import cannot be saved while a datasource has no match, and SlashObserve never guesses between two datasources of the same type; give the match with the mapping that comes with the request, or add the Grafana uid to `grafanaUids`.

The dashboard goes into a folder, chosen in this order:

1. The request's `folder` title: `--folder` for `observe import`, `folder` for the API and the MCP tool.
   The web page sends the folder you pick in its **Folder** card, and sends none when the choice is **Automatic**.
2. The `meta.folderTitle` of an export taken from Grafana's dashboard API, which names the folder the dashboard had in Grafana.
3. Your default folder: the folder you last created a dashboard in, else General.

A title matches an existing folder without regard to case.
A title of `General`, or an export title that is empty or not a valid folder title, means the default; an invalid `folder` in the request is refused with `400 invalid_request`.
When no folder has the title, an Editor or Admin creates it.
When a folder has the title but you cannot create dashboards in it, the dashboard goes to your default folder instead, and the response carries a `folder_note` saying so (`observe import` prints it on standard error, even with `-q`).
The **Folder** card on the web page lists the folders where you may create dashboards, except General, because a title of `General` means your default folder, which **Automatic** already gives you.
After an import the page says "In folder `<title>`" and shows the `folder_note` when the dashboard did not go where the request or the export asked.
The folder is chosen before the dashboard is saved, so an import that then fails can leave a new, empty folder behind; delete it from its settings page.

The request body, with the dashboard JSON as a string, can be at most 8 MiB.
It saves the dashboard as version 1 and runs every query once.
Failing queries mark their panels and variables unverified in the import report instead of failing the import.
Later saves run every query too, but keep a failing panel or variable unverified, instead of failing, for as long as its queries and the variables they use are unchanged; a new or changed query must return data, and a panel whose queries pass again is no longer unverified.
The original JSON is stored with the import so a newer importer can convert it again, and it is never shown to the agent.

A query's min step written as a variable (`$name`, `${name}` or `[[name]]`) imports as `${name}`, so the query uses the step selected in that variable.
A `custom` variable of durations that is single-select, has no All and is used as a min step becomes an `interval` variable, and the import report says so.
A min step of `$__interval` or `$__rate_interval` imports as no minimum, and a resolution factor from 2 to 10 is kept as `intervalFactor`.
A dashboard's time settings carry over to the [dashboard fields](../concepts/observespec.md#the-dashboard) `timezone`, `weekStart`, `fiscalYearStartMonth` and `nowDelay` (from the time picker's now delay), and its time range may use `now/fy` and `now/fQ`.
A value SlashObserve does not support, such as a delay in days or a time zone name the server does not know, is left out and listed in the import report.

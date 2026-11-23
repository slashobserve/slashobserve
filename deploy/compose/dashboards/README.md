# Provisioned dashboards

The demo stack mounts `Demo` from this directory into the server at `/provision/Demo` and sets `SLASHOBSERVE_PROVISION_DIR` to `/provision`.
Each `.json` file here is an ObserveSpec document that the server saves as a dashboard at startup and again within 30 seconds of a change.
A file directly in this directory goes to General, and a file in a subdirectory goes to the folder named like that subdirectory.
Other files, this one included, are ignored.
A saved file runs every query like any other save, so a file whose queries fail keeps its error in `GET /api/v1/provision` and leaves its dashboard as it was.
See `docs/operate/provisioning.md` for the full rules.

## Reference dashboards

`Demo/astronomy-overview.json` goes to the folder Demo, and `SlashObserve/internals.json` to the folder SlashObserve, which the replicas stack loads because its Prometheus scrapes the servers.
They set the bar for how a SlashObserve dashboard looks: a headline line of stats, rows named by the question they answer, full lines of two, three, four or six panels, named series and a unit on every number.
`internal/provision/reference_test.go` checks those layout rules on every `.json` file in this directory, so `go test -short ./internal/provision/` fails on a file that breaks one.
A file's path is its identity, so renaming or moving one creates a new dashboard on every demo that already loaded the old path.
A panel over a series that exists only after its first event, such as agent runs on a server without a model or the first order of the Astronomy Shop, adds a query that ends in `or vector(0)`, because a provisioned file whose panel returns no data is not saved.
A file that fails this way is tried again every 30 seconds, so the dashboard appears when the stores have data.
`Demo/astronomy-overview.json` also holds one panel of every type the product draws, in the last four rows, so a new panel type is added there when it lands.
Its news panel needs github.com on the outbound allowlist and says so until an Admin allows it.

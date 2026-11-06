# Provisioned dashboards

The demo stack mounts this directory into the server at `/provision` and sets `SLASHOBSERVE_PROVISION_DIR` to it.
Each `.json` file here is an ObserveSpec document that the server saves as a dashboard at startup and again within 30 seconds of a change.
A file directly in this directory goes to General, and a file in a subdirectory goes to the folder named like that subdirectory.
Other files, this one included, are ignored.
A saved file runs every query like any other save, so a file whose queries fail keeps its error in `GET /api/v1/provision` and leaves its dashboard as it was.
See `docs/operate/provisioning.md` for the full rules.

## Reference dashboards

`Demo/shop-overview.json` and `Demo/node-health.json` go to the folder Demo, and `SlashObserve/internals.json` to the folder SlashObserve.
They set the bar for how a SlashObserve dashboard looks: a headline line of stats, rows named by the question they answer, full lines of two, three, four or six panels, named series and a unit on every number.
`internal/provision/reference_test.go` checks those layout rules on every `.json` file in this directory, so `go test -short ./internal/provision/` fails on a file that breaks one.
A file's path is its identity, so renaming or moving one creates a new dashboard on every demo that already loaded the old path.
A panel over a series that exists only after its first event, such as agent runs on a server without a model, adds a query that ends in `or vector(0)`, because a provisioned file whose panel returns no data is not saved.

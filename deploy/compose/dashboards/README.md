# Provisioned dashboards

The demo stack mounts this directory into the server at `/provision` and sets `SLASHOBSERVE_PROVISION_DIR` to it.
Each `.json` file here is an ObserveSpec document that the server saves as a dashboard at startup and again within 30 seconds of a change.
A file directly in this directory goes to General, and a file in a subdirectory goes to the folder named like that subdirectory.
Other files, this one included, are ignored.
A saved file runs every query like any other save, so a file whose queries fail keeps its error in `GET /api/v1/provision` and leaves its dashboard as it was.
See `docs/operate/provisioning.md` for the full rules.

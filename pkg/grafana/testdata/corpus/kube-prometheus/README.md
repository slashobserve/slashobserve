These JSON files are re-serialized from the `data` strings of the kube-prometheus ConfigMapList YAML, using `json.dumps` with `indent=2` and a trailing newline.
This changes their formatting from upstream, which is the change notice required by Apache-2.0 section 4(b).
See `../SOURCES.md` for the upstream commit.

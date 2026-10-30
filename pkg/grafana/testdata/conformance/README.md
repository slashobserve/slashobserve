The option conformance test checks that every catalogued Grafana option, set to a non-default value, converts.
`TestOptionConformance` and `TestConformanceCatalogs` in `pkg/grafana/conformance_test.go` read this directory.

## Licence

Grafana is AGPL-3.0.
A catalog holds Grafana option names (JSON keys) and option values (enum members, numbers, booleans, short strings) only.
It holds no Grafana description, comment, type definition or code.
Each catalog names the Grafana v12.3.0 files it was read from under `sources`.
Every `base.json` is authored in this repository under Apache-2.0.

## Layout

One directory per area.
An area name is lowercase `[a-z0-9-]+`, such as `dashboard`, `timeseries`, `fieldconfig` or `transformations`.

| File | Content |
| --- | --- |
| `base.json` | A minimal Grafana dashboard that imports fully. |
| `catalog.json` | The area's options with non-default values. |
| `seed.txt` | The ids that failed when the area was first catalogued, sorted, one per line. Never edited by later changes. |
| `gaps/<id>` | One empty file per known gap. |

## Catalog

`catalog.json` has `grafana` (always `v12.3.0`), `sources` (not empty), `root` and `entries`.
Unknown fields are refused.

`root` is the path inside `base.json` that every entry `path` and `with` key is relative to.
It is `""` for the dashboard, `panels[0]` for a panel, `panels[0].targets[0]` for a query, `templating.list[0]` for a variable and `panels[0].transformations[0]` for a transformation.

An entry has `id`, `path`, `value`, and optionally `with` and `expect`.

- `id` matches `^[A-Za-z0-9_.\[\]=:+-]{1,200}$` and is unique in the catalog.
  By convention it is `<path>=<value label>`, the label being the value or a short tag for objects and lists.
- `path` uses the importer's report syntax: keys joined by `.`, array indexes as `[n]`.
  Setting a path creates missing objects.
  An index may address an existing element or the next one, which appends.
  Any other index, a path through a scalar, and a key that contains `.` or `[` are catalog errors.
- `value` is the non-default value to set.
  Every enum member except the default gets its own entry, booleans get the negation of the default, numbers and strings get one representative value, and lists and objects get one representative value per distinct behaviour.
- `with` holds prerequisite settings, applied before `path` in key order.
  The entry's baseline is `base.json` plus `with`, without `path`.
  A `with` key equal to `path` is a catalog error.
- `expect` is one of the following.

| `expect` | The variant must |
| --- | --- |
| `convert` (default) | Import cleanly, and its Spec must differ from the baseline's. |
| `cosmetic` | Import cleanly with the baseline's Spec, and `isCosmetic(path, value)` must be true. Only for areas whose root is a panel, so a cosmetic option needs both a `cosmeticOptions` entry and a catalog line. |
| `bookkeeping` | Import cleanly with the baseline's Spec. For ids, versions, plugin versions and editor state with no behaviour; never for an option a Grafana user can see. |
| `excluded` | Not import cleanly. For the four milestone 2 exclusions (alert rules, annotations, Grafana's HTTP API, export); never a gap. |

Importing cleanly means `Convert` returns no error, the result is ready (every source datasource has a match), every panel line is `imported` and the report has no notes.

For an area whose root is a panel, a `convert` entry whose value is the importer's own Grafana default is a catalog error.

## Known gaps

An entry that does not pass yet has an empty file `gaps/<id>`.
An entry that fails without a gap file fails the test with the outcome.
An entry that passes while its gap file exists fails with the path to delete.

Gaps only shrink.
Every gap file must be listed in the area's `seed.txt`, and `seed.txt` is written once, by the seeding flag.
A new gap therefore cannot appear without an edit to `seed.txt` that a reviewer sees.
A change that converts an option deletes its gap file.
A change that adds entries to an area that already has a seed must make them pass.

## Adding an area

1. Write `base.json` and `catalog.json`.
2. Run `go test ./pkg/grafana -run TestConformanceCatalogs -count=1` until the catalog is well formed and the base imports cleanly.
3. Run `go test ./pkg/grafana -run TestOptionConformance -count=1 -conformance.seed` once.
   It writes `seed.txt` and the `gaps/` files for areas without a seed, logs `seed exists for <area>, nothing written` for the others, and never deletes a file.
4. Commit the area.

`go test ./pkg/grafana -run 'TestOptionConformance|TestConformanceCatalogs' -count=1 -v` logs one line per area, `<area>: <n> entries, <p> pass, <g> known gaps`, and a total.

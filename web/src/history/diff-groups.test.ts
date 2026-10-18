import { describe, expect, test } from "vitest"

import { groupChanges, panelTitles, type DiffChange } from "./diff-groups"

const titles = {
  from: { p: "Old P" },
  to: { p: "P", r: "Row", c: "Child" },
}

function group(changes: DiffChange[]) {
  return groupChanges(changes, titles).map((g) => ({
    key: g.key,
    label: g.label,
    fields: g.changes.map((c) => c.field),
  }))
}

describe("groupChanges", () => {
  test("groups a top-level panel change under the panel", () => {
    expect(
      group([{ op: "replace", path: "/panels/p/title", from: "a", to: "b" }])
    ).toEqual([{ key: "panel:p", label: 'Panel "P"', fields: ["title"] }])
  })

  test("files a row child change under the child", () => {
    expect(
      group([
        { op: "replace", path: "/panels/r/panels/c/title", from: "a", to: "b" },
      ])
    ).toEqual([{ key: "panel:c", label: 'Panel "Child"', fields: ["title"] }])
  })

  test("files an added row child under the child with no field", () => {
    expect(
      group([{ op: "add", path: "/panels/r/panels/c", to: { id: "c" } }])
    ).toEqual([{ key: "panel:c", label: 'Panel "Child"', fields: [""] }])
  })

  test("keeps the top-level panel order on the dashboard", () => {
    expect(
      group([{ op: "reorder", path: "/panels", from: ["a"], to: ["b"] }])
    ).toEqual([
      {
        key: "dashboard",
        label: "Dashboard settings",
        fields: ["panel order"],
      },
    ])
  })

  test("files a row's child order under the row", () => {
    expect(
      group([
        { op: "reorder", path: "/panels/r/panels", from: ["c"], to: ["d"] },
      ])
    ).toEqual([
      { key: "panel:r", label: 'Panel "Row"', fields: ["panel order"] },
    ])
  })

  test("names a query change by ref", () => {
    expect(
      group([
        {
          op: "replace",
          path: "/panels/p/queries/B/query",
          from: "a",
          to: "b",
        },
      ])
    ).toEqual([
      { key: "panel:p", label: 'Panel "P"', fields: ["queries.B.query"] },
    ])
  })

  test("names an override change by its match kind and value", () => {
    expect(
      group([
        {
          op: "replace",
          path: "/panels/p/overrides/query:B/unit",
          from: "s",
          to: "ms",
        },
        {
          op: "replace",
          path: "/panels/p/overrides/seriesRegex:api.v1~1.*/unit",
          from: "s",
          to: "ms",
        },
        {
          op: "add",
          path: "/panels/p/overrides/series:a~1b",
          to: { match: { series: "a/b" } },
        },
      ])
    ).toEqual([
      {
        key: "panel:p",
        label: 'Panel "P"',
        fields: [
          'override query "B" > unit',
          'override seriesRegex "api.v1/.*" > unit',
          'override series "a/b"',
        ],
      },
    ])
  })

  test("labels query and override reorders like panel order", () => {
    expect(
      group([
        { op: "reorder", path: "/panels/p/queries", from: ["A"], to: ["B"] },
        { op: "reorder", path: "/panels/p/overrides", from: ["a"], to: ["b"] },
      ])
    ).toEqual([
      {
        key: "panel:p",
        label: 'Panel "P"',
        fields: ["query order", "override order"],
      },
    ])
  })

  test("sorts an edited untitled panel with the present panels", () => {
    const spec = {
      panels: [
        { id: "t", type: "text" },
        { id: "s", title: "S" },
      ],
    }
    const present = panelTitles(spec)
    const labels = groupChanges(
      [
        { op: "remove", path: "/panels/gone/description" },
        { op: "replace", path: "/panels/t/content", from: "a", to: "b" },
        { op: "replace", path: "/panels/s/title", from: "a", to: "b" },
      ],
      { from: {}, to: present }
    ).map((g) => g.label)
    expect(labels).toEqual(['Panel "t"', 'Panel "S"', 'Panel "gone"'])
  })

  test("falls back to the old title and then the id", () => {
    expect(
      groupChanges(
        [
          { op: "remove", path: "/panels/p/description" },
          { op: "remove", path: "/panels/gone/description" },
        ],
        { from: { p: "Old P" }, to: {} }
      ).map((g) => g.label)
    ).toEqual(['Panel "Old P"', 'Panel "gone"'])
  })

  test("leaves variable and dashboard changes unchanged", () => {
    expect(
      group([
        { op: "replace", path: "/title", from: "a", to: "b" },
        { op: "replace", path: "/variables/pod/label", from: "a", to: "b" },
        { op: "reorder", path: "/variables", from: ["a"], to: ["b"] },
      ])
    ).toEqual([
      {
        key: "dashboard",
        label: "Dashboard settings",
        fields: ["title", "variable order"],
      },
      { key: "variable:pod", label: 'Variable "pod"', fields: ["label"] },
    ])
  })
})

describe("panelTitles", () => {
  test("records every panel id in document order, children included", () => {
    expect(
      Object.entries(
        panelTitles({
          panels: [
            { id: "a", title: "A" },
            { id: "r", panels: [{ id: "c" }] },
            { id: "t", type: "text" },
          ],
        })
      )
    ).toEqual([
      ["a", "A"],
      ["r", "r"],
      ["c", "c"],
      ["t", "t"],
    ])
  })

  test("ignores a non-dashboard value", () => {
    expect(panelTitles(null)).toEqual({})
  })
})

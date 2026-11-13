import { describe, expect, it } from "vitest"

import {
  DiscoverUrlError,
  importDiscoverUrl,
  importSummary,
  isDiscoverUrl,
} from "./discover-url"

const kibana = "https://kibana.example.com/app/discover#/?"

function url(base: string, state: Record<string, string>): string {
  return base + new URLSearchParams(state).toString()
}

const kuery = url(kibana, {
  _g: "(filters:!(),refreshInterval:(pause:!t,value:0),time:(from:now-15m,to:now))",
  _a: `(columns:!(_source),filters:!(),index:'abc',query:(language:kuery,query:'service.name : "checkout"'),sort:!())`,
})

function phrase(field: string, value: string) {
  return `(meta:(key:${field},negate:!f,params:(query:${value}),type:phrase),query:(match_phrase:(${field}:${value})))`
}

describe("isDiscoverUrl", () => {
  it.each([
    [kuery, true],
    [`  ${kuery}\n`, true],
    [
      "http://osd:5601/app/data-explorer/discover#?_a=(discover:(columns:!(a)))",
      true,
    ],
    ["https://kibana.example.com/s/ops/app/discover#/view/abc?_g=()", true],
    ["https://kibana.example.com/app/dashboards#/list?_g=()", false],
    ["service.name:checkout", false],
    ["https://kibana.example.com/app/discover", false],
  ])("is %j -> %s", (text, expected) => {
    expect(isDiscoverUrl(text)).toBe(expected)
  })
})

describe("importDiscoverUrl", () => {
  it("takes the query, the range and the columns of a kuery link", () => {
    const result = importDiscoverUrl(
      url(kibana, {
        _g: "(time:(from:now-15m,to:now))",
        _a: `(columns:!(message,_source,log.level),query:(language:kuery,query:'service.name : "checkout"'))`,
      })
    )
    expect(result).toEqual({
      q: 'service.name : "checkout"',
      from: "now-15m",
      to: "now",
      cols: ["message", "log.level"],
      pills: [],
      dropped: 0,
    })
  })

  it("prefixes a lucene query", () => {
    expect(
      importDiscoverUrl(
        url(kibana, {
          _a: "(query:(language:lucene,query:'level:error AND host:a'))",
        })
      ).q
    ).toBe("lucene: level:error AND host:a")
  })

  it("keeps an absolute time range and drops a half range", () => {
    const absolute = importDiscoverUrl(
      url(kibana, {
        _g: "(time:(from:'2026-11-07T10:00:00.000Z',to:'2026-11-07T11:00:00.000Z'))",
      })
    )
    expect([absolute.from, absolute.to]).toEqual([
      "2026-11-07T10:00:00.000Z",
      "2026-11-07T11:00:00.000Z",
    ])
    const half = importDiscoverUrl(url(kibana, { _g: "(time:(from:now-1h))" }))
    expect([half.from, half.to]).toEqual([undefined, undefined])
  })

  it("maps phrase and phrases filters and counts the others as dropped", () => {
    const result = importDiscoverUrl(
      url(kibana, {
        _a:
          "(filters:!(" +
          [
            phrase("service.name", "checkout"),
            "(meta:(disabled:!t,key:log.level,negate:!f,params:!(error,warn),type:phrases))",
            "(meta:(key:host.name,negate:!t,params:(query:canary),type:phrase))",
            "(meta:(key:bytes,negate:!f,params:(gte:10),type:range))",
            "(meta:(key:retry,negate:!f,type:exists))",
            "(meta:(negate:!f,type:custom),query:(bool:()))",
          ].join(",") +
          "))",
      })
    )
    expect(result.pills).toEqual([
      {
        kind: "is",
        field: "service.name",
        value: "checkout",
        negate: false,
        pinned: false,
        paused: false,
      },
      {
        kind: "oneOf",
        field: "log.level",
        values: ["error", "warn"],
        negate: false,
        pinned: false,
        paused: true,
      },
      {
        kind: "is",
        field: "host.name",
        value: "canary",
        negate: true,
        pinned: false,
        paused: false,
      },
    ])
    expect(result.dropped).toBe(3)
  })

  it("reads the OpenSearch Dashboards shape with the query in _q", () => {
    const result = importDiscoverUrl(
      "http://osd:5601/app/data-explorer/discover#?" +
        new URLSearchParams({
          _a: "(discover:(columns:!(_source,message)),metadata:(view:discover))",
          _g: "(time:(from:now-1h,to:now))",
          _q: "(filters:!(),query:(language:kuery,query:'level:error'))",
        }).toString()
    )
    expect(result.q).toBe("level:error")
    expect(result.cols).toEqual(["message"])
    expect(result.from).toBe("now-1h")
  })

  it("leaves a field out when the link does not carry it", () => {
    const result = importDiscoverUrl(url(kibana, { _g: "()" }))
    expect(result).toEqual({ pills: [], dropped: 0 })
  })

  it("throws for a link that is not a Discover link", () => {
    expect(() => importDiscoverUrl("https://example.com/a?b=c")).toThrow(
      DiscoverUrlError
    )
  })

  it("throws when the state is not valid rison", () => {
    expect(() => importDiscoverUrl(url(kibana, { _a: "(query:('x)" }))).toThrow(
      DiscoverUrlError
    )
  })
})

describe("importSummary", () => {
  const pill = {
    kind: "is" as const,
    field: "a",
    value: "b",
    negate: false,
    pinned: false,
    paused: false,
  }

  it("lists what was filled and what was not applied", () => {
    expect(
      importSummary({
        q: "a",
        from: "now-1h",
        to: "now",
        cols: ["x", "y"],
        pills: [pill, pill, pill],
        dropped: 1,
      })
    ).toBe(
      "Filled from a Discover link: query, time range, 2 columns, 3 filters. 1 filter from the URL was not applied."
    )
  })

  it("uses the singular and the plural of the dropped filters", () => {
    expect(importSummary({ q: "a", pills: [pill], dropped: 2 })).toBe(
      "Filled from a Discover link: query, 1 filter. 2 filters from the URL were not applied."
    )
  })

  it("says so when nothing could be used", () => {
    expect(importSummary({ pills: [], dropped: 0 })).toBe(
      "Nothing in that Discover link could be used."
    )
  })
})

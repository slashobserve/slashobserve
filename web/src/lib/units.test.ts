import { describe, expect, it } from "vitest"

import {
  axisDecimals,
  commonDecimals,
  decimalsByUnit,
  formatValue,
  formatValue as f,
  formatValueParts,
  ownDecimals,
  parseUnit,
} from "./units"

describe("formatValueParts", () => {
  it.each([
    ["reqps", 12.5, ["", "12.5", " req/s"]],
    ["percent", 42, ["", "42", "%"]],
    ["prefix:$", 3, ["$", "3", ""]],
    ["bytes", 1536, ["", "1.5", " KiB"]],
    ["short", 1234567, ["", "1.23", " Mil"]],
    ["none", 7, ["", "7", ""]],
    ["dtdurations", 3723, ["", "1h 2m 3s", ""]],
  ])("splits %s into prefix, amount and suffix", (unit, value, parts) => {
    const { prefix, amount, suffix } = formatValueParts(value, unit)
    expect([prefix, amount, suffix]).toEqual(parts)
  })

  it("keeps an absent value whole", () => {
    expect(formatValueParts(null, "reqps")).toEqual({
      prefix: "",
      amount: "-",
      suffix: "",
    })
  })
})

describe("formatValue", () => {
  it("formats the reference examples", () => {
    expect(
      [
        f(1536, "bytes"),
        f(1500, "decbytes"),
        f(0.1234, "percentunit"),
        f(12.5, "reqps"),
        f(0.25, "s"),
        f(1500, "ms"),
        f(3723, "dtdurations"),
        f(1234567, "short"),
        f(null, "s"),
        f(2, "none"),
        f(1, "bool_yes_no"),
      ].join("|")
    ).toBe(
      "1.5 KiB|1.5 kB|12.34%|12.5 req/s|250 ms|1.5 s|1h 2m 3s|1.23 Mil|-|2|Yes"
    )
  })

  it("handles missing and non-finite values", () => {
    expect(f(undefined)).toBe("-")
    expect(f(Number.NaN, "bytes")).toBe("-")
    expect(f(Infinity)).toBe("+Inf")
    expect(f(-Infinity, "s")).toBe("-Inf")
  })

  it("drops trailing zeros and respects explicit decimals", () => {
    expect(f(3)).toBe("3")
    expect(f(1.5)).toBe("1.5")
    expect(f(1234567.891)).toBe("1,234,567.89")
    expect(f(0.004)).toBe("0.004")
    expect(f(0, "percentunit", 2)).toBe("0.00%")
    expect(f(-0.001, "none", 1)).toBe("0.0")
  })

  it("scales binary and decimal byte units from their own step", () => {
    expect(f(512, "bytes")).toBe("512 B")
    expect(f(2048, "kbytes")).toBe("2 MiB")
    expect(f(5, "kbytes")).toBe("5 KiB")
    expect(f(1_500_000, "decbytes")).toBe("1.5 MB")
    expect(f(1024 * 1024, "bits")).toBe("1 Mib")
  })

  it("formats rates", () => {
    expect(f(1500, "bps")).toBe("1.5 kb/s")
    expect(f(2048, "binBps")).toBe("2 KiB/s")
    expect(f(3, "MiBs")).toBe("3 MiB/s")
    expect(f(1500, "reqps")).toBe("1.5 K req/s")
    expect(f(30, "cpm")).toBe("30 c/min")
  })

  it("picks the largest duration unit at least 1", () => {
    expect(f(0, "s")).toBe("0 s")
    expect(f(0.0005, "s")).toBe("500 µs")
    expect(f(90, "s")).toBe("1.5 min")
    expect(f(36, "h")).toBe("1.5 day")
    expect(f(1, "d")).toBe("1 day")
  })

  it("formats human durations and clocks", () => {
    expect(f(0, "dtdurations")).toBe("0s")
    expect(f(3_723_004, "dtdurationms")).toBe("1h 2m 3s")
    expect(f(90_061, "dtdurations")).toBe("1d 1h 1m")
    expect(f(3723, "clocks")).toBe("01:02:03")
    expect(f(3_723_000, "clockms")).toBe("01:02:03")
  })

  it("formats dates", () => {
    const ms = new Date(2026, 9, 17, 9, 5, 7).getTime()
    expect(f(ms, "dateTimeAsIso")).toBe("2026-10-17 09:05:07")
    expect(f(Date.now(), "dateTimeAsIsoNoDateIfToday")).toMatch(
      /^\d{2}:\d{2}:\d{2}$/
    )
    expect(f(Date.now() - 3 * 3_600_000, "dateTimeFromNow")).toBe("3 hours ago")
    expect(f(1e17, "dateTimeFromNow")).toBe("-")
    expect(f(1e17, "dateTimeAsIso")).toBe("-")
  })

  it("formats booleans, hex, scientific and plain strings", () => {
    expect(f(0, "bool")).toBe("false")
    expect(f(2, "bool_on_off")).toBe("On")
    expect(f(255, "hex")).toBe("FF")
    expect(f(255, "hex0x")).toBe("0xFF")
    expect(f(12345, "sci")).toBe("1.23e+4")
    expect(f(12.5, "string")).toBe("12.5")
  })

  it("formats currency and physical units", () => {
    expect(f(1234.5, "currencyUSD")).toBe("$1,234.50")
    expect(f(25_000, "currencyUSD")).toBe("$25K")
    expect(f(1500, "watt")).toBe("1.5 kW")
    expect(f(2_400_000_000, "hertz")).toBe("2.4 GHz")
    expect(f(21.5, "celsius")).toBe("21.5 °C")
    expect(f(45, "humidity")).toBe("45 %H")
    expect(f(7, "dthms")).toBe("7 dthms")
  })

  it("scales short and SI short numbers", () => {
    expect(f(999, "short")).toBe("999")
    expect(f(1500, "sishort")).toBe("1.5 k")
    expect(f(5e12, "short")).toBe("5 Tri")
  })

  it("exposes the axis formatter with the same rules", () => {
    expect(formatValue(1536, "bytes")).toBe("1.5 KiB")
  })
})

describe("axisDecimals", () => {
  const labels = (splits: number[], unit?: Parameters<typeof f>[1]) =>
    splits.map((split) => f(split, unit, axisDecimals(splits, unit)))

  it("keeps neighbouring splits distinguishable at large magnitude", () => {
    const gb = [15.425, 15.43, 15.435].map((value) => value * 1e9)
    expect(labels(gb, "decbytes")).toEqual([
      "15.425 GB",
      "15.430 GB",
      "15.435 GB",
    ])
    expect(labels([1000, 1000.2, 1000.4, 1000.6])).toEqual([
      "1,000.0",
      "1,000.2",
      "1,000.4",
      "1,000.6",
    ])
  })

  it("does not widen splits that are already distinct", () => {
    expect(labels([0, 500, 1000, 1500])).toEqual(["0", "500", "1,000", "1,500"])
    expect(labels([0, 0.5, 1], "reqps")).toEqual([
      "0.0 req/s",
      "0.5 req/s",
      "1.0 req/s",
    ])
  })

  it("keeps explicit decimals", () => {
    expect(axisDecimals([1000, 1000.2], undefined, 0)).toBe(0)
  })
})

describe("commonDecimals", () => {
  const column = (values: (number | null)[], unit?: Parameters<typeof f>[1]) =>
    values.map((value) => f(value, unit, commonDecimals(values, unit)))

  it("gives every value in a column the same number of decimals", () => {
    expect(column([2.4, 2.45, 12], "reqps")).toEqual([
      "2.40 req/s",
      "2.45 req/s",
      "12.00 req/s",
    ])
    expect(column([1, 2, 30])).toEqual(["1", "2", "30"])
  })

  it("counts decimals after scaling the value for its unit", () => {
    expect(column([0.1234, 0.5], "percentunit")).toEqual(["12.34%", "50.00%"])
    expect(column([1536, 2048], "bytes")).toEqual(["1.5 KiB", "2.0 KiB"])
    expect(column([0, 1.5], "s")).toEqual(["0.0 s", "1.5 s"])
  })

  it("widens for small values and ignores missing or non-finite ones", () => {
    expect(column([0.5, 0.004])).toEqual(["0.500", "0.004"])
    expect(column([null, Infinity, 3.5, 4])).toEqual([
      "-",
      "+Inf",
      "3.5",
      "4.0",
    ])
    expect(commonDecimals([null, Number.NaN])).toBeUndefined()
    expect(commonDecimals([])).toBeUndefined()
  })

  it("rounds small values next to large ones to about four significant figures of the largest", () => {
    expect(column([0.0017, 1234])).toEqual(["0", "1,234"])
    expect(column([0.0017, 12.5])).toEqual(["0.00", "12.50"])
    expect(column([0.0017, 0.5])).toEqual(["0.0017", "0.5000"])
    expect(column([0.002, 3], "reqps")).toEqual(["0.002 req/s", "3.000 req/s"])
    expect(column([0, 0])).toEqual(["0", "0"])
    expect(column([0.0017, 1234], "percentunit")).toEqual(["0%", "123,400%"])
  })

  it("keeps explicit decimals and leaves non-numeric units alone", () => {
    expect(commonDecimals([2.4, 2.45], "reqps", 1)).toBe(1)
    expect(commonDecimals([2.4, 2.45], "bool")).toBeUndefined()
    expect(commonDecimals([1, 2], "dateTimeAsIso")).toBeUndefined()
  })
})

describe("decimalsByUnit", () => {
  it("shares one decimal count per unit", () => {
    const shared = decimalsByUnit([
      { unit: "reqps", value: 2.4 },
      { unit: "reqps", value: 2.45 },
      { unit: "bytes", value: 1536 },
      { unit: "bytes", value: 2048 },
    ])
    expect(shared.get("reqps")).toBe(2)
    expect(shared.get("bytes")).toBe(1)
  })

  it("keeps values with their own decimals out of the count", () => {
    const shared = decimalsByUnit([
      { unit: "reqps", value: 2.4 },
      { unit: "reqps", decimals: 4, value: 2.4567 },
    ])
    expect(shared.get("reqps")).toBe(1)
  })

  it("groups values without a unit and tolerates missing values", () => {
    const shared = decimalsByUnit([
      { value: 1.5 },
      { value: null },
      { value: 2 },
    ])
    expect(shared.get(undefined)).toBe(1)
    expect(shared.get("reqps")).toBeUndefined()
  })

  it("leaves a unit with no countable value undefined", () => {
    const shared = decimalsByUnit([{ unit: "reqps", value: null }])
    expect(shared.has("reqps")).toBe(true)
    expect(shared.get("reqps")).toBeUndefined()
  })
})

describe("custom units", () => {
  it("formats suffix units with a space before the text", () => {
    expect(f(12, "suffix:req")).toBe("12 req")
    expect(f(12.5, "suffix: req", 1)).toBe("12.5 req")
  })

  it("formats prefix units with the text right before the number", () => {
    expect(f(3, "prefix:$")).toBe("$3")
    expect(f(-3, "prefix:$")).toBe("$-3")
    expect(f(-2.5, "prefix:$", 2)).toBe("$-2.50")
  })

  it("formats large values with a prefix unit", () => {
    expect(f(1234567.891, "prefix:$")).toBe("$1,234,567.89")
    expect(f(1e12, "prefix:$")).toBe("$1,000,000,000,000")
  })

  it("formats unknown units as plain numbers", () => {
    expect(f(3, "bogus")).toBe("3")
    expect(f(3, "suffix:")).toBe("3")
    expect(f(3, "constructor")).toBe("3")
  })

  it("shares decimals across prefixed values", () => {
    expect(commonDecimals([2.4, 2.45], "prefix:$")).toBe(2)
    expect(commonDecimals([2.4, 2.45], "suffix:req")).toBe(2)
  })

  it("derives axis decimals for custom units like plain ones", () => {
    const splits = [0, 0.5, 1, 1.5]
    expect(axisDecimals(splits, "prefix:$")).toBe(axisDecimals(splits, "none"))
    expect(axisDecimals(splits, "suffix:req")).toBe(
      axisDecimals(splits, "none")
    )
    expect(axisDecimals([1000, 1000.2], "prefix:$")).toBe(1)
  })

  it("derives own decimals and axis labels for custom units", () => {
    expect(ownDecimals(2.5, "prefix:$")).toBe(1)
    expect(ownDecimals(3, "suffix:req")).toBe(0)
    expect(formatValue(2.5, "prefix:$", 2)).toBe("$2.50")
    expect(formatValue(12, "suffix:req")).toBe("12 req")
  })

  it("parses ids, custom units and unknown units", () => {
    expect(parseUnit(undefined)).toEqual({ kind: "id", id: "none" })
    expect(parseUnit("")).toEqual({ kind: "id", id: "none" })
    expect(parseUnit("ms")).toEqual({ kind: "id", id: "ms" })
    expect(parseUnit("suffix: req ")).toEqual({ kind: "suffix", text: "req" })
    expect(parseUnit("prefix: $")).toEqual({ kind: "prefix", text: " $" })
    expect(parseUnit("bogus")).toEqual({ kind: "id", id: "none" })
  })
})

describe("scaled SI units", () => {
  it.each([
    [1500, "flops", "1.5 kFLOPS"],
    [1500, "mflops", "1.5 GFLOPS"],
    [0.5, "mflops", "500 kFLOPS"],
    [0.002, "farad", "2 mF"],
    [1500, "µfarad", "1.5 mF"],
    [2500, "ffarad", "2.5 pF"],
    [2200, "ohm", "2.2 kΩ"],
    [0.5, "kohm", "500 Ω"],
    [1500, "massmg", "1.5 g"],
    [2000, "masst", "2 kt"],
    [1500, "radusv", "1.5 mSv"],
    [2e9, "Hs", "2 GH/s"],
    [1500, "KHs", "1.5 MH/s"],
    [0.003, "litre", "3 mL"],
    [5, "pressurebar", "5 bar"],
    [0, "farad", "0 F"],
    [-2500, "rothz", "-2.5 kHz"],
  ])("writes %s with %s as %s", (value, unit, expected) => {
    expect(f(value, unit)).toBe(expected)
  })
})

describe("fixed and prefixed units", () => {
  it.each([
    [45, "degree", "45°"],
    [29.92, "pressurehg", '29.92 "Hg'],
    [3000, "rotrpm", "3,000 rpm"],
    [9.81, "accMS2", "9.81 m/sec²"],
    [12, "velocitykmh", "12 km/h"],
    [4, "areaKM2", "4 km²"],
    [7, "conmmolL", "7 mmol/L"],
    [7, "conμgm3", "7 μg/m³"],
    [0.5, "currencyBTC", "฿0.5"],
    [2, "currencymBTC", "mBTC2"],
    [1.5, "PiBs", "1.5 PiB/s"],
    [1.5, "Pbits", "1.5 Pb/s"],
  ])("writes %s with %s as %s", (value, unit, expected) => {
    expect(f(value, unit)).toBe(expected)
  })

  it("writes ISO currencies through Intl", () => {
    expect(f(12.5, "currencyRUB")).toContain("12.50")
    expect(f(12.5, "currencyCHF")).toContain("12.50")
    expect(f(25_000, "currencySEK")).toContain("25K")
  })
})

describe("custom unit forms", () => {
  it.each([
    [1500, "count:req", "1.5 K req"],
    [5, "count:req", "5 req"],
    [2_500_000, "count: req ", "2.5 Mil req"],
    [1500, "currency:€", "€1.5 K"],
    [12, "currency:€", "€12"],
    [2000, "si:mF", "2 F"],
    [5, "si:mF", "5 mF"],
    [0.002, "si:F", "2 mF"],
    [1500, "si:Hz", "1.5 kHz"],
    [5, "si:μF", "5 µF"],
    [3, "si:rpm", "3 rpm"],
  ])("writes %s with %s as %s", (value, unit, expected) => {
    expect(f(value, unit)).toBe(expected)
  })

  it("writes a time unit as a date in the local time zone", () => {
    const ms = new Date(2026, 9, 7, 15, 4, 9).getTime()
    expect(f(ms, "time:YYYY-MM-DD HH:mm:ss")).toBe("2026-10-07 15:04:09")
    expect(f(ms, "time: YYYY-MM-DD HH:mm:ss")).toBe("2026-10-07 15:04:09")
    expect(f(ms, "time:hh:mm A")).toBe("03:04 PM")
  })

  it("keeps date units out of decimals and scaling", () => {
    const ms = new Date(2026, 9, 7).getTime()
    expect(f(ms, "time:D/M/YY", 3)).toBe("7/10/26")
    expect(commonDecimals([ms], "time:YYYY")).toBeUndefined()
  })

  it("parses every custom kind and treats empty text as plain", () => {
    expect(parseUnit("time: YYYY ")).toEqual({ kind: "time", text: "YYYY" })
    expect(parseUnit("si:mF")).toEqual({ kind: "si", text: "mF" })
    expect(parseUnit("count:req")).toEqual({ kind: "count", text: "req" })
    expect(parseUnit("currency:€")).toEqual({ kind: "currency", text: "€" })
    for (const kind of ["time", "si", "count", "currency", "suffix"]) {
      expect(parseUnit(`${kind}: `), kind).toEqual({ kind: "id", id: "none" })
    }
  })
})

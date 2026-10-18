import { describe, expect, it } from "vitest"

import { formatAxisValue, formatValue as f } from "./units"

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
    expect(formatAxisValue(1536, "bytes")).toBe("1.5 KiB")
  })
})

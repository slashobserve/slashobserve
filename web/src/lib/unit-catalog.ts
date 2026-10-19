import { parseUnit, type UnitId } from "./units"

export type UnitGroup = {
  label: string
  units: { id: UnitId; label: string }[]
}

type Entry = readonly [id: UnitId, name: string]

function group(label: string, entries: readonly Entry[]): UnitGroup {
  return {
    label,
    units: entries.map(([id, name]) => ({
      id,
      label: id === "none" ? name : `${name} (${id})`,
    })),
  }
}

export const unitGroups: readonly UnitGroup[] = [
  group("Misc", [
    ["none", "None"],
    ["short", "short"],
    ["sishort", "SI short"],
    ["percent", "percent, 0 to 100"],
    ["percentunit", "percent, 0.0 to 1.0"],
    ["humidity", "humidity"],
    ["dB", "decibel"],
    ["hex0x", "hexadecimal with 0x"],
    ["hex", "hexadecimal"],
    ["sci", "scientific notation"],
    ["locale", "locale format"],
    ["pixel", "pixels"],
    ["string", "string"],
    ["bool", "true or false"],
    ["bool_yes_no", "yes or no"],
    ["bool_on_off", "on or off"],
  ]),
  group("Data", [
    ["bits", "bits, IEC"],
    ["bytes", "bytes, IEC"],
    ["kbytes", "kibibytes"],
    ["mbytes", "mebibytes"],
    ["gbytes", "gibibytes"],
    ["tbytes", "tebibytes"],
    ["pbytes", "pebibytes"],
    ["decbits", "bits, SI"],
    ["decbytes", "bytes, SI"],
    ["deckbytes", "kilobytes"],
    ["decmbytes", "megabytes"],
    ["decgbytes", "gigabytes"],
    ["dectbytes", "terabytes"],
    ["decpbytes", "petabytes"],
  ]),
  group("Data rate", [
    ["pps", "packets per second"],
    ["binBps", "bytes per second, IEC"],
    ["Bps", "bytes per second, SI"],
    ["binbps", "bits per second, IEC"],
    ["bps", "bits per second, SI"],
    ["KiBs", "kibibytes per second"],
    ["Kibits", "kibibits per second"],
    ["KBs", "kilobytes per second"],
    ["Kbits", "kilobits per second"],
    ["MiBs", "mebibytes per second"],
    ["Mibits", "mebibits per second"],
    ["MBs", "megabytes per second"],
    ["Mbits", "megabits per second"],
    ["GiBs", "gibibytes per second"],
    ["Gibits", "gibibits per second"],
    ["GBs", "gigabytes per second"],
    ["Gbits", "gigabits per second"],
    ["TiBs", "tebibytes per second"],
    ["Tibits", "tebibits per second"],
    ["TBs", "terabytes per second"],
    ["Tbits", "terabits per second"],
  ]),
  group("Throughput", [
    ["cps", "counts per second"],
    ["ops", "operations per second"],
    ["reqps", "requests per second"],
    ["rps", "reads per second"],
    ["wps", "writes per second"],
    ["iops", "I/O operations per second"],
    ["eps", "events per second"],
    ["mps", "messages per second"],
    ["recps", "records per second"],
    ["rowsps", "rows per second"],
    ["cpm", "counts per minute"],
    ["opm", "operations per minute"],
    ["rpm", "reads per minute"],
    ["wpm", "writes per minute"],
  ]),
  group("Time", [
    ["hertz", "hertz"],
    ["ns", "nanoseconds"],
    ["µs", "microseconds"],
    ["ms", "milliseconds"],
    ["s", "seconds"],
    ["m", "minutes"],
    ["h", "hours"],
    ["d", "days"],
    ["dtdurationms", "duration from milliseconds"],
    ["dtdurations", "duration from seconds"],
    ["dthms", "duration as hh:mm:ss"],
    ["dtdhms", "duration as d hh:mm:ss"],
    ["timeticks", "timeticks, 1/100 s"],
    ["clockms", "clock from milliseconds"],
    ["clocks", "clock from seconds"],
  ]),
  group("Date and time", [
    ["dateTimeAsIso", "date and time, ISO"],
    ["dateTimeAsIsoNoDateIfToday", "date and time, ISO, time only today"],
    ["dateTimeAsUS", "date and time, US"],
    ["dateTimeAsUSNoDateIfToday", "date and time, US, time only today"],
    ["dateTimeAsLocal", "date and time, local"],
    ["dateTimeAsLocalNoDateIfToday", "date and time, local, time only today"],
    ["dateTimeAsSystem", "date and time, browser default"],
    ["dateTimeFromNow", "time from now"],
  ]),
  group("Energy", [
    ["watt", "watts"],
    ["kwatt", "kilowatts"],
    ["mwatt", "milliwatts"],
    ["watth", "watt-hours"],
    ["kwatth", "kilowatt-hours"],
    ["joule", "joules"],
    ["amp", "amperes"],
    ["mamp", "milliamperes"],
    ["volt", "volts"],
    ["mvolt", "millivolts"],
    ["dBm", "decibel-milliwatts"],
  ]),
  group("Temperature", [
    ["celsius", "degrees Celsius"],
    ["fahrenheit", "degrees Fahrenheit"],
    ["kelvin", "kelvin"],
  ]),
  group("Currency", [
    ["currencyUSD", "US dollars"],
    ["currencyEUR", "euros"],
    ["currencyGBP", "pounds sterling"],
    ["currencyJPY", "yen"],
    ["currencyINR", "rupees"],
  ]),
  group("Length", [
    ["lengthmm", "millimeters"],
    ["lengthm", "meters"],
    ["lengthkm", "kilometers"],
  ]),
  group("Mass", [
    ["massg", "grams"],
    ["masskg", "kilograms"],
  ]),
]

const labels = new Map(
  unitGroups.flatMap((entry) =>
    entry.units.map((unit) => [unit.id, unit.label] as const)
  )
)

export function unitLabel(unit: string | undefined): string {
  const parsed = parseUnit(unit)
  switch (parsed.kind) {
    case "suffix":
      return `Custom suffix "${parsed.text}"`
    case "prefix":
      return `Custom prefix "${parsed.text}"`
    case "id":
      return labels.get(parsed.id) ?? "None"
  }
}

export type Level = "debug" | "info" | "warn" | "error"
export type Field = string | number | boolean
export type Fields = Readonly<Record<string, Field>>
export type Logger = (level: Level, msg: string, fields?: Fields) => void

const rank: Record<Level, number> = { debug: 0, info: 1, warn: 2, error: 3 }
const reserved = new Set(["time", "level", "msg", "component", "truncated"])

export function parseLevel(text: string | undefined): Level | undefined {
  const level = text?.trim().toLowerCase()
  return level !== undefined && level in rank ? (level as Level) : undefined
}

export function createLogger(
  write: (line: string) => void,
  threshold: Level,
  now: () => Date = () => new Date()
): Logger {
  return (level, msg, fields) => {
    if (rank[level] < rank[threshold]) {
      return
    }
    const line: Record<string, Field> = {
      time: now().toISOString(),
      level,
      msg,
    }
    for (const [key, value] of Object.entries(fields ?? {})) {
      if (!reserved.has(key)) {
        line[key] = value
      }
    }
    write(JSON.stringify(line))
  }
}

let current: Logger = createLogger(
  (line) => process.stderr.write(line + "\n"),
  "info"
)

export function configureLog(threshold: Level): void {
  current = createLogger((line) => process.stderr.write(line + "\n"), threshold)
}

export function log(level: Level, msg: string, fields?: Fields): void {
  current(level, msg, fields)
}

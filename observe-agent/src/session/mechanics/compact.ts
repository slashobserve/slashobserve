export type Weighed = { assistant: boolean; bytes: number }

export function utf8Bytes(text: string): number {
  return Buffer.byteLength(text)
}

function isContinuation(byte: number | undefined): boolean {
  return byte !== undefined && (byte & 0xc0) === 0x80
}

export function truncateUtf8(content: string, limit: number): string {
  const bytes = Buffer.from(content)
  if (bytes.length <= limit) {
    return content
  }
  let end = limit
  while (end > 0 && isContinuation(bytes[end])) {
    end--
  }
  return bytes.subarray(0, end).toString()
}

export function cutResult(content: string, limit: number): string {
  const size = utf8Bytes(content)
  if (limit <= 0 || size <= limit) {
    return content
  }
  return `${truncateUtf8(content, limit)}\n[cut from ${size} bytes; call the tool again for the full result]`
}

export function keptBytes(size: number, limit: number): number {
  return limit <= 0 ? size : Math.min(size, limit)
}

function lastAssistant(messages: Weighed[], before: number): number {
  for (let i = before - 1; i >= 0; i--) {
    if (messages[i]?.assistant) {
      return i
    }
  }
  return -1
}

function sumBytes(messages: Weighed[]): number {
  return messages.reduce((total, message) => total + message.bytes, 0)
}

export function tailStart(
  messages: Weighed[],
  tailTurns: number,
  tailBytes: number
): number {
  const last = lastAssistant(messages, messages.length)
  if (last < 0) {
    return messages.length
  }
  let start = last
  let kept = 1
  let size = sumBytes(messages.slice(last))
  while (kept < tailTurns) {
    const previous = lastAssistant(messages, start)
    if (previous < 0) {
      break
    }
    const turn = sumBytes(messages.slice(previous, start))
    if (size + turn > tailBytes) {
      break
    }
    start = previous
    kept++
    size += turn
  }
  return start
}

export const prunedResult =
  "[earlier result left out to keep the conversation short; call the tool again if you need it]"

export type PruneResult = { id: string; bytes: number }

export type PruneMessage = { assistant: boolean; results: PruneResult[] }

export type PruneConfig = { keepTurns: number; stepBytes: number }

export type Prunable = { callId: string; prunable: boolean }

function keptFrom(
  messages: PruneMessage[],
  keepTurns: number
): number | undefined {
  let seen = 0
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i]?.assistant) {
      seen++
      if (seen === keepTurns) {
        return i
      }
    }
  }
  return undefined
}

export class Pruner {
  private readonly config: PruneConfig
  private readonly prunableIds = new Set<string>()
  private boundaryAt = 0

  constructor(config: PruneConfig) {
    this.config = config
  }

  note(results: Prunable[]): void {
    for (const result of results) {
      if (result.prunable) {
        this.prunableIds.add(result.callId)
      }
    }
  }

  reset(): void {
    this.boundaryAt = 0
  }

  prunable(id: string): boolean {
    return this.prunableIds.has(id)
  }

  boundary(messages: PruneMessage[]): number {
    if (this.config.stepBytes <= 0 || this.config.keepTurns <= 0) {
      return 0
    }
    const cut = keptFrom(messages, this.config.keepTurns)
    if (cut === undefined) {
      return this.boundaryAt
    }
    let candidates = 0
    for (const message of messages.slice(Math.min(this.boundaryAt, cut), cut)) {
      for (const result of message.results) {
        if (this.prunableIds.has(result.id)) {
          candidates += result.bytes
        }
      }
    }
    if (candidates >= this.config.stepBytes) {
      this.boundaryAt = cut
    }
    return this.boundaryAt
  }
}

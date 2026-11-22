const encoder = new TextEncoder()

export class FakeStream {
  readonly response: Response
  private controller!: ReadableStreamDefaultController<Uint8Array>

  constructor() {
    this.response = new Response(
      new ReadableStream<Uint8Array>({
        start: (controller) => {
          this.controller = controller
        },
      }),
      { headers: { "content-type": "text/event-stream" } }
    )
  }

  write(text: string) {
    this.controller.enqueue(encoder.encode(text))
  }

  frame(event: string, data: unknown, id?: string) {
    const idLine = id === undefined ? "" : `id: ${id}\n`
    this.write(`${idLine}event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
  }

  end() {
    this.controller.close()
  }

  drop() {
    this.controller.error(new TypeError("network error"))
  }
}

export function refusedStream(status: number, code: string): Response {
  return new Response(JSON.stringify({ error: `refused ${code}`, code }), {
    status,
    headers: { "content-type": "application/json" },
  })
}

export function lastEventIdOf(init: RequestInit | undefined): string | null {
  const headers = (init?.headers ?? {}) as Record<string, string>
  return headers["last-event-id"] ?? null
}

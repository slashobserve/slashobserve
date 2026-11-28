type Listener = (event: MessageEvent<string>) => void

export class FakeEventSource {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static instances: FakeEventSource[] = []
  readyState = FakeEventSource.OPEN
  onerror: (() => void) | null = null
  closed = false
  readonly url: string
  private listeners = new Map<string, Listener[]>()

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(name: string, listener: Listener) {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener])
  }

  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
  }

  emit(name: string, data?: unknown) {
    const init: MessageEventInit<string> = {}
    if (data !== undefined) init.data = JSON.stringify(data)
    const message = new MessageEvent(name, init)
    for (const listener of this.listeners.get(name) ?? []) listener(message)
  }

  static open(): FakeEventSource[] {
    return FakeEventSource.instances.filter((source) => !source.closed)
  }
}

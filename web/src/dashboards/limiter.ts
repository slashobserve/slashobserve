type Entry = { start: () => void }

export type Limiter = {
  run<T>(task: () => Promise<T>, signal?: AbortSignal): Promise<T>
}

export function createLimiter(max: number): Limiter {
  let active = 0
  const queue: Entry[] = []

  function drain() {
    while (active < max) {
      const entry = queue.shift()
      if (!entry) return
      entry.start()
    }
  }

  return {
    run<T>(task: () => Promise<T>, signal?: AbortSignal): Promise<T> {
      return new Promise<T>((resolve, reject) => {
        if (signal?.aborted) {
          reject(signal.reason)
          return
        }
        const onAbort = () => {
          const index = queue.indexOf(entry)
          if (index >= 0) queue.splice(index, 1)
          reject(signal?.reason)
        }
        const entry: Entry = {
          start() {
            signal?.removeEventListener("abort", onAbort)
            active++
            new Promise<T>((done) => done(task()))
              .then(resolve, reject)
              .finally(() => {
                active--
                drain()
              })
          },
        }
        signal?.addEventListener("abort", onAbort, { once: true })
        queue.push(entry)
        drain()
      })
    },
  }
}

export const panelRequestLimiter = createLimiter(6)

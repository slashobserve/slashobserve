export type Throttle = { call: () => void; cancel: () => void }

export function throttle(run: () => void, waitMs: number): Throttle {
  let last = Number.NEGATIVE_INFINITY
  let timer: ReturnType<typeof setTimeout> | undefined
  function fire() {
    timer = undefined
    last = Date.now()
    run()
  }
  return {
    call: () => {
      if (timer !== undefined) return
      const wait = last + waitMs - Date.now()
      if (wait <= 0) fire()
      else timer = setTimeout(fire, wait)
    },
    cancel: () => {
      clearTimeout(timer)
      timer = undefined
    },
  }
}

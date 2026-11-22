import { useEffect, useState, type ReactNode } from "react"
import { useMutation } from "@tanstack/react-query"
import { act } from "@testing-library/react"

type Hold = { promise: Promise<void>; release: () => void }

function newHold(): Hold {
  let release = () => {}
  const promise = new Promise<void>((resolve) => {
    release = resolve
  })
  return { promise, release }
}

let hold = newHold()
let hideHost = () => {}

export function resetHeldMutation() {
  hold = newHold()
  hideHost = () => {}
}

export function useHeldMutation<Variables, Result>(
  send: (variables: Variables) => Promise<Result>
) {
  return useMutation({
    mutationFn: send,
    onSettled: () => hold.promise,
  })
}

export function HeldMutationHost({ children }: { children: ReactNode }) {
  const [shown, setShown] = useState(true)
  useEffect(() => {
    hideHost = () => setShown(false)
  }, [])
  return shown ? children : null
}

export async function unmountHostThenSettle() {
  await act(async () => {
    hideHost()
  })
  await act(async () => {
    hold.release()
  })
}

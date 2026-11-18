const loading = '[data-panel-id][data-state="loading"]'

export function panelsSettled(root: ParentNode): boolean {
  return (
    root.querySelector("[data-panel-id]") !== null &&
    root.querySelector(loading) === null
  )
}

function nextFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()))
}

function pause(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

type Options = { timeoutMs: number; settleMs: number }

export async function waitForPanels(
  root: ParentNode,
  { timeoutMs, settleMs }: Options
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  while (!panelsSettled(root)) {
    if (Date.now() >= deadline) return false
    await pause(100)
  }
  await pause(settleMs)
  await nextFrame()
  return panelsSettled(root)
}

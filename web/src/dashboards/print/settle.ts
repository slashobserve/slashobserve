const loading = '[data-panel-id][data-state="loading"]'

export function panelsSettled(root: ParentNode): boolean {
  return root.querySelector(loading) === null
}

export function slowPanelNames(root: ParentNode): string[] {
  return [...root.querySelectorAll(loading)].map((panel) => {
    const label = panel.getAttribute("aria-labelledby")
    const title = label === null ? null : document.getElementById(label)
    return title?.textContent ?? panel.getAttribute("data-panel-id") ?? ""
  })
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

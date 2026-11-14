export type HandedImport = { name: string; text: string; pasted: boolean }

let handed: HandedImport | null = null

export function handOffImport(next: HandedImport) {
  handed = next
}

export function peekHandedImport(): HandedImport | null {
  return handed
}

export function clearHandedImport() {
  handed = null
}

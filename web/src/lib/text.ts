export function lowerFirst(text: string): string {
  return text.charAt(0).toLowerCase() + text.slice(1)
}

export function nameParts(name: string): string[] {
  return name.split(/(?<=[._])|(?<=[a-z0-9])(?=[A-Z])/)
}

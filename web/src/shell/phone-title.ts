import type { Crumb } from "./crumbs"

export type PhoneTitle = { kind: "wordmark" } | { kind: "text"; label: string }

export function phoneTitle(
  pathname: string,
  crumbs: readonly Crumb[]
): PhoneTitle {
  const last = crumbs.at(-1)
  if (pathname === "/" || last === undefined) return { kind: "wordmark" }
  return { kind: "text", label: last.label }
}

export function headingInPhoneBar(
  title: string,
  crumbs: readonly Crumb[]
): boolean {
  return crumbs.at(-1)?.label === title
}

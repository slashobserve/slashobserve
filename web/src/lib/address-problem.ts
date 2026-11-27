export type AddressRules = {
  example: string
  https?: boolean
  noun?: "URL" | "address"
  empty?: string
  maxBytes?: number
  publicHost?: string
  check?: (parsed: URL) => string | undefined
}

export function addressProblem(
  url: string,
  {
    example,
    https = false,
    noun = "address",
    empty,
    maxBytes,
    publicHost,
    check,
  }: AddressRules
): string | undefined {
  if (empty !== undefined && url === "") return empty
  if (
    maxBytes !== undefined &&
    new TextEncoder().encode(url).length > maxBytes
  ) {
    return `The ${noun} can be at most ${maxBytes} characters.`
  }
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return https
      ? `Enter a full https address, such as ${example}.`
      : `Enter a full http or https URL, such as ${example}.`
  }
  if (
    https ? parsed.protocol !== "https:" : !/^https?:$/.test(parsed.protocol)
  ) {
    return `The ${noun} must start with ${https ? "https://" : "http:// or https://"}.`
  }
  const checked = check?.(parsed)
  if (checked !== undefined) return checked
  if (
    publicHost !== undefined &&
    (!parsed.hostname.includes(".") ||
      /^[\d.]+$/.test(parsed.hostname) ||
      parsed.hostname.startsWith("["))
  ) {
    return `The address must use a public domain name, like ${publicHost}. localhost and IP addresses are refused.`
  }
  return undefined
}

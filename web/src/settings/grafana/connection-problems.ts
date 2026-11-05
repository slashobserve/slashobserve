import { ApiError } from "@/api/errors"
import { loadProblemText } from "@/service-accounts/queries"

export type ConnectionInput = { name: string; url: string; token: string }
export type ConnectionProblems = Partial<Record<keyof ConnectionInput, string>>

const maxNameCharacters = 100
const maxURLBytes = 2048
const maxTokenCharacters = 4096

export function trimConnection(input: ConnectionInput): ConnectionInput {
  return {
    name: input.name.trim(),
    url: input.url.trim(),
    token: input.token.trim(),
  }
}

function urlProblem(url: string): string | undefined {
  if (url === "") return "Enter the Grafana URL."
  if (new TextEncoder().encode(url).length > maxURLBytes) {
    return `The URL can be at most ${maxURLBytes} characters.`
  }
  let protocol: string
  try {
    protocol = new URL(url).protocol
  } catch {
    return "Enter a full http or https URL, such as https://grafana.example.com."
  }
  if (protocol !== "http:" && protocol !== "https:") {
    return "The URL must start with http:// or https://."
  }
  return undefined
}

export function connectionProblems(raw: ConnectionInput): ConnectionProblems {
  const input = trimConnection(raw)
  const problems: ConnectionProblems = {}
  if (input.name === "") problems.name = "Enter a name."
  else if ([...input.name].length > maxNameCharacters) {
    problems.name = `The name can be at most ${maxNameCharacters} characters.`
  }
  const url = urlProblem(input.url)
  if (url !== undefined) problems.url = url
  if (input.token === "") problems.token = "Enter a service account token."
  else if (input.token.length > maxTokenCharacters) {
    problems.token = `The token can be at most ${maxTokenCharacters} characters.`
  }
  return problems
}

export function grafanaProblemText(error: unknown): string {
  if (!(error instanceof ApiError)) return loadProblemText(error)
  if (error.code === "secret_key_missing") {
    return "Set SLASHOBSERVE_SECRET_KEY on the server and restart."
  }
  return error.message
}

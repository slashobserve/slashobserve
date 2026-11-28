import { ApiError, loadProblemText } from "@/api/errors"
import { addressProblem } from "@/lib/address-problem"

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

export type StoredConnection = { name: string; url: string }

export function connectionPatch(
  raw: ConnectionInput,
  stored: StoredConnection
): Partial<ConnectionInput> {
  const input = trimConnection(raw)
  const patch: Partial<ConnectionInput> = {}
  if (input.name !== stored.name) patch.name = input.name
  if (input.url !== stored.url) patch.url = input.url
  if (input.token !== "") patch.token = input.token
  return patch
}

export function connectionProblems(
  raw: ConnectionInput,
  stored?: StoredConnection
): ConnectionProblems {
  const input = trimConnection(raw)
  const problems: ConnectionProblems = {}
  if (input.name === "") problems.name = "Enter a name."
  else if ([...input.name].length > maxNameCharacters) {
    problems.name = `The name can be at most ${maxNameCharacters} characters.`
  }
  const url = addressProblem(input.url, {
    example: "https://grafana.example.com",
    noun: "URL",
    empty: "Enter the Grafana URL.",
    maxBytes: maxURLBytes,
  })
  if (url !== undefined) problems.url = url
  const tokenKept = stored !== undefined && input.url === stored.url
  if (input.token === "") {
    if (stored === undefined) problems.token = "Enter a service account token."
    else if (!tokenKept) {
      problems.token =
        "Enter the token again: a new URL never reuses the stored one."
    }
  } else if (input.token.length > maxTokenCharacters) {
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

export const maxPromptLength = 4000
const maxPromptNameLength = 64
const suggestedWordCount = 3

export const invalidNameText =
  "Use letters, numbers, spaces, - and _, starting and ending with a letter or number."

const validName = /^[\p{L}\p{N}](?:[\p{L}\p{N} _-]*[\p{L}\p{N}])?$/u
const outsideNameCharacters = /[^\p{L}\p{N} _-]/gu
const edgeSeparators = /^[-_ ]+|[-_ ]+$/g

export function normalizePromptName(raw: string): string {
  return raw.trim().split(/\s+/).filter(Boolean).join(" ")
}

export function promptNameError(raw: string): string | null {
  const name = normalizePromptName(raw)
  if (name === "") return "Enter a name."
  if ([...name].length > maxPromptNameLength) {
    return `Use at most ${maxPromptNameLength} characters.`
  }
  return validName.test(name) ? null : invalidNameText
}

export function promptBodyError(raw: string): string | null {
  const body = raw.trim()
  if (body === "") return "Enter the prompt."
  if ([...body].length > maxPromptLength) {
    return `Use at most ${maxPromptLength} characters.`
  }
  return null
}

function cutAtWord(text: string, limit: number): string {
  const characters = [...text]
  if (characters.length <= limit) return text
  const cut = characters.slice(0, limit)
  const boundary = characters[limit] === " " ? limit : cut.lastIndexOf(" ")
  return (boundary > 0 ? cut.slice(0, boundary) : cut).join("")
}

export function suggestPromptName(body: string): string {
  const firstLine = body.split("\n", 1)[0] ?? ""
  const words = firstLine
    .replace(outsideNameCharacters, " ")
    .split(" ")
    .filter(Boolean)
    .slice(0, suggestedWordCount)
  const name = cutAtWord(words.join(" "), maxPromptNameLength).replace(
    edgeSeparators,
    ""
  )
  const [first = "", ...rest] = [...name]
  return `${first.toLocaleUpperCase()}${rest.join("")}`
}

import type { Transform } from "@/spec"

import {
  matchesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type FormatStringOptions = Extract<Transform, { type: "formatString" }>

const wordBreak = /(\p{Ll}|\p{N})(\p{Lu})/gu
const nonWord = /[^\p{L}\p{N}]+/u

function words(text: string): string[] {
  return text
    .replace(wordBreak, "$1 $2")
    .split(nonWord)
    .filter((word) => word !== "")
}

function capitalize(word: string): string {
  const [first = "", ...rest] = word
  return first.toUpperCase() + rest.join("").toLowerCase()
}

function sentence(text: string): string {
  return capitalize(text)
}

function title(text: string): string {
  return text.replace(/\S+/gu, capitalize)
}

function formatter(options: FormatStringOptions): (text: string) => string {
  switch (options.format) {
    case "upper":
      return (text) => text.toUpperCase()
    case "lower":
      return (text) => text.toLowerCase()
    case "sentence":
      return sentence
    case "title":
      return title
    case "pascal":
      return (text) => words(text).map(capitalize).join("")
    case "camel":
      return (text) =>
        words(text)
          .map((word, i) => (i === 0 ? word.toLowerCase() : capitalize(word)))
          .join("")
    case "snake":
      return (text) =>
        words(text)
          .map((word) => word.toLowerCase())
          .join("_")
    case "kebab":
      return (text) =>
        words(text)
          .map((word) => word.toLowerCase())
          .join("-")
    case "trim":
      return (text) => text.trim()
    case "substring":
      return (text) => text.slice(options.start ?? 0, options.end)
  }
}

export function formatString(
  frames: PreparedFrame[],
  options: FormatStringOptions
): PreparedFrame[] {
  const format = formatter(options)
  return frames.map((frame) =>
    withFields(
      frame,
      frame.fields.map((field): PreparedField =>
        matchesName(frame, field, options.field)
          ? {
              ...field,
              values: field.values.map((value) =>
                typeof value === "string" ? format(value) : value
              ),
            }
          : field
      )
    )
  )
}

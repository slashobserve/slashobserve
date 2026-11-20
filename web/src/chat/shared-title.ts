type SharedTitle = "renamed" | "kept" | undefined

const keptNotes = {
  chat: "The observation made from this chat keeps its title, because you cannot edit it.",
  observation:
    "The chat this observation was made from keeps its title, because only the person who started it can rename it.",
}

export function keptTitleNote(
  renamed: keyof typeof keptNotes,
  outcome: SharedTitle
): string | null {
  return outcome === "kept" ? keptNotes[renamed] : null
}

import { setDiagnosticsEffect, type Diagnostic } from "@codemirror/lint"
import {
  StateField,
  type EditorState,
  type Extension,
  type Range,
} from "@codemirror/state"
import {
  Decoration,
  EditorView,
  WidgetType,
  type DecorationSet,
} from "@codemirror/view"

class LintLine extends WidgetType {
  readonly message: string
  readonly severity: "error" | "warning"

  constructor(message: string, severity: "error" | "warning") {
    super()
    this.message = message
    this.severity = severity
  }

  eq(other: LintLine) {
    return other.message === this.message && other.severity === this.severity
  }

  toDOM() {
    const line = document.createElement("div")
    line.className = "cm-lint-line"
    line.dataset.severity = this.severity
    line.setAttribute("aria-hidden", "true")
    line.textContent = this.message
    return line
  }
}

function lintDecorations(
  state: EditorState,
  diagnostics: readonly Diagnostic[]
): DecorationSet {
  const ranges: Range<Decoration>[] = diagnostics.map((diagnostic) => {
    const line = state.doc.lineAt(Math.min(diagnostic.from, state.doc.length))
    const severity = diagnostic.severity === "warning" ? "warning" : "error"
    return Decoration.widget({
      widget: new LintLine(diagnostic.message, severity),
      block: true,
      side: 1,
    }).range(line.to)
  })
  return Decoration.set(ranges, true)
}

export function lintLines(): Extension {
  return StateField.define<DecorationSet>({
    create: () => Decoration.none,
    update(decorations, transaction) {
      for (const effect of transaction.effects) {
        if (effect.is(setDiagnosticsEffect)) {
          return lintDecorations(transaction.state, effect.value)
        }
      }
      return decorations.map(transaction.changes)
    },
    provide: (field) => EditorView.decorations.from(field),
  })
}

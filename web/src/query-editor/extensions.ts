import { autocompletion, completionKeymap } from "@codemirror/autocomplete"
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands"
import { json } from "@codemirror/lang-json"
import { sql, StandardSQL } from "@codemirror/lang-sql"
import { syntaxHighlighting } from "@codemirror/language"
import { lintGutter, lintKeymap } from "@codemirror/lint"
import { Compartment, type Extension, type Text } from "@codemirror/state"
import {
  EditorView,
  highlightActiveLine,
  ViewPlugin,
  keymap,
  lineNumbers,
} from "@codemirror/view"
import { classHighlighter } from "@lezer/highlight"
import { PromQLExtension } from "@prometheus-io/codemirror-promql"

export const theme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--foreground)",
    fontSize: "13px",
  },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": {
    fontFamily: "var(--font-mono)",
    lineHeight: "1.5",
  },
  ".cm-content": { caretColor: "var(--foreground)" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--foreground)" },
  ".cm-gutters": {
    backgroundColor: "var(--muted)",
    color: "var(--muted-foreground)",
    border: "none",
  },
  ".cm-activeLine": {
    backgroundColor: "color-mix(in oklch, var(--muted) 60%, transparent)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--muted)",
    color: "var(--foreground)",
  },
  ".cm-tooltip": {
    backgroundColor: "var(--popover)",
    color: "var(--popover-foreground)",
    border: "1px solid var(--border)",
    borderRadius: "var(--radius-md)",
    overflow: "hidden",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul": {
    fontFamily: "var(--font-mono)",
    maxHeight: "8rem",
    maxWidth: "min(24rem, 90vw)",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li": { padding: "2px 8px" },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]": {
    backgroundColor: "var(--muted)",
    color: "var(--foreground)",
  },
  ".cm-completionDetail": {
    fontStyle: "normal",
    color: "var(--muted-foreground)",
  },
  ".cm-completionIcon": { opacity: "0.6" },
  ".cm-completionInfo": {
    fontFamily: "var(--font-sans)",
    padding: "4px 8px",
  },
  ".cm-diagnostic": { borderLeft: "none", padding: "4px 8px" },
  ".cm-diagnosticSource": { display: "none" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection":
    {
      backgroundColor:
        "color-mix(in oklch, var(--foreground) 15%, transparent)",
    },
})

const bodyLanguage = new Compartment()

function startsWithBrace(doc: Text): boolean {
  return /^\s*\{/.test(doc.sliceString(0, 256))
}

function jsonWhenQueryDsl(): Extension {
  const plugin = ViewPlugin.define((view) => {
    let isJson = false
    let destroyed = false
    function sync() {
      const next = startsWithBrace(view.state.doc)
      if (next === isJson) return
      isJson = next
      queueMicrotask(() => {
        if (destroyed) return
        view.dispatch({
          effects: bodyLanguage.reconfigure(next ? json() : []),
        })
      })
    }
    sync()
    return {
      update(update) {
        if (update.docChanged) sync()
      },
      destroy() {
        destroyed = true
      },
    }
  })
  return [bodyLanguage.of([]), plugin]
}

function languageExtension(type: string | undefined): Extension {
  if (type === "prometheus") {
    return new PromQLExtension()
      .activateLinter(false)
      .setComplete({})
      .asExtension()
  }
  if (type === "clickhouse") {
    return sql({ dialect: StandardSQL, upperCaseKeywords: true })
  }
  if (type === "elasticsearch" || type === "opensearch") {
    return jsonWhenQueryDsl()
  }
  return []
}

export function languageFor(type: string | undefined): Extension {
  return [
    languageExtension(type),
    syntaxHighlighting(classHighlighter),
    history(),
    keymap.of([
      ...defaultKeymap,
      ...historyKeymap,
      ...completionKeymap,
      ...lintKeymap,
    ]),
    autocompletion(),
    lineNumbers(),
    highlightActiveLine(),
    EditorView.lineWrapping,
    lintGutter(),
    theme,
  ]
}

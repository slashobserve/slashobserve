import { useEffect, useRef, useState } from "react"

import {
  draftFailure,
  draftQueryRequest,
  type DraftAnswer,
  type DraftFailure,
  type DraftSignal,
} from "./draft-query"

type AskProblem = Exclude<DraftFailure, { kind: "aborted" }>

type Answer = { explanation: string; warnings: string[] }

export type AskState = {
  question: string
  setQuestion: (text: string) => void
  asking: boolean
  answer: Answer | null
  problem: AskProblem | null
  fromQuestion: string | null
  ask: (text: string) => void
  cancel: () => void
  clearProblem: () => void
  askAgain: () => void
}

export function useAsk({
  uid,
  signal,
  range,
  initialQuestion,
  initialFromQuestion,
  onAnswer,
  onCannotTranslate,
}: {
  uid: string
  signal: DraftSignal
  range: { from: string; to: string }
  initialQuestion: string | undefined
  initialFromQuestion: string | null
  onAnswer: (answer: DraftAnswer, question: string) => void
  onCannotTranslate: () => void
}): AskState {
  const [question, setQuestion] = useState(initialQuestion ?? "")
  const [fromQuestion, setFromQuestion] = useState(initialFromQuestion)
  const [answer, setAnswer] = useState<Answer | null>(null)
  const [asking, setAsking] = useState(false)
  const [problem, setProblem] = useState<AskProblem | null>(null)
  const abort = useRef<AbortController | null>(null)

  useEffect(() => () => abort.current?.abort(), [])

  async function ask(text: string) {
    abort.current?.abort()
    const controller = new AbortController()
    abort.current = controller
    setAsking(true)
    setProblem(null)
    try {
      const drafted = await draftQueryRequest(
        uid,
        { question: text, from: range.from, to: range.to, signal },
        controller.signal
      )
      if (controller.signal.aborted) return
      setAnswer({
        explanation: drafted.explanation,
        warnings: drafted.warnings,
      })
      setFromQuestion(text)
      onAnswer(drafted, text)
    } catch (error) {
      const failure = draftFailure(error)
      if (failure.kind === "aborted") return
      setProblem(failure)
      if (failure.kind === "cannot_translate") onCannotTranslate()
    } finally {
      if (abort.current === controller) {
        abort.current = null
        setAsking(false)
      }
    }
  }

  return {
    question,
    setQuestion,
    asking,
    answer,
    problem,
    fromQuestion,
    ask: (text) => void ask(text),
    cancel: () => abort.current?.abort(),
    clearProblem: () => setProblem(null),
    askAgain: () => {
      if (fromQuestion !== null) setQuestion(fromQuestion)
    },
  }
}

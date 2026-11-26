import type { StatusTone } from "@/components/status-label"
import { buildStatusText } from "@/observe/build"
import { requestStatusView, type RequestSummary } from "@/observe/queries"
import { requestOrigin } from "@/observe/request-origin"

export function chatState(state: RequestSummary["state"]): {
  word: "Running" | "Stopped" | "Done"
  tone: StatusTone
} {
  switch (state) {
    case "queued":
    case "running":
    case "canceling":
      return { word: "Running", tone: "running" }
    case "succeeded":
      return { word: "Done", tone: "ok" }
    case "failed":
    case "canceled":
      return { word: "Stopped", tone: "warning" }
  }
}

export type ChatStateFilter = "running" | "stopped" | "done"

const everyState = {
  queued: true,
  running: true,
  canceling: true,
  succeeded: true,
  failed: true,
  canceled: true,
} satisfies Record<RequestSummary["state"], true>

export function statesFor(filter: ChatStateFilter): RequestSummary["state"][] {
  return (Object.keys(everyState) as RequestSummary["state"][]).filter(
    (state) => chatState(state).word.toLowerCase() === filter
  )
}

function savedText(request: RequestSummary): string {
  if (request.dashboard_id === null && request.dashboard_title === null) {
    return requestStatusView(request).text
  }
  const verb = requestOrigin(request) === "observe" ? "Saved" : "Revised"
  return `${verb} ${request.dashboard_title ?? "a dashboard"}`
}

function observationOutcome(request: RequestSummary): string {
  const title = request.observation_title
  if (request.kind === "build") {
    if (request.dashboard_id === null) {
      return title === null ? "Answered, no dashboard" : `Answered in ${title}`
    }
    return title === null
      ? "Answered, dashboard unchanged"
      : `Answered in ${title}, dashboard unchanged`
  }
  if (title !== null) return `Answered in ${title}`
  return request.result_kind === "answer"
    ? "Answered, no observation"
    : "Saved an observation"
}

function succeededOutcome(request: RequestSummary): string {
  switch (request.result_kind) {
    case "version":
    case "partial_version":
      return savedText(request)
    case "observation":
    case "answer":
      return observationOutcome(request)
    case "suggestion":
      return "Suggested a panel"
    case "proposal":
      return "Opened a proposal"
    case "none":
      return "Nothing saved"
  }
}

export type SavedObservation = { id: string; title: string }

export function savedObservations(
  turns: readonly RequestSummary[]
): SavedObservation[] {
  const found = new Map<string, string>()
  for (const turn of turns) {
    if (
      turn.result_kind === "observation" &&
      turn.observation_id !== null &&
      turn.observation_title !== null
    ) {
      found.set(turn.observation_id, turn.observation_title)
    }
  }
  return [...found].map(([id, title]) => ({ id, title }))
}

export function deleteObservationLabel(observation: SavedObservation): string {
  return `Also delete the observation "${observation.title}"`
}

function removedText(observations: readonly SavedObservation[]): string {
  const names = joinThings(
    observations.map((observation) => `"${observation.title}"`)
  )
  const [subject, their] =
    observations.length === 1
      ? [`The observation ${names} is`, "its"]
      : [`The observations ${names} are`, "their"]
  return `${subject} deleted for everyone, with ${their} blocks, discussion and published RCA revisions, and this cannot be undone.`
}

export function deleteChatText(
  request: RequestSummary,
  removing: ReadonlySet<string> = new Set()
): string {
  const left = "It leaves your chat lists and cannot be added back."
  const removed = savedObservations([request]).filter((observation) =>
    removing.has(observation.id)
  )
  if (removed.length > 0) return `${left} ${removedText(removed)}`
  switch (request.result_kind) {
    case "version":
    case "partial_version":
      return `${left} The dashboard it made stays.`
    case "answer":
      return `${left} The answer stays on the chat page, which still opens from its link.`
    case "observation":
      return request.observation_title === null
        ? `${left} The answer stays in its observation, which is not deleted.`
        : `${left} The answer stays in the observation "${request.observation_title}", which is not deleted.`
    case "suggestion":
      return `${left} The panel suggestion stays.`
    case "proposal":
      return `${left} The proposal stays.`
    case "none":
      return `${left} Nothing else is removed.`
  }
}

function savedThings(
  turns: readonly RequestSummary[],
  removing: ReadonlySet<string>
): string[] {
  const dashboards = new Map<string, string>()
  const observations = new Map<string, string>()
  let unsavedAnswers = 0
  let suggestions = 0
  let proposals = 0
  for (const turn of turns) {
    switch (turn.result_kind) {
      case "version":
      case "partial_version":
        dashboards.set(
          turn.dashboard_id ?? turn.id,
          turn.dashboard_title === null
            ? "a dashboard"
            : `the dashboard "${turn.dashboard_title}"`
        )
        break
      case "answer":
        unsavedAnswers += 1
        break
      case "observation":
        if (
          turn.observation_id !== null &&
          !removing.has(turn.observation_id)
        ) {
          observations.set(
            turn.observation_id,
            turn.observation_title === null
              ? "an observation"
              : `the observation "${turn.observation_title}"`
          )
        }
        break
      case "suggestion":
        suggestions += 1
        break
      case "proposal":
        proposals += 1
        break
      case "none":
        break
    }
  }
  const things = [...dashboards.values(), ...observations.values()]
  if (suggestions > 0) {
    things.push(suggestions === 1 ? "a panel suggestion" : "panel suggestions")
  }
  if (proposals > 0) things.push(proposals === 1 ? "a proposal" : "proposals")
  if (unsavedAnswers > 0) {
    things.push(
      unsavedAnswers === 1
        ? "an answer on its chat page"
        : "answers on their chat pages"
    )
  }
  return things
}

function joinThings(things: readonly string[]): string {
  if (things.length < 2) return things.join("")
  return `${things.slice(0, -1).join(", ")} and ${things[things.length - 1]}`
}

export function deleteTurnsText(
  turns: readonly RequestSummary[],
  removing: ReadonlySet<string> = new Set()
): string {
  if (turns.length === 1) return deleteChatText(turns[0], removing)
  const left = `All ${turns.length} messages in this chat leave your chat lists and cannot be added back.`
  const things = savedThings(turns, removing)
  const removed = savedObservations(turns).filter((observation) =>
    removing.has(observation.id)
  )
  const stays =
    things.length > 0
      ? ` What they saved stays: ${joinThings(things)}.`
      : removed.length === 0
        ? " Nothing else is removed."
        : ""
  return removed.length === 0
    ? `${left}${stays}`
    : `${left}${stays} ${removedText(removed)}`
}

export function chatOutcome(request: RequestSummary): string {
  switch (request.state) {
    case "queued":
    case "running":
    case "canceling":
      return buildStatusText({
        status: request.status,
        build: request.build,
        failureCode: request.failure_code,
        canceling: request.state === "canceling",
      })
    case "failed":
    case "canceled":
      if (request.result_kind === "partial_version") {
        return request.dashboard_title === null
          ? requestStatusView(request).text
          : `Saved part of ${request.dashboard_title}`
      }
      return buildStatusText({
        status: request.status,
        build: request.build,
        failureCode: request.failure_code,
        continued: request.continued_by !== null,
      })
    case "succeeded":
      return succeededOutcome(request)
  }
}

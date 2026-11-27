import type { Role } from "@/lib/roles"

import { defaultExpiry, type ExpiryValue } from "./key-status"
import type { ServiceAccount } from "./queries"

type KeyStep =
  | { phase: "form"; problem: string | null }
  | { phase: "creating" }
  | { phase: "partly_done"; problem: string }
  | { phase: "issued"; token: string; accountName: string; role: Role }

export type KeyPanelState = {
  step: KeyStep
  keyName: string
  accountChoice: string | null
  newAccountName: string
  roleChoice: Role | null
  takesRunsChoice: boolean | null
  expiry: ExpiryValue
  submitted: boolean
  createdAccount: ServiceAccount | null
}

export type KeyPanelEvent =
  | { type: "key_named"; name: string }
  | { type: "account_chosen"; choice: string }
  | { type: "account_named"; name: string }
  | { type: "role_chosen"; role: Role }
  | { type: "takes_runs_chosen"; takesRuns: boolean }
  | { type: "expiry_chosen"; expiry: ExpiryValue }
  | { type: "submit_refused" }
  | { type: "submit_started" }
  | { type: "account_created"; account: ServiceAccount }
  | { type: "account_updated"; account: ServiceAccount }
  | { type: "failed"; problem: string }
  | { type: "key_issued"; token: string; accountName: string; role: Role }

export const emptyKeyPanel: KeyPanelState = {
  step: { phase: "form", problem: null },
  keyName: "",
  accountChoice: null,
  newAccountName: "",
  roleChoice: null,
  takesRunsChoice: null,
  expiry: defaultExpiry,
  submitted: false,
  createdAccount: null,
}

function editable(state: KeyPanelState): boolean {
  return state.step.phase === "form" || state.step.phase === "partly_done"
}

function chooseAccount(state: KeyPanelState, choice: string): KeyPanelState {
  const { step } = state
  return {
    ...state,
    accountChoice: choice,
    roleChoice: null,
    takesRunsChoice: null,
    step:
      step.phase === "partly_done" && choice !== state.createdAccount?.id
        ? { phase: "form", problem: step.problem }
        : step,
  }
}

function fail(state: KeyPanelState, problem: string): KeyPanelState {
  const accountExists =
    state.createdAccount !== null &&
    state.accountChoice === state.createdAccount.id
  return {
    ...state,
    step: accountExists
      ? { phase: "partly_done", problem }
      : { phase: "form", problem },
  }
}

export function reduceKeyPanel(
  state: KeyPanelState,
  event: KeyPanelEvent
): KeyPanelState {
  if (state.step.phase === "issued") return state
  if (state.step.phase === "creating") {
    switch (event.type) {
      case "account_created":
        return {
          ...state,
          createdAccount: event.account,
          accountChoice: event.account.id,
        }
      case "account_updated":
        return state.createdAccount?.id === event.account.id
          ? { ...state, createdAccount: event.account }
          : state
      case "failed":
        return fail(state, event.problem)
      case "key_issued":
        return {
          ...state,
          step: {
            phase: "issued",
            token: event.token,
            accountName: event.accountName,
            role: event.role,
          },
        }
      default:
        return state
    }
  }
  if (!editable(state)) return state
  switch (event.type) {
    case "key_named":
      return { ...state, keyName: event.name }
    case "account_chosen":
      return chooseAccount(state, event.choice)
    case "account_named":
      return { ...state, newAccountName: event.name }
    case "role_chosen":
      return { ...state, roleChoice: event.role }
    case "takes_runs_chosen":
      return { ...state, takesRunsChoice: event.takesRuns }
    case "expiry_chosen":
      return { ...state, expiry: event.expiry }
    case "submit_refused":
      return { ...state, submitted: true }
    case "submit_started":
      return { ...state, submitted: true, step: { phase: "creating" } }
    default:
      return state
  }
}

export function accountWithoutKey(state: KeyPanelState): ServiceAccount | null {
  return state.step.phase === "partly_done" ? state.createdAccount : null
}

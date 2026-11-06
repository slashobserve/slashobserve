export type FailureText = { meaning: string; next: string }

const retryWithNames =
  "Try again and name the metric, service or table you want."

const texts: Record<string, FailureText> = {
  run_timeout: {
    meaning: "The run reached the server's time limit.",
    next: "Try again with a narrower prompt, or continue the build.",
  },
  call_timeout: {
    meaning: "A model call took too long to answer.",
    next: "Try again.",
  },
  rate_limited: {
    meaning: "The model provider is limiting requests from this server.",
    next: "Wait a minute, then try again.",
  },
  provider_error: {
    meaning: "The model provider answered with an error.",
    next: "Try again, and ask an admin to check the provider if it keeps failing.",
  },
  output_limit: {
    meaning: "The model's answer was cut off at its output limit.",
    next: "Try again and ask for fewer panels.",
  },
  no_tool_call: {
    meaning: "The agent stopped making progress.",
    next: retryWithNames,
  },
  no_progress: {
    meaning: "The agent stopped making progress.",
    next: retryWithNames,
  },
  validation_exhausted: {
    meaning: "The agent could not get its queries past the checks.",
    next: retryWithNames,
  },
  check_failed: {
    meaning: "The agent could not get its queries past the checks.",
    next: retryWithNames,
  },
  refused: {
    meaning: "The model declined the request.",
    next: "Rephrase the prompt.",
  },
  conflict: {
    meaning: "Someone saved a newer version while the agent worked.",
    next: "Open the dashboard and ask again.",
  },
  canceled: {
    meaning: "The run was canceled.",
    next: "Save what was built, continue it, or try again.",
  },
  requester_forbidden: {
    meaning: "You no longer have access to this dashboard.",
    next: "Ask an admin for Edit access.",
  },
  requester_gone: {
    meaning: "You no longer have access to this dashboard.",
    next: "Ask an admin for Edit access.",
  },
  crashed: {
    meaning: "The server stopped while it ran this request.",
    next: "Try again.",
  },
  lease_expired: {
    meaning: "The server stopped while it ran this request.",
    next: "Try again.",
  },
  no_finding: {
    meaning: "The agent ended without writing a finding.",
    next: "Ask a narrower question, with the service and the time range.",
  },
}

export function failureText(code: string | null): FailureText | null {
  if (code === null || !Object.hasOwn(texts, code)) return null
  return texts[code]
}

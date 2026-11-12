declare const OBSERVE_AGENT_VERSION: string | undefined

export const version: string =
  typeof OBSERVE_AGENT_VERSION === "undefined" ? "dev" : OBSERVE_AGENT_VERSION

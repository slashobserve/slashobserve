declare const HARNESS_VERSION: string | undefined

export const version: string =
  typeof HARNESS_VERSION === "undefined" ? "dev" : HARNESS_VERSION

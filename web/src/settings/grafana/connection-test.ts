import { toast } from "sonner"

import type { GrafanaConnection } from "./queries"

export function testOutcome(connection: GrafanaConnection, verb: string) {
  const test = connection.last_test
  if (test === null) {
    toast.warning(`${verb} ${connection.name}, but it could not be tested yet.`)
  } else if (test.ok) {
    toast.success(`${verb} ${connection.name}. ${test.message}`)
  } else {
    toast.warning(
      `${verb} ${connection.name}, but the test failed: ${test.message}`
    )
  }
}

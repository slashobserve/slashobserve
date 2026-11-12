import type { SessionFactory } from "./types"
import { replayProvider } from "./replay/replay"
import { SessionError } from "./types"

export const createSession: SessionFactory = async (
  settings,
  start,
  signal
) => {
  if (settings.provider === "replay") {
    const provider = await replayProvider(
      settings.replay_dir,
      settings.max_tokens
    )
    return provider.start(start, signal)
  }
  throw new SessionError(
    `observe-agent has no ${settings.provider} session yet`
  )
}

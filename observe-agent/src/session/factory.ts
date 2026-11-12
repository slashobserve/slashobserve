import type { SessionFactory } from "./types"
import { createAnthropicSession } from "./anthropic"
import { createOpenAISession } from "./openai"
import { replayProvider } from "./replay/replay"
import { SessionError } from "./types"

export const createSession: SessionFactory = async (
  settings,
  start,
  signal
) => {
  switch (settings.provider) {
    case "replay": {
      const provider = await replayProvider(
        settings.replay_dir,
        settings.max_tokens
      )
      return provider.start(start, signal)
    }
    case "anthropic":
      return createAnthropicSession(settings, start, signal)
    case "openai":
      return createOpenAISession(settings, start, signal)
    default:
      throw new SessionError(
        `observe-agent cannot call provider ${settings.provider}`
      )
  }
}

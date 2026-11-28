import { useEffect } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { LoadProblem } from "@/components/load-problem"
import { useNow } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { truncatingTriggerClass } from "@/activity/show-filter"
import { ApiError, loadProblemText } from "@/api/errors"
import { Composer } from "@/conversation/composer"
import { ungated } from "@/conversation/delivery"
import { useComposer } from "@/conversation/use-composer"
import { observeRequestQuery, type ObserveRequest } from "@/observe/queries"

import { chatOptionMeta } from "./chat-rows"
import { useChat, useChats, type Chat } from "./chats-queries"
import { Conversation } from "./conversation"
import {
  panelChips,
  panelTarget,
  useFocusOnPanel,
  type ChatPanel,
} from "./panel-target"

const listedChats = 50
const allChatsValue = "all-chats"
const chatsHeading = "Your chats on this dashboard"
const newChatId = "new"

export type OpenChat = (chatId: string, options?: { replace?: boolean }) => void

type Props = {
  dashboardId: string
  dashboardTitle: string
  chatId: string | undefined
  onOpenChat: OpenChat
  panel?: ChatPanel | null
}

function withOpenChat(chats: readonly Chat[], open: Chat | undefined): Chat[] {
  if (open === undefined || chats.some((chat) => chat.id === open.id)) {
    return [...chats]
  }
  return [open, ...chats]
}

export function DashboardChat({
  dashboardId,
  dashboardTitle,
  chatId,
  onOpenChat,
  panel = null,
}: Props) {
  const navigate = useNavigate()
  const now = useNow()
  const fresh = chatId === newChatId
  const { chats, query } = useChats({ dashboardId }, listedChats)
  const latestId = chats.at(0)?.id
  const openId = fresh ? undefined : (chatId ?? latestId)
  useEffect(() => {
    if (chatId === undefined && latestId !== undefined) {
      onOpenChat(latestId, { replace: true })
    }
  }, [chatId, latestId, onOpenChat])
  const open = useChat(openId)
  const turnLink =
    open.error instanceof ApiError && open.error.code === "not_found"
  const turn = useQuery({
    ...observeRequestQuery(openId ?? ""),
    enabled: turnLink,
  })
  const turnChatId = turnLink ? turn.data?.chat_id : undefined
  useEffect(() => {
    if (turnChatId != null && turnChatId !== openId) {
      onOpenChat(turnChatId, { replace: true })
    }
  }, [openId, turnChatId, onOpenChat])
  const options = withOpenChat(chats, open.data?.chat)
  const elsewhere =
    open.data !== undefined && open.data.chat.dashboard_id !== dashboardId
  const items = [
    ...options.map((chat) => ({ value: chat.id, label: chat.title })),
    { value: allChatsValue, label: "All your chats" },
  ]

  function choose(next: string | null) {
    if (next === null) return
    if (next === allChatsValue) {
      void navigate({ to: "/chats", search: { dashboard: dashboardId } })
      return
    }
    onOpenChat(next)
  }

  function follow(request: ObserveRequest) {
    if (request.chat_id !== null && request.chat_id !== openId) {
      onOpenChat(request.chat_id, { replace: true })
    }
  }

  const unlisted = query.data === undefined && chatId === undefined
  const failed = unlisted && query.isError

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-none items-center gap-2 border-b px-4 py-2">
        <Select items={items} value={openId ?? null} onValueChange={choose}>
          <SelectTrigger
            size="sm"
            aria-label="Chat"
            className={`flex-1 ${truncatingTriggerClass}`}
          >
            <SelectValue placeholder="New chat" />
          </SelectTrigger>
          <SelectContent
            aria-label={chatsHeading}
            alignItemWithTrigger={false}
            align="start"
          >
            <SelectGroup>
              <SelectLabel>{chatsHeading}</SelectLabel>
              {options.map((chat) => (
                <SelectItem key={chat.id} value={chat.id}>
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate">{chat.title}</span>
                    <span className="text-xs text-muted-foreground">
                      {chatOptionMeta(chat, now)}
                    </span>
                  </span>
                </SelectItem>
              ))}
            </SelectGroup>
            <SelectSeparator />
            <SelectItem value={allChatsValue}>All your chats</SelectItem>
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          disabled={openId === undefined}
          onClick={() => onOpenChat(newChatId)}
        >
          <PlusIcon aria-hidden="true" />
          New chat
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col px-4 pt-3">
        {failed ? (
          <LoadProblem
            title="Could not load your chats"
            detail={loadProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : unlisted ? (
          <Skeleton className="h-24 rounded-card" />
        ) : openId === undefined ? (
          <NewChat
            dashboardId={dashboardId}
            dashboardTitle={dashboardTitle}
            panel={panel}
            onSent={(request) =>
              onOpenChat(request.chat_id ?? request.id, { replace: true })
            }
          />
        ) : (
          <>
            <Conversation
              key={openId}
              requestId={openId}
              layout="pane"
              asks={!elsewhere}
              panel={elsewhere ? null : panel}
              onSent={follow}
            />
            {elsewhere ? <ElsewhereNote chatId={openId} /> : null}
          </>
        )}
      </div>
    </div>
  )
}

function NewChat({
  dashboardId,
  dashboardTitle,
  panel,
  onSent,
}: {
  dashboardId: string
  dashboardTitle: string
  panel: ChatPanel | null
  onSent: (request: ObserveRequest) => void
}) {
  const model = useComposer({
    subject: { kind: "dashboard", id: dashboardId },
    place: { kind: "chat", chatId: null },
    target: panelTarget(panel),
    gate: ungated,
    onSent: (sent) => (sent.via === "chat" ? onSent(sent.request) : undefined),
  })
  useFocusOnPanel(panel, model.field)
  return (
    <div className="flex min-h-0 flex-1 flex-col justify-end gap-3 pb-1">
      <p className="text-[13px] text-muted-foreground">
        Ask the agent about {dashboardTitle}. When a change fits, the answer
        offers it with Revise the dashboard.
      </p>
      <div>
        <Composer
          model={model}
          layout="page"
          rows={2}
          autoFocus
          contextChips={[
            { id: "dashboard", label: dashboardTitle },
            ...panelChips(panel),
          ]}
          placeholder={`Ask about ${dashboardTitle}`}
        />
        <p className="mt-1.5 text-center text-xs text-muted-foreground">
          Only you and Admins can see this chat.
        </p>
      </div>
    </div>
  )
}

function ElsewhereNote({ chatId }: { chatId: string }) {
  return (
    <p className="flex-none border-t py-3 text-[13px] text-muted-foreground">
      This chat is not about this dashboard.{" "}
      <Link
        to="/c/$requestId"
        params={{ requestId: chatId }}
        className="text-foreground underline underline-offset-4"
      >
        Continue it on the chat page
      </Link>
    </p>
  )
}

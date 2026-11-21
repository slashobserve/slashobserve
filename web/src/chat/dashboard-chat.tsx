import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

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
import type { ObserveRequest } from "@/observe/queries"

import { ChatComposer } from "./chat-composer"
import { chatOptionMeta } from "./chat-rows"
import { useChat, useChats, type Chat } from "./chats-queries"
import { Conversation } from "./conversation"

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
}: Props) {
  const navigate = useNavigate()
  const now = useNow()
  const fresh = chatId === newChatId
  const { chats, query } = useChats({ dashboardId }, listedChats)
  const openId = fresh ? undefined : (chatId ?? chats.at(0)?.id)
  const open = useChat(openId)
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

  const waiting = query.data === undefined && chatId === undefined

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
        {waiting ? (
          <Skeleton className="h-24 rounded-card" />
        ) : openId === undefined ? (
          <NewChat
            dashboardId={dashboardId}
            dashboardTitle={dashboardTitle}
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
  onSent,
}: {
  dashboardId: string
  dashboardTitle: string
  onSent: (request: ObserveRequest) => void
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col justify-end gap-3 pb-1">
      <p className="text-[13px] text-muted-foreground">
        Ask the agent about {dashboardTitle}, or for a change to it. Only you
        can see this chat.
      </p>
      <ChatComposer
        rows={2}
        autoFocus
        dashboardId={dashboardId}
        dashboardTitle={dashboardTitle}
        placeholder={`Ask about ${dashboardTitle}, or for a change to it`}
        onSent={onSent}
      />
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

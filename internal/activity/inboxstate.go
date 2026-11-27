package activity

import "github.com/slashobserve/slashobserve/internal/lifecycle"

type ItemState string

const (
	ItemDone   ItemState = "done"
	ItemUnread ItemState = "unread"
	ItemRead   ItemState = "read"
)

var InboxItemStates = lifecycle.Derived{Name: "inbox item", States: []lifecycle.DerivedState{
	{Name: string(ItemDone), When: "The reader marked it done; it stays done when the inbox is read, until it is brought back or marked unread."},
	{Name: string(ItemUnread), When: "It is newer than the position the reader read up to, or the reader marked it unread."},
	{Name: string(ItemRead), When: "None of the above."},
}}

func (i InboxItem) State() ItemState {
	switch {
	case i.Done:
		return ItemDone
	case i.Unread:
		return ItemUnread
	}
	return ItemRead
}

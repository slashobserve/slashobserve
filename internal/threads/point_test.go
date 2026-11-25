package threads

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

const (
	firstBlock  = "0f7d1c2e-3a4b-4c5d-8e6f-a1b2c3d4e5f6"
	secondBlock = "9a8b7c6d-5e4f-4a3b-9c2d-1e0f9a8b7c6d"
)

func TestPointOfMessageUsesTheMessagesOwnBlockAndDropsAnAnchorOfAnotherBlock(t *testing.T) {
	spot := &anchor.Anchor{Kind: anchor.Text, Quote: "p99 doubled"}
	cases := []struct {
		name         string
		thread       Thread
		message      Message
		wantBlock    string
		wantAnchored bool
	}{
		{"a thread with nothing", Thread{}, Message{}, "", false},
		{"the thread's block when the message names none", Thread{BlockID: firstBlock, Anchor: spot}, Message{}, firstBlock, true},
		{"the same block as the thread", Thread{BlockID: firstBlock, Anchor: spot}, Message{BlockID: firstBlock}, firstBlock, true},
		{"a reply on another block than the thread's", Thread{BlockID: firstBlock, Anchor: spot}, Message{BlockID: secondBlock}, secondBlock, false},
		{"a reply on a block in a page thread", Thread{}, Message{BlockID: secondBlock}, secondBlock, false},
		{"a dashboard thread's anchor with no block", Thread{Anchor: spot}, Message{}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			point, err := pointOfMessage(tc.thread, tc.message)
			if err != nil {
				t.Fatal(err)
			}
			block := ""
			if point.BlockID.Valid {
				block = point.BlockID.String()
			}
			if block != tc.wantBlock {
				t.Errorf("block %q, want %q", block, tc.wantBlock)
			}
			if anchored := len(point.Anchor) > 0; anchored != tc.wantAnchored {
				t.Errorf("anchor %s, want anchored %v", point.Anchor, tc.wantAnchored)
			}
			if tc.wantAnchored && !json.Valid(point.Anchor) {
				t.Errorf("anchor %s is not JSON", point.Anchor)
			}
		})
	}
}

func TestPointOfMessageRefusesABlockThatIsNotAnID(t *testing.T) {
	if _, err := pointOfMessage(Thread{}, Message{BlockID: "nope"}); err == nil {
		t.Fatal("a block id that is not a UUID was accepted")
	}
}

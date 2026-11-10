package events

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNoticeMarshalsSeqAloneALiveEventAloneOrBothMerged(t *testing.T) {
	live := &Event{Type: TypeVersionCreated, DashboardID: "d1", Version: 2}
	tests := []struct {
		name string
		in   notice
		want string
	}{
		{"seq alone", notice{Seq: 3}, `{"seq":3}`},
		{"live event alone", notice{Event: live}, `{"type":"version.created","dashboard_id":"d1","version":2}`},
		{"both merged", notice{Seq: 3, Event: live}, `{"seq":3,"type":"version.created","dashboard_id":"d1","version":2}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNoticeUnmarshalsEachShape(t *testing.T) {
	var seqOnly notice
	if err := json.Unmarshal([]byte(`{"seq":7}`), &seqOnly); err != nil {
		t.Fatal(err)
	}
	if seqOnly.Seq != 7 || seqOnly.Event != nil {
		t.Fatalf("seq only: %+v", seqOnly)
	}

	var both notice
	if err := json.Unmarshal([]byte(`{"seq":7,"type":"version.created","dashboard_id":"d1"}`), &both); err != nil {
		t.Fatal(err)
	}
	if both.Seq != 7 || both.Event == nil || both.Type != TypeVersionCreated || both.DashboardID != "d1" {
		t.Fatalf("both: %+v", both)
	}
}

func TestNotifyRefusesANoticeWithNeitherSeqNorEvent(t *testing.T) {
	for _, seq := range []int64{0, -1} {
		if err := Notify(context.Background(), nil, seq, nil); err == nil {
			t.Errorf("seq %d: expected an error", seq)
		}
	}
}

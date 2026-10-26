package proxydhcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecorderSequencesSurviveEvictionAndClear(t *testing.T) {
	recorder := NewRecorder(2)
	subscriber := recorder.Subscribe()
	defer recorder.Unsubscribe(subscriber)
	for count := uint64(1); count <= 3; count++ {
		recorder.Record(Event{Sequence: 99, MAC: "aa:bb:cc:dd:ee:01"})
		select {
		case event := <-subscriber:
			if event.Sequence != count {
				t.Fatalf("subscriber sequence=%d, want %d", event.Sequence, count)
			}
		default:
			t.Fatal("subscriber missed an event")
		}
	}
	events := recorder.Events()
	if len(events) != 2 || events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("eviction changed sequences: %+v", events)
	}
	recorder.Clear()
	recorder.Record(Event{MAC: "aa:bb:cc:dd:ee:01"})
	events = recorder.Events()
	if len(events) != 1 || events[0].Sequence != 4 {
		t.Fatalf("clear reused a sequence: %+v", events)
	}
	encoded, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "sequence") {
		t.Fatal("internal sequence leaked into event JSON")
	}
}

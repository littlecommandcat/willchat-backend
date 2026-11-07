package core

import (
	"strings"
	"testing"
)

func TestPresence(t *testing.T) {
	h := NewHub()
	alice := h.Register("1", []string{"g"})
	bob := h.Register("2", []string{"g"})
	if msg := string(<-alice.Send); !strings.Contains(msg, "member_online") || !strings.Contains(msg, "mr-2") {
		t.Fatalf("alice should see bob online, got %s", msg)
	}
	if got := h.OnlineFor("1", []string{"g"}); len(got) != 1 || got[0] != "mr-2" {
		t.Fatalf("OnlineFor = %v", got)
	}
	second := h.Register("2", []string{"g"}) // second tab: no new event
	h.Unregister(bob)
	if len(alice.Send) != 0 {
		t.Fatal("bob still has a tab open, no offline expected")
	}
	h.Unregister(second)
	if msg := string(<-alice.Send); !strings.Contains(msg, "member_offline") {
		t.Fatalf("expected offline, got %s", msg)
	}
}

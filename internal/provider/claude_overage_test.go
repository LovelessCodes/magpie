package provider

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A usage-based seat's /usage tells only how the subscription is billed
// ("You are currently using your subscription…"), no window. What Claude
// Code said of its credits as it answered is that account's allowance, and
// stands past claudeHeard: the credits' own reset starts them again.
func TestClaudeOverageSeatKeepsItsCredits(t *testing.T) {
	var out atomic.Value
	out.Store(subscriptionNotice)
	var fail atomic.Bool
	fail.Store(false)
	fakeClaudeUsage(t, &out, &fail)
	defer func() {
		claudeUsage.Lock()
		claudeUsage.m = nil
		claudeUsage.Unlock()
	}()

	reset := time.Now().Add(72 * time.Hour).Truncate(time.Second)
	NoteClaudeLimits("over@example.com", []ClaudeLimit{{Kind: "overage", Used: 0.84, ResetsAt: reset.Unix()}})

	// the credits are kept, and are the account's allowance
	claudeUsage.Lock()
	e, kept := claudeUsage.m["over@example.com"]
	claudeUsage.Unlock()
	if !kept || len(e.ws) != 1 || e.ws[0].Name != claudeOverageName {
		t.Fatalf("the credits Claude Code named weren't kept: %+v", e.ws)
	}

	// the word is hours old, and /usage runs again and tells no window
	claudeUsage.Lock()
	e = claudeUsage.m["over@example.com"]
	e.heard, e.tried = time.Now().Add(-3*time.Hour), time.Now().Add(-time.Hour)
	claudeUsage.m["over@example.com"] = e
	claudeUsage.Unlock()

	AskClaudeUsage()
	ws, err := claudeWindows(context.Background(), "over@example.com", true)
	if err != nil {
		t.Fatalf("the credits went with a /usage that tells no window: %v", err)
	}
	if len(ws) != 1 || ws[0].Name != "Extra usage" || ws[0].Used != 84 || ws[0].Span != 0 ||
		ws[0].ResetsAt == nil || !ws[0].ResetsAt.Equal(reset) {
		t.Fatalf("windows: %+v", ws)
	}

	// a window the credits' reset has passed reads as started again
	claudeUsage.Lock()
	e = claudeUsage.m["over@example.com"]
	e.ws[0].ResetsAt = ptr(time.Now().Add(-time.Minute))
	claudeUsage.m["over@example.com"] = e
	claudeUsage.Unlock()
	AskClaudeUsage()
	if ws, err := claudeWindows(context.Background(), "over@example.com", true); err != nil || len(ws) != 1 || ws[0].Used != 0 || ws[0].ResetsAt != nil {
		t.Fatalf("after the credits' reset: %+v %v", ws, err)
	}
}

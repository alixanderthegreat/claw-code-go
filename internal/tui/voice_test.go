package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"

	"claw-code-go/internal/runtime"
)

func voiceModel(mode string) Model {
	return Model{textarea: textarea.New(), voice: newVoiceState(&runtime.Config{BaseURL: "http://127.0.0.1:1/v1", VoiceMode: mode})}
}

// TestTranscriptJoinsTheDraft: a transcript is added to what's already typed, not put in its place,
// and marks the draft as spoken.
func TestTranscriptJoinsTheDraft(t *testing.T) {
	m := voiceModel("")
	m.textarea.SetValue("look at")
	mm, _, ok := m.handleVoiceMsg(voiceTranscriptMsg{text: "the compaction code"})
	if !ok {
		t.Fatal("transcript not handled")
	}
	got := mm.(Model)
	if got.textarea.Value() != "look at the compaction code" || !got.voice.draftSpoken {
		t.Errorf("input = %q, spoken = %v", got.textarea.Value(), got.voice.draftSpoken)
	}
}

// TestWhenRepliesAreSpoken: auto speaks a reply to a spoken message only; on always; off never.
// What's said is the text after the last tool call.
func TestWhenRepliesAreSpoken(t *testing.T) {
	for _, c := range []struct {
		mode   string
		spoken bool
		want   bool
	}{
		{"auto", true, true}, {"auto", false, false}, {"on", false, true}, {"off", true, false},
	} {
		m := voiceModel(c.mode)
		m.voice.draftSpoken = c.spoken
		m.voiceTurnStarted()
		m.voice.reply = "Done. Fixed it."
		if got := m.speakReplyCmd() != nil; got != c.want {
			t.Errorf("mode %s, spoken %v: speaks = %v, want %v", c.mode, c.spoken, got, c.want)
		}
		if m.voice.turnSpoken || m.voice.reply != "" {
			t.Errorf("mode %s: turn state not reset after the reply", c.mode)
		}
	}

	m := voiceModel("on")
	m.voiceTurnStarted()
	m.voice.reply = "```\ncode only\n```"
	if m.speakReplyCmd() != nil {
		t.Errorf("a reply with nothing to say aloud shouldn't start the speaker")
	}
}

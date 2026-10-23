package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/runtime"
	"claw-code-go/internal/voice"
)

// voice.go: speech as a way to talk to Gordi. Ctrl+R starts and stops recording; the transcript
// lands in the input box to check before Enter. A reply to a message you spoke is said aloud
// ("auto"), or every reply ("on"), or none ("off") - /voice switches. Esc stops speech.

// voiceState is shared by every copy of the Model, so the recorder and speaker outlive the value
// receivers Bubble Tea passes around.
type voiceState struct {
	client  *voice.Client
	speaker *voice.Speaker
	mode    string // auto | on | off

	rec          *voice.Recording // non-nil while recording
	transcribing bool
	draftSpoken  bool   // the input box holds a transcript
	turnSpoken   bool   // the message in flight was spoken
	reply        string // the reply's text since its last tool call - what gets said
}

type (
	voiceTranscriptMsg struct {
		text string
		err  error
	}
	voiceSpokenMsg struct{ err error }
)

func newVoiceState(cfg *runtime.Config) *voiceState {
	client := voice.NewClient(cfg.BaseURL, cfg.APIKey, cfg.STTModel)
	mode := cfg.VoiceMode
	if mode != "on" && mode != "off" {
		mode = "auto"
	}
	return &voiceState{client: client, speaker: voice.NewSpeaker(client), mode: mode}
}

// toggleRecording is Ctrl+R: start recording, or stop and transcribe.
func (m Model) toggleRecording() (tea.Model, tea.Cmd) {
	v := m.voice
	if v.transcribing {
		return m, nil
	}
	if v.rec == nil {
		v.speaker.Stop() // don't record our own voice
		rec, err := voice.StartRecording()
		if err != nil {
			m.viewBuf += renderBlock(errorStyle, "Voice: "+err.Error())
			m = m.refreshViewport()
			return m, nil
		}
		v.rec = rec
		return m, nil
	}
	path, err := v.rec.Stop()
	v.rec = nil
	if err != nil {
		m.viewBuf += renderBlock(errorStyle, "Voice: "+err.Error())
		m = m.refreshViewport()
		return m, nil
	}
	v.transcribing = true
	client := v.client
	return m, func() tea.Msg {
		defer os.Remove(path)
		text, err := client.Transcribe(context.Background(), path)
		return voiceTranscriptMsg{text: text, err: err}
	}
}

// handleVoiceMsg handles voice's own messages; ok is false for any other message.
func (m Model) handleVoiceMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case voiceTranscriptMsg:
		m.voice.transcribing = false
		switch {
		case msg.err != nil:
			m.viewBuf += renderBlock(errorStyle, "Voice: "+msg.err.Error())
			m = m.refreshViewport()
		case msg.text == "":
			m.viewBuf += renderBlock(warnStyle, "Voice: no speech heard")
			m = m.refreshViewport()
		default:
			draft := strings.TrimRight(m.textarea.Value(), " ")
			if draft != "" {
				draft += " "
			}
			m.textarea.SetValue(draft + msg.text)
			m.textarea.CursorEnd()
			m.voice.draftSpoken = true
		}
		return m, nil, true
	case voiceSpokenMsg:
		if msg.err != nil {
			m.viewBuf += renderBlock(errorStyle, "Voice: "+msg.err.Error())
			m = m.refreshViewport()
		}
		return m, nil, true
	}
	return m, nil, false
}

// voiceTurnStarted marks whether the message just sent was spoken.
func (m Model) voiceTurnStarted() {
	m.voice.turnSpoken = m.voice.draftSpoken
	m.voice.draftSpoken = false
	m.voice.reply = ""
}

// speakReplyCmd says the finished reply, if this turn calls for it.
func (m Model) speakReplyCmd() tea.Cmd {
	v := m.voice
	speak := v.mode == "on" || (v.mode == "auto" && v.turnSpoken)
	text := voice.Prose(v.reply)
	v.turnSpoken, v.reply = false, ""
	if !speak || text == "" {
		return nil
	}
	speaker := v.speaker
	return func() tea.Msg { return voiceSpokenMsg{err: speaker.Say(text)} }
}

// handleVoiceCommand is /voice [auto|on|off].
func (m Model) handleVoiceCommand(parts []string) (tea.Model, tea.Cmd) {
	v := m.voice
	if len(parts) > 1 {
		switch parts[1] {
		case "auto", "on", "off":
			v.mode = parts[1]
			if v.mode == "off" {
				v.speaker.Stop()
			}
		default:
			m.viewBuf += renderBlock(errorStyle, "usage: /voice [auto|on|off]")
			m = m.refreshViewport()
			return m, nil
		}
	}
	desc := map[string]string{
		"auto": "replies to messages you spoke are said aloud",
		"on":   "every reply is said aloud",
		"off":  "nothing is said aloud",
	}[v.mode]
	m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Voice: %s - %s. Ctrl+R records, Esc stops speech.", v.mode, desc))
	m = m.refreshViewport()
	return m, nil
}

// voiceStatus is the status bar's voice segment, empty when idle.
func (m Model) voiceStatus() string {
	v := m.voice
	switch {
	case v.rec != nil:
		return "● Recording - Ctrl+R to stop"
	case v.transcribing:
		return "Transcribing…"
	case v.speaker.Speaking():
		return "Speaking - Esc to stop"
	}
	return ""
}

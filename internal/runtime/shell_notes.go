package runtime

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// shell_notes.go: what the user ran themselves with "!" in the TUI. The output used to go to the
// transcript only, so the model never knew: "good" after "! calendar list" got an answer that hadn't
// seen the calendar. Now each command and its output wait here and go out at the head of the user's
// next message, as Claude Code does - a "!" alone doesn't start a turn.

// maxShellNoteChars bounds one command's output in the note; the user saw all of it already.
const maxShellNoteChars = 8000

// NoteShellCommand records a command the user ran, for their next message.
func (loop *ConversationLoop) NoteShellCommand(command, output string, err error) {
	out := strings.TrimRight(output, "\n")
	if len(out) > maxShellNoteChars {
		cut := maxShellNoteChars
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = out[:cut] + fmt.Sprintf("\n... [%d more bytes]", len(out)-cut)
	}
	note := "$ " + strings.TrimSpace(command)
	if out != "" {
		note += "\n" + out
	}
	if err != nil {
		note += "\n(" + err.Error() + ")"
	}
	loop.shellNotes = append(loop.shellNotes, note)
}

// takeShellNotes is the block the next user message starts with, or "", and clears it.
func (loop *ConversationLoop) takeShellNotes() string {
	if len(loop.shellNotes) == 0 {
		return ""
	}
	block := "I ran this in my shell since my last message:\n\n" + strings.Join(loop.shellNotes, "\n\n")
	loop.shellNotes = nil
	return block
}

package runtime

import (
	"context"
	"strings"
	"testing"
)

// TestShellCommandReachesTheModel: what the user ran with "!" goes out with their next message -
// "good" after "! calendar list" used to be answered by a model that never saw the calendar - once,
// ahead of what they say; /clear drops any still waiting.
func TestShellCommandReachesTheModel(t *testing.T) {
	client := &scriptedClient{}
	loop := newScriptedLoop(client, nil)
	loop.NoteShellCommand("calendar list", "2026-10-24 - The Porcaro Halloween Party\n", nil)

	loop.SendMessage(context.Background(), "good")
	loop.SendMessage(context.Background(), "and tomorrow?")
	var users []string
	for _, m := range loop.Session.Messages {
		if m.Role == "user" && m.Content[0].Type == "text" {
			users = append(users, m.Content[0].Text)
		}
	}
	if len(users) != 2 || !strings.Contains(users[0], "$ calendar list\n2026-10-24 - The Porcaro Halloween Party") || !strings.HasSuffix(users[0], "good") {
		t.Fatalf("first message = %q", users)
	}
	if strings.Contains(users[1], "calendar list") {
		t.Errorf("the note went out twice: %q", users[1])
	}

	loop.NoteShellCommand("ls", "x", nil)
	loop.ClearSession()
	if loop.takeShellNotes() != "" {
		t.Errorf("/clear should drop waiting notes")
	}
}

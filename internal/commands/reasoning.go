package commands

import (
	"fmt"
	"strings"

	"claw-code-go/internal/runtime"
)

// RegisterReasoningCommand adds /mode: the session's {REASON:...} mode (runtime/reasoning.go).
func RegisterReasoningCommand(r *Registry) {
	r.Register(Command{
		Name:        "mode",
		Description: "Set this session's reasoning mode (e.g. /mode holodeck, /mode iholodeck, /mode default)",
		Handler: func(args string, loop interface{}) error {
			l, ok := loop.(*runtime.ConversationLoop)
			if !ok {
				return fmt.Errorf("mode: no conversation loop")
			}
			if strings.TrimSpace(args) == "" {
				fmt.Println(describeMode(l.ReasoningMode()))
				fmt.Println("Modes: " + strings.Join(runtime.ReasoningModes, ", ") + ` (an "i" in front for instruct)`)
				return nil
			}
			mode, err := l.SetReasoningMode(args)
			if err != nil {
				return err
			}
			fmt.Println(describeMode(mode))
			return nil
		},
	})
}

func describeMode(mode string) string {
	if mode == "" {
		return "Reasoning mode: the model's default"
	}
	return "Reasoning mode: " + mode
}

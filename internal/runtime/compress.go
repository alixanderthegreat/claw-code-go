package runtime

import "claw-code-go/internal/tools"

// compressInput is a bash call's input with its command as rtk would run it, and true, when
// CompressOutput is on and rtk has a version of the command (greatproject #648). The model's own
// input is left as it gave it: the session and the TUI show what it asked for, and only the run
// goes through rtk.
func (loop *ConversationLoop) compressInput(name string, input map[string]any) (map[string]any, bool) {
	if name != "bash" || loop.Config == nil || !loop.Config.CompressOutput {
		return input, false
	}
	command, _ := input["command"].(string)
	if command == "" {
		return input, false
	}
	rewritten, ok := tools.CompressCommand(command)
	if !ok {
		return input, false
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	out["command"] = rewritten
	return out, true
}

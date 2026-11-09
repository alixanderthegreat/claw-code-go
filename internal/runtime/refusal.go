package runtime

import (
	"fmt"

	"claw-code-go/internal/permissions"
)

// refusedBy says who refused a tool call. The model is told, because "permission denied" alone
// reads as an obstacle to get around, when a deny rule is a fence: the action it guards stays
// with the operator (#30).
type refusedBy int

const (
	refusedByRule     refusedBy = iota // a deny rule, or the tool is blocked outright
	refusedUnattended                  // it needed asking, and no one was there to ask
	refusedByOperator                  // the operator said no at the prompt
)

// fencedAction names what was refused, as narrowly as the rule allows: "bash commands
// containing \"git push\"" rather than just "bash", so the model knows which action is fenced
// and which calls of the same tool are still fine.
func fencedAction(tool string, rule *permissions.Rule) string {
	if rule == nil || rule.Pattern == "" {
		return fmt.Sprintf("the %s tool", tool)
	}
	return fmt.Sprintf("%s calls containing %q", tool, rule.Pattern)
}

// refusalText is the tool result the model reads when a call is refused. It says who refused,
// what is fenced (fencedAction), and what to do now: not retry it, and not reach the same result
// another way - another command, a script, a different tool.
func refusalText(by refusedBy, tool string, rule *permissions.Rule) string {
	what := fencedAction(tool, rule)
	switch by {
	case refusedByRule:
		return fmt.Sprintf("Refused by a permission rule: %s stay with the operator. "+
			"Don't retry it or get the same result another way (another command, a script, a different tool). "+
			"Tell the operator what you wanted to do and why, then carry on with the rest.", what)
	case refusedUnattended:
		return fmt.Sprintf("Not run: %s needed the operator's permission, and no one was there to ask. "+
			"Don't try it another way. Say in your answer what you needed allowed and why, "+
			"so the operator can add a rule or run it.", what)
	case refusedByOperator:
		return fmt.Sprintf("The operator declined this call (%s). Don't retry it or work around it.", what)
	}
	return fmt.Sprintf("Permission denied for tool: %s (%s)", tool, what)
}

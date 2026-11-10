package permissions

import (
	"regexp"
	"strings"
)

// What an "always" answer covers (#28). It used to be the whole tool: "always" on `git status`
// allowed every bash command after it, `rm -rf` and `git push` included. For bash it's now the
// kind of command - its first word, or its first two for tools whose second word is the action
// (`git status`, `go test`) - and every kind in a pipeline or list must have been granted. For any
// other tool it's still the tool: a file tool's path is the call, not a kind.
//
// Like the rules, this guards against mistakes, not against someone hiding a command on purpose.

// subcommandTools are commands whose second word says what they do.
var subcommandTools = map[string]bool{
	"git": true, "go": true, "tea": true, "gh": true, "docker": true, "npm": true,
	"cargo": true, "systemctl": true, "kubectl": true, "apt": true, "make": true,
}

// wrappers run another command given as their arguments: granting one would grant anything.
var wrappers = map[string]bool{
	"sudo": true, "env": true, "xargs": true, "sh": true, "bash": true, "zsh": true, "eval": true,
	"exec": true, "timeout": true, "nohup": true, "time": true, "nice": true, "watch": true,
	"ssh": true, "parallel": true, "command": true, "builtin": true, "source": true, ".": true,
}

var (
	harmlessRedirect = regexp.MustCompile(`[0-9]?>&[0-9]|&?[0-9]?>>?\s*/dev/null`)
	listSeparator    = regexp.MustCompile(`\|\||&&|[;|&\n]`)
	assignment       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// Kinds says what an "always" for this call would cover: the kinds of command for bash, the tool
// otherwise. When it can't be remembered, kinds is nil and why says so.
func Kinds(tool, input string) (kinds []string, why string) {
	if tool != "bash" {
		return []string{tool}, ""
	}
	switch {
	case strings.Contains(input, "$("), strings.Contains(input, "`"),
		strings.Contains(input, "<("), strings.Contains(input, ">("):
		return nil, "it runs a command inside a command"
	}
	input = harmlessRedirect.ReplaceAllString(input, " ")
	if strings.Contains(input, ">") {
		return nil, "it writes to a file"
	}
	seen := map[string]bool{}
	for _, seg := range listSeparator.Split(input, -1) {
		f := strings.Fields(seg)
		if len(f) == 0 {
			continue
		}
		k, why := kind(f)
		if why != "" {
			return nil, why
		}
		if !seen[k] {
			seen[k] = true
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		return nil, "there's no command"
	}
	return kinds, ""
}

// kind is one simple command's kind, from its words.
func kind(f []string) (string, string) {
	switch {
	case assignment.MatchString(f[0]):
		return "", "it sets a variable for the command"
	case wrappers[f[0]]:
		return "", f[0] + " runs whatever follows it"
	case f[0] == "find":
		for _, w := range f[1:] {
			if w == "-exec" || w == "-execdir" || w == "-ok" || w == "-delete" {
				return "", "find " + w + " runs or deletes"
			}
		}
	case subcommandTools[f[0]]:
		if len(f) < 2 || strings.HasPrefix(f[1], "-") {
			return "", f[0] + "'s action isn't its second word"
		}
		return f[0] + " " + f[1], ""
	}
	return f[0], ""
}

// Covers says, for the permission box, what "always" would allow for the rest of the session.
func Covers(tool, input string) string {
	kinds, why := Kinds(tool, input)
	if kinds == nil {
		return "always isn't offered: " + why
	}
	if tool != "bash" {
		return "always: every " + tool + " call"
	}
	return "always: " + strings.Join(kinds, ", ")
}

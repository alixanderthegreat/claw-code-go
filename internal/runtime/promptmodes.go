package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// promptmodes.go: the modes a model's template can't read from a tag, given as a system prompt
// instead (greatproject #441) - the way consult runs joker on the 9B. The prompts are consult's:
// claw-code-go asks consult for one when /mode sets it and keeps it in memory, so no copy of them
// is ever written here.

// ModeSource is where the prompt modes come from: consult's GET /modes and /modes/NAME/prompt,
// through the gateway (or CONSULT_URL). Answers are kept for the process; a failure isn't.
type ModeSource struct {
	mu      sync.Mutex
	base    string
	http    *http.Client
	names   []string
	prompts map[string]string
}

// consultModes is the process's mode source, pointed at consult when a loop starts.
var consultModes = &ModeSource{http: &http.Client{Timeout: 5 * time.Second}}

// SetBase points the source at consult's URL. An empty base turns the prompt modes off.
func (s *ModeSource) SetBase(base string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base = strings.TrimRight(base, "/")
	if s.base != base {
		s.base, s.names, s.prompts = base, nil, nil
	}
}

// consultBase is CONSULT_URL, or the gateway in the provider's base URL with /consult on it - as
// the consult command finds it.
func consultBase(baseURL string) string {
	if u := os.Getenv("CONSULT_URL"); u != "" {
		return u
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/consult"
}

// Names are consult's modes, sorted; nil when consult can't be reached.
func (s *ModeSource) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names != nil || s.base == "" {
		return s.names
	}
	resp, err := s.http.Get(s.base + "/modes")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var modes map[string]string
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&modes) != nil {
		return nil
	}
	for m := range modes {
		s.names = append(s.names, m)
	}
	sort.Strings(s.names)
	return s.names
}

// Prompt is a mode's prompt as consult sends it: the system message, less its "introduce
// yourself" task (#438).
func (s *ModeSource) Prompt(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.prompts[name]; ok {
		return p, nil
	}
	if s.base == "" {
		return "", fmt.Errorf("no consult to ask for %s's prompt: set CONSULT_URL", name)
	}
	resp, err := s.http.Get(s.base + "/modes/" + url.PathEscape(name) + "/prompt")
	if err != nil {
		return "", fmt.Errorf("consult couldn't be reached for %s's prompt: %w", name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("consult has no prompt for %s (%s)", name, resp.Status)
	}
	if s.prompts == nil {
		s.prompts = map[string]string{}
	}
	s.prompts[name] = strings.TrimSpace(string(body))
	return s.prompts[name], nil
}

// promptModes are the modes model gets as a system prompt: consult's modes its template doesn't
// read from a tag.
func promptModes(model string) []string {
	tags, _ := modesFor(model)
	inTags := map[string]bool{}
	for _, t := range tags {
		inTags[t] = true
	}
	var out []string
	for _, m := range consultModes.Names() {
		if !inTags[m] {
			out = append(out, m)
		}
	}
	return out
}

// promptModeName is mode's name in consult, without the instruct "i", when mode reaches model as
// a system prompt rather than a tag; "" otherwise.
func promptModeName(model, mode string) string {
	if mode == "" {
		return ""
	}
	if tags, _ := modesFor(model); tags != nil {
		if _, ok := parseMode(mode, tags); ok {
			return ""
		}
	}
	for _, m := range promptModes(model) {
		if m == mode || "i"+m == mode {
			return m
		}
	}
	return ""
}

// promptModeText is the session's mode as a system prompt, "" when the mode is a tag or none, or
// consult can't be reached (the mode was checked when it was set).
func (loop *ConversationLoop) promptModeText() string {
	name := promptModeName(loop.Config.Model, loop.ReasoningMode())
	if name == "" {
		return ""
	}
	p, err := consultModes.Prompt(name)
	if err != nil {
		return ""
	}
	return p
}

// promptModeInstruct is whether the session's prompt mode is in instruct form: thinking off.
func (loop *ConversationLoop) promptModeInstruct() bool {
	mode := loop.ReasoningMode()
	name := promptModeName(loop.Config.Model, mode)
	return name != "" && mode == "i"+name
}

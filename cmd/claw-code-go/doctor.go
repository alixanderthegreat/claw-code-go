package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"claw-code-go/internal/config"
	ctxpkg "claw-code-go/internal/context"
	"claw-code-go/internal/handoff"
	"claw-code-go/internal/runtime"
	"claw-code-go/internal/suggestions"
	"claw-code-go/internal/voice"
)

// doctor.go: `claw-code-go doctor` - what this machine is set up for, and whether each part that's
// on works: the config, the model, voice, the instruction files, the tools. A feature that isn't
// configured is off, not broken (#21): a phone without a mic says "voice off", not an error.

// check is one line of the report.
type check struct {
	name, status, detail string // status: ok, off, warn or FAIL
}

func runDoctor(args []string) {
	if len(args) == 1 && args[0] == "suggestions" {
		if err := suggestionsReport(os.Stdout, suggestions.Path(), time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: claw-code-go doctor [suggestions]")
		os.Exit(2)
	}
	checks := doctor()
	failed := false
	for _, c := range checks {
		fmt.Printf("%-13s %-5s %s\n", c.name, c.status, c.detail)
		failed = failed || c.status == "FAIL"
	}
	if failed {
		os.Exit(1)
	}
}

func doctor() []check {
	var out []check
	path, _ := config.GlobalConfigPath()

	switch _, err := os.Stat(path); {
	case os.IsNotExist(err):
		out = append(out, check{"config", "FAIL", path + " doesn't exist: run claw-code-go once to write a starter"})
		return out
	case config.Validate() != nil:
		out = append(out, check{"config", "FAIL", config.Validate().Error()})
		return out
	}
	if open, mode, err := config.WorldReadable(); err == nil && open {
		out = append(out, check{"config", "warn", fmt.Sprintf("%s is readable by others (%v): chmod 600 it", path, mode)})
	} else {
		out = append(out, check{"config", "ok", path})
	}

	cfg := runtime.LoadConfig()
	out = append(out, modelCheck(cfg.BaseURL, cfg.APIKey, cfg.Model, http.DefaultClient))

	if cfg.VoiceMode == "" {
		out = append(out, check{"voice", "off", "voice_mode isn't set"})
	} else if rec, player, err := voice.Check(); err != nil {
		out = append(out, check{"voice", "FAIL", fmt.Sprintf("voice_mode is %q, but %v", cfg.VoiceMode, err)})
	} else {
		speaks := cfg.SpeechVoice
		if speaks == "" {
			speaks = "the service's default (Mary's): set speech_voice"
		}
		out = append(out, check{"voice", "ok", fmt.Sprintf("%s: records with %s, plays with %s, speaks as %s", cfg.VoiceMode, rec, player, speaks)})
	}

	wd, _ := os.Getwd()
	files := ctxpkg.StableSections(wd).InstructionFiles
	if len(files) == 0 {
		out = append(out, check{"instructions", "off", "no CLAUDE.md, AGENTS.md or GORDI.md applies here"})
	} else {
		out = append(out, check{"instructions", "ok", strings.Join(files, ", ")})
	}

	out = append(out, toolsCheck(ctxpkg.Tools()))
	out = append(out, handoffCheck(handoff.Dir(), wd))
	return out
}

// handoffCheck says whether /handoff has anything here. No condenser on this machine is off, not
// broken: the handoffs come from a nightly run that only some machines have.
func handoffCheck(dir, wd string) check {
	if _, err := os.Stat(dir); err != nil {
		return check{"handoff", "off", dir + " doesn't exist: no condensed Claude Code sessions on this machine"}
	}
	all, err := handoff.List(dir, wd)
	if err != nil {
		return check{"handoff", "FAIL", err.Error()}
	}
	if len(all) == 0 {
		return check{"handoff", "ok", "none for this folder yet (" + dir + ")"}
	}
	return check{"handoff", "ok", fmt.Sprintf("%d for this folder, the newest %s: /handoff loads it", len(all), handoff.Describe(all[0]))}
}

// modelCheck asks the server for its models and looks for the configured one.
func modelCheck(baseURL, apiKey, model string, client *http.Client) check {
	if baseURL == "" {
		return check{"model", "off", "no base_url: the model is Anthropic's or OpenAI's, by /login"}
	}
	if model == "" {
		return check{"model", "FAIL", "base_url is set but model isn't"}
	}
	url := strings.TrimSuffix(baseURL, "/")
	if strings.HasSuffix(url, "/v1") {
		url += "/models"
	} else {
		url += "/v1/models"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return check{"model", "FAIL", err.Error()}
	}
	if apiKey != "" && apiKey != "not-needed" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if client.Timeout == 0 {
		c := *client
		c.Timeout = 10 * time.Second
		client = &c
	}
	resp, err := client.Do(req)
	if err != nil {
		// No answer in time isn't the same as no server: one that takes a request at a time
		// holds even its model list behind a long generation. Down is FAIL; slow is a warning.
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return check{"model", "warn", fmt.Sprintf("%s didn't list its models in %s: busy, or not answering; run doctor again", baseURL, client.Timeout)}
		}
		return check{"model", "FAIL", fmt.Sprintf("can't reach %s: %v", baseURL, err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return check{"model", "FAIL", fmt.Sprintf("%s answered %s", url, resp.Status)}
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return check{"model", "FAIL", fmt.Sprintf("%s didn't answer with a model list", url)}
	}
	for _, m := range list.Data {
		if m.ID == model {
			return check{"model", "ok", model + " at " + baseURL}
		}
	}
	return check{"model", "FAIL", fmt.Sprintf("%s isn't among the %d models at %s", model, len(list.Data), baseURL)}
}

// toolsCheck reports the tools the machine's GORDI.md lists: off when it lists none, warn when some
// aren't on PATH (the model is told which), ok when all are.
func toolsCheck(tools []ctxpkg.Tool) check {
	if len(tools) == 0 {
		return check{"tools", "off", "the machine's GORDI.md (" + ctxpkg.MachineFile() + ") lists none"}
	}
	var missing []string
	for _, t := range tools {
		if !t.OnPath {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) > 0 {
		return check{"tools", "warn", fmt.Sprintf("%d of %d on PATH; not here: %s", len(tools)-len(missing), len(tools), strings.Join(missing, ", "))}
	}
	return check{"tools", "ok", fmt.Sprintf("all %d on PATH", len(tools))}
}

// suggestionsReport is `doctor suggestions` (#50): how the next-prompt suggestions fared, from their
// log - accepted, edited or ignored, all of them and split by whether they looked outward - and
// whether the outward gate holds them now. No log yet is not an error.
func suggestionsReport(w io.Writer, path string, now time.Time) error {
	s, err := suggestions.Read(path, now)
	if errors.Is(err, os.ErrNotExist) || err == nil && s.All.Total() == 0 && s.Skipped == 0 {
		fmt.Fprintln(w, "no suggestions logged yet")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "suggestions: %d total (last 30 days: %d)", s.All.Total(), s.Recent)
	if s.Skipped > 0 {
		fmt.Fprintf(w, ", %d malformed lines skipped", s.Skipped)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)
	for _, row := range []struct {
		name string
		c    suggestions.Counts
	}{{"all:", s.All}, {"outward:", s.Outward}, {"inward:", s.Inward}} {
		c := row.c
		fmt.Fprintf(w, "%-9s %-20s %-20s %s\n", row.name,
			fmt.Sprintf("accepted %d (%.1f%%)", c.Accepted, c.Rate(c.Accepted)),
			fmt.Sprintf("edited %d (%.1f%%)", c.Edited, c.Rate(c.Edited)),
			fmt.Sprintf("ignored %d (%.1f%%)", c.Ignored, c.Rate(c.Ignored)))
	}
	_, why := suggestions.Held(s.Outward, s.Inward)
	fmt.Fprintln(w)
	fmt.Fprintln(w, why)
	if s.Probes > 0 {
		fmt.Fprintf(w, "probes: %d outward suggestions shown while held\n", s.Probes)
	}
	return nil
}

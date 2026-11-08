package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"claw-code-go/internal/config"
	ctxpkg "claw-code-go/internal/context"
	"claw-code-go/internal/runtime"
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
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: claw-code-go doctor")
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
		out = append(out, check{"voice", "ok", fmt.Sprintf("%s: records with %s, plays with %s", cfg.VoiceMode, rec, player)})
	}

	wd, _ := os.Getwd()
	files := ctxpkg.StableSections(wd).InstructionFiles
	if len(files) == 0 {
		out = append(out, check{"instructions", "off", "no CLAUDE.md, AGENTS.md or GORDI.md applies here"})
	} else {
		out = append(out, check{"instructions", "ok", strings.Join(files, ", ")})
	}

	out = append(out, toolsCheck(ctxpkg.Tools()))
	return out
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

// Command screenshot takes a screenshot for Gordi to look at, and prints its path:
//
//	screenshot        the whole screen
//	screenshot -w     the active window
//	screenshot -r     a region you drag out with the mouse
//	screenshot clean  delete every shot now
//
// read_file on the printed path gives the image to the model (claw-code-go's internal/tools/
// images.go). Shots go to $XDG_RUNTIME_DIR/screenshots - private to you and in memory, gone at
// logout - and each run deletes those older than 15 minutes and all but the newest few, so a
// screen's contents don't pile up anywhere. It shells out to xfce4-screenshooter on X11, or grim
// on Wayland.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	keepFor  = 15 * time.Minute
	keepLast = 3
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "screenshot:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dir, err := shotsDir()
	if err != nil {
		return err
	}
	mode := "-f"
	if len(args) > 0 {
		switch args[0] {
		case "clean":
			n := prune(dir, 0, 0, time.Now())
			fmt.Printf("deleted %d screenshots\n", n)
			return nil
		case "-w", "-r":
			mode = args[0]
		case "-h", "--help", "help":
			fmt.Println("usage: screenshot [-w | -r]   the whole screen, the active window, or a region you drag out\n       screenshot clean        delete every shot now")
			return nil
		default:
			return fmt.Errorf("unknown argument %q (screenshot -h)", args[0])
		}
	}
	path := filepath.Join(dir, "shot-"+time.Now().Format("150405")+".png")
	if err := capture(mode, path); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no screenshot was saved (was the region cancelled?)")
	}
	prune(dir, keepFor, keepLast, time.Now())
	fmt.Println(path)
	return nil
}

// shotsDir is $XDG_RUNTIME_DIR/screenshots, made private; without XDG_RUNTIME_DIR there's no
// memory-only place that's yours alone, so it refuses rather than write to disk.
func shotsDir() (string, error) {
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR isn't set, so there's no private in-memory place for screenshots")
	}
	dir := filepath.Join(rt, "screenshots")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, os.Chmod(dir, 0o700)
}

func capture(mode, path string) error {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if mode != "-f" {
			return fmt.Errorf("on Wayland only the whole screen is supported (grim)")
		}
		cmd = exec.Command("grim", path)
	case os.Getenv("DISPLAY") != "":
		cmd = exec.Command("xfce4-screenshooter", mode, "-s", path)
	default:
		return fmt.Errorf("no display: neither DISPLAY nor WAYLAND_DISPLAY is set")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v %s", cmd.Args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

// prune deletes shots older than maxAge and all but the newest keep (maxAge 0 and keep 0: all of
// them), returning how many went.
func prune(dir string, maxAge time.Duration, keep int, now time.Time) int {
	entries, _ := filepath.Glob(filepath.Join(dir, "shot-*.png"))
	type shot struct {
		path string
		mod  time.Time
	}
	var shots []shot
	for _, p := range entries {
		if info, err := os.Stat(p); err == nil {
			shots = append(shots, shot{p, info.ModTime()})
		}
	}
	sort.Slice(shots, func(i, j int) bool { return shots[i].mod.After(shots[j].mod) })
	n := 0
	for i, s := range shots {
		if (maxAge == 0 && keep == 0) || i >= keep || now.Sub(s.mod) > maxAge {
			if os.Remove(s.path) == nil {
				n++
			}
		}
	}
	return n
}

package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Players, in the order tried.
var players = []struct {
	name string
	args func(path string) []string
}{
	{"pw-play", func(p string) []string { return []string{p} }},
	{"aplay", func(p string) []string { return []string{"-q", p} }},
	{"mpv", func(p string) []string { return []string{"--no-video", "--really-quiet", p} }},
}

const (
	firstChunkLen = 160 // about one sentence: speech starts while the rest is made
	chunkLen      = 600
)

// Speaker says replies aloud, one at a time: a new Say or a Stop cuts off the one playing.
type Speaker struct {
	client *Client

	mu     sync.Mutex
	cancel context.CancelFunc // the Say in progress, nil when quiet
	gen    int                // counts Says, so one that finishes late can't clear a newer one
}

// NewSpeaker returns a Speaker that synthesizes through client.
func NewSpeaker(client *Client) *Speaker { return &Speaker{client: client} }

// Say speaks text and returns when it's done, stopped, or failed; stopping isn't an error. Each
// chunk is synthesized while the one before it plays.
func (s *Speaker) Say(text string) error {
	chunks := Chunks(text, firstChunkLen, chunkLen)
	if len(chunks) == 0 {
		return nil
	}
	player, err := findPlayer()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	defer s.finish(gen, cancel)

	type clip struct {
		path string
		err  error
	}
	clips := make(chan clip, 1)
	go func() {
		defer close(clips)
		for _, c := range chunks {
			wav, err := s.client.Synthesize(ctx, c)
			if err == nil {
				var path string
				path, err = writeTemp(wav)
				wav = nil
				select {
				case clips <- clip{path, err}:
				case <-ctx.Done():
					os.Remove(path)
					return
				}
			} else {
				select {
				case clips <- clip{err: err}:
				case <-ctx.Done():
				}
			}
			if err != nil {
				return
			}
		}
	}()

	for c := range clips {
		if c.err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return c.err
		}
		err := exec.CommandContext(ctx, player.bin, player.args(c.path)...).Run()
		os.Remove(c.path)
		if ctx.Err() != nil {
			for c := range clips { // drain, so nothing synthesized is left on disk
				os.Remove(c.path)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("play: %s: %w", player.bin, err)
		}
	}
	return nil
}

// Stop cuts off whatever is being said.
func (s *Speaker) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Speaking reports whether a Say is in progress.
func (s *Speaker) Speaking() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil
}

func (s *Speaker) finish(gen int, cancel context.CancelFunc) {
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == gen {
		s.cancel = nil
	}
}

type foundPlayer struct {
	bin  string
	args func(string) []string
}

func findPlayer() (foundPlayer, error) {
	for _, p := range players {
		if bin, err := exec.LookPath(p.name); err == nil {
			return foundPlayer{bin, p.args}, nil
		}
	}
	return foundPlayer{}, errors.New("no audio player found: install pw-play (PipeWire), aplay (alsa-utils) or mpv")
}

func writeTemp(wav []byte) (string, error) {
	f, err := os.CreateTemp("", "claw-speech-*.wav")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(wav); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

package identity

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Console is an official provider CLI running under a pseudo-terminal. The
// terminal is owned by the app session; OAuth credentials remain in the CLI's
// normal credential store.
type Console struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func StartConsole(provider string, signIn bool) (*Console, error) {
	bin, args, err := consoleCommand(provider, signIn)
	if err != nil {
		return nil, err
	}
	script, err := exec.LookPath("script")
	if err != nil {
		return nil, errors.New("pseudo-terminal support (script) is unavailable")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command(script, append([]string{"-q", "/dev/null", bin}, args...)...)
	} else {
		cmd = exec.Command(script, "-q", "-c", shellCommand(bin, args), "/dev/null")
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start provider terminal: %w", err)
	}
	return &Console{cmd: cmd, stdin: in, stdout: out, done: make(chan struct{})}, nil
}

func consoleCommand(provider string, signIn bool) (string, []string, error) {
	switch provider {
	case "chatgpt":
		bin := findExecutable("codex", macPath("/Applications/ChatGPT.app/Contents/Resources/codex"))
		if bin == "" {
			return "", nil, errors.New("Codex CLI is not installed")
		}
		if signIn {
			return bin, []string{"login", "--device-auth"}, nil
		}
		return bin, []string{"logout"}, nil
	case "claude_subscription":
		bin := findExecutable("claude", macPath("/opt/homebrew/bin/claude"), macPath("/usr/local/bin/claude"))
		if bin == "" {
			return "", nil, errors.New("Claude Code is not installed")
		}
		if signIn {
			return bin, []string{"auth", "login", "--claudeai"}, nil
		}
		return bin, []string{"auth", "logout"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported provider identity %q", provider)
	}
}

func shellCommand(bin string, args []string) string {
	parts := []string{shellQuote(bin)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return "exec " + strings.Join(parts, " ")
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func (c *Console) Output() io.ReadCloser { return c.stdout }

// Write sends terminal bytes exactly as typed. Enter, control keys and escape
// sequences are encoded by the terminal UI, just as they are by a local TTY.
func (c *Console) Write(data string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("provider terminal is closed")
	}
	_, err := io.WriteString(c.stdin, data)
	return err
}
func (c *Console) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.stdin != nil {
		_, _ = io.WriteString(c.stdin, "\x03")
		_ = c.stdin.Close()
	}
	process := c.cmd.Process
	c.mu.Unlock()
	if process == nil {
		return nil
	}
	time.AfterFunc(2*time.Second, func() {
		select {
		case <-c.done:
		default:
			_ = process.Kill()
		}
	})
	return nil
}
func (c *Console) Wait() error { err := c.cmd.Wait(); close(c.done); return err }

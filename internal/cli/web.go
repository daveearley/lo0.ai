package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"lo0.ai/lo0/internal/web"
)

// Browser opens a URL in the user's browser.
type Browser func(ctx context.Context, url string) error

// SystemBrowser is the Browser for an operating system, named as GOOS
// names it: open on macOS, the URL handler on Windows and xdg-open
// elsewhere.
func SystemBrowser(goos string) Browser {
	return func(ctx context.Context, url string) error {
		var cmd *exec.Cmd
		switch goos {
		case "darwin":
			cmd = exec.CommandContext(ctx, "open", url)
		case "windows":
			cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.CommandContext(ctx, "xdg-open", url)
		}
		return cmd.Run()
	}
}

func (a *App) web(ctx context.Context, args []string) error {
	var (
		c      commonFlags
		port   int
		noOpen bool
	)
	fs := a.flags("web", "[--port N] [--no-open] [--claude-dir DIR] [--offline]")
	fs.IntVar(&port, "port", 0, "serve on port `N` instead of a free one")
	fs.BoolVar(&noOpen, "no-open", false, "print the address without opening a browser")
	fs.StringVar(&c.claudeDir, "claude-dir", "", "read Claude Code files from `DIR` instead of ~/.claude")
	fs.BoolVar(&c.offline, "offline", false, "do not look up the newest Claude Code release")
	if err := parse(fs, args); err != nil {
		return err
	}
	if port < 0 || port > 65535 {
		fmt.Fprintln(a.Err, "--port must be between 0 and 65535")
		return errUsage
	}
	svc, err := a.service(c)
	if err != nil {
		return err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return fmt.Errorf("making a token: %w", err)
	}
	token := hex.EncodeToString(secret[:])
	srv, err := web.New(web.Options{Service: svc, Token: token, Home: a.Env.Home, Version: a.Version, Now: a.now})
	if err != nil {
		return err
	}
	// Only this machine can connect: the listener is on the loopback
	// address, and the token keeps out other users of the same machine.
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", port, err)
	}
	url := "http://" + ln.Addr().String() + "/?token=" + token
	fmt.Fprintf(a.Out, "lo0 is serving this machine's assessment at\n\n  %s\n\nOnly this machine can reach it. Press Ctrl-C to stop.\n", url)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ctx, ln) }()
	if !noOpen && a.Browser != nil {
		if err := a.Browser(ctx, url); err != nil {
			fmt.Fprintln(a.Err, "Could not open a browser; open the address above instead.")
		}
	}
	return <-served
}

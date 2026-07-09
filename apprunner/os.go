package apprunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// OS is the real Runner: apps run as `systemctl --user` services with lingering
// on, so they survive logout and reboot and are restarted on crash by systemd,
// not by käsi (feature-apps.md). käsi only writes the unit and reads state. Each
// app <name> lives in <appsRoot>/<name>/ (the store keeps apps as direct
// children), which becomes the unit's WorkingDirectory.
type OS struct {
	// appsRoot is the store directory; app <name> is at appsRoot/name.
	appsRoot string
	// unitDir is where user units are written (~/.config/systemd/user).
	unitDir string
}

var _ Runner = (*OS)(nil)

// slug is the app-name shape (feature-apps.md): a name becomes a unit file, so
// it must be a bare slug — no path separators, no unit metacharacters.
var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// NewOS builds the systemd adapter over the store's apps root. unitDir defaults
// to ~/.config/systemd/user; a missing HOME is tolerated (unit writes then fail
// loudly at Install time, deferred to reconciliation, rather than at boot).
func NewOS(appsRoot string) *OS {
	home, _ := os.UserHomeDir()
	return &OS{
		appsRoot: appsRoot,
		unitDir:  filepath.Join(home, ".config", "systemd", "user"),
	}
}

// unit is the systemd unit name for an app.
func unit(name string) string { return "kasi-app-" + name + ".service" }

func (o *OS) unitPath(name string) string { return filepath.Join(o.unitDir, unit(name)) }

// Install writes the unit and reloads the manager, ensuring linger so user
// services run without an active login. Idempotent: a rewrite replaces the unit.
func (o *OS) Install(ctx context.Context, name string, port int, startCmd string) error {
	if !slug.MatchString(name) {
		return fmt.Errorf("apprunner: invalid app name %q", name)
	}
	if err := os.MkdirAll(o.unitDir, 0o755); err != nil {
		return fmt.Errorf("apprunner: unit dir: %w", err)
	}
	body := o.unitFile(name, port, startCmd)
	if err := os.WriteFile(o.unitPath(name), []byte(body), 0o644); err != nil {
		return fmt.Errorf("apprunner: write unit: %w", err)
	}
	// Linger is a one-time per-user setting; enabling it again is harmless. A
	// failure here is not fatal — the unit is written and can still be started.
	_ = o.run(ctx, "loginctl", "enable-linger")
	if err := o.systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	return nil
}

// unitFile renders the service unit. The command runs through a login shell so
// the app's runtime (bun, etc.) resolves on PATH the way it does interactively;
// PORT is handed in via the environment, the one thing the app must read.
func (o *OS) unitFile(name string, port int, startCmd string) string {
	workdir := filepath.Join(o.appsRoot, name)
	return fmt.Sprintf(`[Unit]
Description=käsi app: %s
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
Environment=PORT=%d
ExecStart=/bin/bash -lc "%s"
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, name, workdir, port, execEscape(startCmd))
}

// execEscape makes startCmd safe inside a double-quoted systemd ExecStart arg:
// backslash and double-quote are escaped for the quoting, and % is doubled so
// systemd does not read it as a specifier.
func execEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `%`, `%%`)
	return s
}

// Start enables and starts the unit (enable so it comes back after reboot).
func (o *OS) Start(ctx context.Context, name string) error {
	if !slug.MatchString(name) {
		return fmt.Errorf("apprunner: invalid app name %q", name)
	}
	return o.systemctl(ctx, "enable", "--now", unit(name))
}

// Stop takes the unit down and disables it, leaving the unit file in place.
func (o *OS) Stop(ctx context.Context, name string) error {
	if !slug.MatchString(name) {
		return fmt.Errorf("apprunner: invalid app name %q", name)
	}
	return o.systemctl(ctx, "disable", "--now", unit(name))
}

// Restart bounces the unit so a running app picks up new code — `enable --now`
// would leave an already-running unit untouched, so this is the explicit path.
func (o *OS) Restart(ctx context.Context, name string) error {
	if !slug.MatchString(name) {
		return fmt.Errorf("apprunner: invalid app name %q", name)
	}
	return o.systemctl(ctx, "restart", unit(name))
}

// Remove stops the app and deletes its unit; a no-op if already gone.
func (o *OS) Remove(ctx context.Context, name string) error {
	if !slug.MatchString(name) {
		return fmt.Errorf("apprunner: invalid app name %q", name)
	}
	// Best-effort stop; a unit that is already gone is fine.
	_ = o.systemctl(ctx, "disable", "--now", unit(name))
	if err := os.Remove(o.unitPath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("apprunner: remove unit: %w", err)
	}
	return o.systemctl(ctx, "daemon-reload")
}

// Status reports whether the unit is active. `is-active` exits non-zero for an
// inactive or unknown unit, so we read its word rather than the exit code: a
// unit that does not exist is simply down, not an error.
func (o *OS) Status(ctx context.Context, name string) (bool, error) {
	if !slug.MatchString(name) {
		return false, fmt.Errorf("apprunner: invalid app name %q", name)
	}
	out, err := o.output(ctx, "systemctl", "--user", "is-active", unit(name))
	state := strings.TrimSpace(out)
	switch state {
	case "active":
		return true, nil
	case "inactive", "failed", "activating", "deactivating", "unknown", "":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("apprunner: status %s: %w", name, err)
	}
	return false, nil
}

// Logs returns the app's most recent journald lines, oldest first, at most n.
func (o *OS) Logs(ctx context.Context, name string, n int) ([]string, error) {
	if !slug.MatchString(name) {
		return nil, fmt.Errorf("apprunner: invalid app name %q", name)
	}
	if n <= 0 {
		n = 20
	}
	out, err := o.output(ctx, "journalctl", "--user", "-u", unit(name),
		"-n", strconv.Itoa(n), "--no-pager", "-o", "cat")
	if err != nil {
		return nil, fmt.Errorf("apprunner: logs %s: %w", name, err)
	}
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// systemctl runs `systemctl --user <args>` and fails on a non-zero exit.
func (o *OS) systemctl(ctx context.Context, args ...string) error {
	return o.run(ctx, "systemctl", append([]string{"--user"}, args...)...)
}

// run executes a command, folding stderr into the error for a legible failure.
func (o *OS) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			return fmt.Errorf("apprunner: %s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("apprunner: %s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return nil
}

// output runs a command and returns its stdout, along with any run error (the
// caller decides whether a non-zero exit is meaningful, as for `is-active`).
func (o *OS) output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	return outb.String(), err
}

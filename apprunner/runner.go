// Package apprunner is the edge that keeps a registered app's process alive on
// the host (feature-apps.md). The apps module records the registry in the log;
// this edge makes the machine match it — writing a service unit, starting and
// stopping it, and reading its liveness and recent logs. It is deliberately
// outside the runtime: an effect calls it live, replay never does (docs/12).
package apprunner

import "context"

// Runner is the app-process capability the apps module's Edges hold. Every
// method is IDEMPOTENT: the apps-reconcile subscription re-fires the install or
// teardown after a crash (decision-013), so a second Install must replace, a
// second Remove must be a no-op — never an error.
type Runner interface {
	// Install writes (or replaces) the service unit for app `name`: a process
	// launched in the app's store directory with PORT=<port> in its environment,
	// running `startCmd`. It reloads the service manager but does NOT bring the
	// app up — Start does. Re-installing an existing app replaces its unit.
	Install(ctx context.Context, name string, port int, startCmd string) error
	// Start brings the app's unit up and enables it so it survives a reboot.
	Start(ctx context.Context, name string) error
	// Stop takes the app's unit down without deleting it.
	Stop(ctx context.Context, name string) error
	// Remove stops the app and deletes its unit; a no-op if it is already gone.
	Remove(ctx context.Context, name string) error
	// Status reports whether the app's unit is currently up. A unit that does
	// not exist is down, not an error.
	Status(ctx context.Context, name string) (up bool, err error)
	// Logs returns the app's most recent log lines, oldest first, at most n.
	Logs(ctx context.Context, name string, n int) ([]string, error)
}

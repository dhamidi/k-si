package main

// `kasi app add|rm <name>` is the thin control client behind apps
// (feature-apps.md), the exact twin of `kasi notify`. It touches no databases: it
// reads the three KASI_* env vars set on every run and POSTs the request to the
// running server's host-gated control endpoint, which does the real work —
// registering the app on a deterministically-assigned port, or unregistering it.
//
//	kasi app add <name> [--start "<cmd>"]  — register <name>; the start command is
//	    --start if given, else the "start" field of ./store/<name>/app.json. The
//	    whole app.json is sent RAW as the app's operations (empty if absent).
//	kasi app rm <name>                     — unregister <name>.
//
// The app dir is ./store/<name>/ because the persistent store is symlinked into
// the run at ./store and apps are its direct children (Flow F, decision-012).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func runApp(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, appUsage)
		return 2
	}
	switch args[0] {
	case "add":
		return runAppAdd(args[1:])
	case "rm":
		return runAppRm(args[1:])
	default:
		fmt.Fprintln(os.Stderr, appUsage)
		return 2
	}
}

const appUsage = `Usage:
  kasi app add <name> [--start "<cmd>"]
  kasi app rm <name>`

// runAppAdd resolves the start command and raw operations, then POSTs action=add.
func runAppAdd(args []string) int {
	var name, start string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--start":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "kasi app add: --start needs a command")
				return 2
			}
			start = args[i+1]
			i++
		default:
			if name != "" {
				fmt.Fprintln(os.Stderr, appUsage)
				return 2
			}
			name = args[i]
		}
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, appUsage)
		return 2
	}

	// The app.json is read RAW as the app's operations, and — absent --start —
	// is the source of the start command. A missing file is fine (empty
	// operations), but then --start is mandatory.
	appJSONPath := filepath.Join(".", "store", name, "app.json")
	operations := ""
	if b, err := os.ReadFile(appJSONPath); err == nil {
		operations = string(b)
		if start == "" {
			var doc struct {
				Start string `json:"start"`
			}
			if err := json.Unmarshal(b, &doc); err != nil {
				fmt.Fprintf(os.Stderr, "kasi app add: %s: %v\n", appJSONPath, err)
				return 1
			}
			start = doc.Start
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "kasi app add: %s: %v\n", appJSONPath, err)
		return 1
	}
	if start == "" {
		fmt.Fprintf(os.Stderr, "kasi app add: no start command (pass --start or set \"start\" in %s)\n", appJSONPath)
		return 1
	}

	return appPOST(url.Values{
		"action":     {"add"},
		"name":       {name},
		"start":      {start},
		"operations": {operations},
	})
}

// runAppRm POSTs action=rm.
func runAppRm(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, appUsage)
		return 2
	}
	return appPOST(url.Values{
		"action": {"rm"},
		"name":   {args[0]},
	})
}

// appPOST adds the per-run credentials to the form and POSTs it to the control
// endpoint, mirroring `kasi notify`. On success it prints the server's response
// body (for `add`, the app's URL) so the agent can use it.
func appPOST(form url.Values) int {
	controlURL := os.Getenv("KASI_CONTROL_URL")
	taskID := os.Getenv("KASI_TASK_ID")
	token := os.Getenv("KASI_NOTIFY_TOKEN")
	if controlURL == "" || taskID == "" || token == "" {
		fmt.Fprintln(os.Stderr, "kasi app: not inside an agent run (KASI_* env missing)")
		return 1
	}

	endpoint, err := url.JoinPath(controlURL, "control", "app")
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi app:", err)
		return 1
	}
	form.Set("task_id", taskID)
	form.Set("token", token)

	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi app:", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "kasi app: server said %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}
	if out := strings.TrimSpace(string(body)); out != "" {
		fmt.Println(out)
	}
	return 0
}

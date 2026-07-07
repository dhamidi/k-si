package main

// `kasi notify "<message>"` is the thin control client behind notifications
// (feature-notifications.md). It touches no databases: it reads the three
// KASI_* env vars set on every run and POSTs the message to the running server's
// host-gated control endpoint, which does the real work. It returns as soon as the
// server has the message, with an exit code, so the agent knows it went out and
// keeps working.

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func runNotify(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `Usage: kasi notify "<message>"`)
		return 2
	}
	message := strings.Join(args, " ")

	controlURL := os.Getenv("KASI_CONTROL_URL")
	taskID := os.Getenv("KASI_TASK_ID")
	token := os.Getenv("KASI_NOTIFY_TOKEN")
	if controlURL == "" || taskID == "" || token == "" {
		fmt.Fprintln(os.Stderr, "kasi notify: not inside an agent run (KASI_* env missing)")
		return 1
	}

	endpoint, err := url.JoinPath(controlURL, "control", "notify")
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi notify:", err)
		return 1
	}
	resp, err := http.PostForm(endpoint, url.Values{
		"task_id": {taskID},
		"token":   {token},
		"message": {message},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasi notify:", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "kasi notify: server said %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}
	return 0
}

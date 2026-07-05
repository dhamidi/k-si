// Package link owns käsi's capability links (docs/04) in ONE place, so the
// builder, the parser, and the web route can never drift. Built and parsed with
// net/url and dispatch's named routes — never string concatenation.
package link

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dhamidi/dispatch"
)

const (
	// CompletionRoute is the named route the web edge registers for the
	// completion link and this package reverse-routes against.
	CompletionRoute = "tasks.done"
	// CompletionPattern is the URI template of the completion link path.
	CompletionPattern = "/tasks/{id}/done"
	// TokenParam is the query parameter carrying the capability token.
	TokenParam = "token"
)

// completionRouter is a private router, for reverse-routing the completion path
// from its named route.
var completionRouter = mustCompletionRouter()

func mustCompletionRouter() *dispatch.Router {
	r := dispatch.New()
	// a no-op handler: this router is only ever used to build paths, never served
	if err := r.GET(CompletionRoute, CompletionPattern, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); err != nil {
		panic(err)
	}
	return r
}

// Completion builds the full completion link: base ("https://host") + the
// reverse-routed completion path + the token query.
func Completion(base string, taskID int64, token string) (string, error) {
	path, err := completionRouter.Path(CompletionRoute, dispatch.Params{"id": strconv.FormatInt(taskID, 10)})
	if err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path
	q := u.Query()
	q.Set(TokenParam, token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ParseCompletion extracts the task id and token from a completion link.
func ParseCompletion(raw string) (taskID int64, token string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return 0, "", err
	}
	// path is /tasks/<id>/done
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "tasks" || parts[2] != "done" {
		return 0, "", fmt.Errorf("link: %q is not a completion link", raw)
	}
	taskID, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("link: bad task id in %q", raw)
	}
	token = u.Query().Get(TokenParam)
	if token == "" {
		return 0, "", fmt.Errorf("link: %q carries no token", raw)
	}
	return taskID, token, nil
}

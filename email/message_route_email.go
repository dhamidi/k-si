package email

import "github.com/dhamidi/k-si/runtime"

// "route-email" — announced for each stored inbound mail; authorise the sender, resolve the route, hand off to tasks
const RouteEmail = "route-email"

type RouteEmailPayload struct {
	InboxID    int64    `json:"inbox_id"`
	Recipient  string   `json:"recipient"`
	Sender     string   `json:"sender"`
	Cc         []string `json:"cc"`
	Subject    string   `json:"subject"`
	MessageID  string   `json:"message_id"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
}

func NewRouteEmail(p RouteEmailPayload) runtime.Msg {
	return runtime.NewMsg(RouteEmail, p)
}

func registerRouteEmail(mod *runtime.Module) {
	runtime.HandleMsg(mod, RouteEmail, handleRouteEmail)
}

func handleRouteEmail(v runtime.View, s Model, p RouteEmailPayload,
	meta runtime.Meta) (Model, []runtime.Cmd) {

	return s, nil
}

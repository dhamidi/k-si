package runtime

import "encoding/json"

// "send" — the one command built into the runtime rather than a domain
// (docs/01). Its payload is a runtime message; its effect puts that message
// on the inbound channel, stamped with the causing message's offset. It is
// the only sanctioned way for one domain to make another act.
const sendTag = "send"

// Send wraps a message as the built-in send command. A handler that needs
// another domain to act returns Send with a message that domain owns —
// constructed via the receiving domain's contract package (docs/15).
func Send(m Msg) Cmd {
	return NewCmd(sendTag, m)
}

func decodeSend(c Cmd) (Msg, bool) {
	if c.Tag != sendTag {
		return Msg{}, false
	}

	var m Msg
	if err := json.Unmarshal(c.Payload, &m); err != nil {
		return Msg{}, false
	}

	return m, true
}

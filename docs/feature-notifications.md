# Notifications

*when the agent can't wait until it's done*

An agent normally reaches you once, at the end: it finishes the task and emails
you the result. A notification is the exception — a short message it sends while
it's *still working*, delivered right away, that needs nothing back.

The case that forces this is two-factor login. An agent driving your browser hits
a Smart-ID prompt: a code, and a sixty-second countdown to approve it on your
phone. "I'll tell you when I'm done" is useless here. The agent has to get the
code in front of you now, and keep going.

Status: this is the design of record. The implementation is being built to it.

## Notification vs. request

käsi already lets an agent stop and ask you something — a *request*, the web form
it raises for a secret, a file, or a decision. A notification is the opposite.

A **request** needs an answer back. The agent stops, käsi emails you a link, you
fill in a form, and a later run resumes with your answer. It's a round trip, and
the task waits for you.

A **notification** needs nothing back. The agent tells you something and keeps
working. You act on it, or don't, on your own. One-way.

Smart-ID is a notification, not a request, because your approval goes to the
*browser*, not to käsi. You tap approve on your phone, the login page advances,
and the agent — still driving that page — sees the result. käsi never learns you
tapped, and doesn't need to. There's nothing to route back, so there's nothing to
wait for.

## Sending one

Inside a task, the agent runs:

```bash
kasi notify "Smart-ID code 4271 — approve on your phone within 60s."
```

It returns as soon as käsi has the message, with an exit code, so the agent knows
it actually went out. Then it keeps working — same turn, no stopping.

The agent doesn't pass your address, the task id, or where käsi is listening.
Those are already in its environment when the run starts — `KASI_TASK_ID`,
`KASI_CONTROL_URL`, and a per-run `KASI_NOTIFY_TOKEN`. The command is just the
message.

## Waiting at the browser

Sending the code is half the job. The agent also has to stay at the login page
while you approve, then carry on once it advances.

This looks like it breaks käsi's rule that an agent never blocks on input. It
doesn't. The agent isn't waiting on *you* — it's waiting on the *browser*. It
watches the page until Smart-ID advances it, the same way it waits for any page to
load. Your approval reaches the page directly; the agent just sees the result.

The wait is bounded. A Smart-ID prompt lasts about a minute, so that's how long
the agent gives it. If the page advances, the login worked and the task rolls on.
If the minute runs out, the agent stops waiting, ends its turn, and — now that it
genuinely needs a decision from you — raises an ordinary request: *"2FA expired.
Reply to have me try again."*

## How it works

`kasi notify` is a control subcommand, but it doesn't touch the databases itself.
It's a thin client: it reads the three environment variables and POSTs the message
to the running server's control endpoint. The server, holding the live
application, injects a `notify-user` message onto its channel — exactly as the web
interface and the inbound-mail poller inject theirs — and the reducer sends the
mail through the same outbound edge that sends replies (the spool in development,
Fastmail with `-send`). The mail is addressed to the task's initiator and threaded
onto the task, so it shows up in context.

It routes through the server instead of sending mail itself for three reasons:

- The Fastmail credential never enters the agent's environment.
- It reuses the one mail path instead of duplicating it.
- The notification lands in the log, so replay and audit show that you were told,
  when, and by which task.

`KASI_NOTIFY_TOKEN` is minted per run. The endpoint accepts a notification only
when the token matches the live run named by `KASI_TASK_ID`, and it's reachable
only on the host-gated listen address. So an agent can't notify as another task,
or fire one after its run has ended.

## Limitations

- **Email only.** Delivery is email today — it's already configured, already trips
  your phone, and already threads onto the task. A louder channel (a push, a text)
  is a later addition if a minute of mail latency ever proves too slow.
- **The message is logged.** The text is recorded, so a Smart-ID code lands in the
  log. That's harmless — it's a compare-this-number, worthless once the countdown
  ends. If a notification ever needs to carry a real secret, we'd record *that* you
  were notified, not *what* with.
- **One line, no structure.** A notification is a single string — no subject, no
  severity, no attachments. Those are easy to add if a use for them shows up.

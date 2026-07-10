# Delivery mechanisms

*bring your own mail provider*

käsi lives on email — it's how work arrives and how käsi answers. A **delivery
mechanism** is how that mail gets in and out: a provider that carries mail to käsi,
away from it, or both. Fastmail is built in. But you're not stuck with it — you can
add another provider yourself, in käsi's settings, without redeploying anything.

The one this guide walks through is **ForwardEmail**: paste a token and a DNS
record, and mail starts flowing — arriving the instant it's sent, and leaving
signed so it lands in the inbox.

Status: this is the design of record. Fastmail is built; the pluggable mechanisms,
ForwardEmail, and the settings that configure them are being built on top of the
settings module (decision-020, decision-023).

## What a mechanism is

A mechanism is a named provider with two possible jobs:

- **an inbound source** — how mail addressed to käsi reaches it, and
- **an outbound sender** — how käsi's replies leave.

A provider can do one or both. Fastmail does both (built in). ForwardEmail does
both. The development spool is an outbound-only mechanism that writes replies to
files instead of sending them. You turn mechanisms on and configure them on the
`/settings` page — not by editing flags and restarting.

## Adding ForwardEmail

On `/settings`, open the **ForwardEmail** mechanism and fill in two things:

- your **domain** (e.g. `decode.ee`), and
- your ForwardEmail **API token** — stored as a secret, never shown again.

käsi then generates an inbound **webhook token** and shows you the exact DNS `TXT`
record to paste at your registrar:

```
forward-email=https://<vm>.exe.xyz:<port>/inbound/forwardemail/<token>
```

Flip **inbound** and **outbound** on, and you're done. That record tells
ForwardEmail where to deliver your mail; the token in it is what lets käsi trust the
delivery.

## How mail comes in — the webhook

ForwardEmail doesn't wait to be asked. The moment an email arrives, it **POSTs it
to that URL** — a push, not a poll — so käsi sees it in about a second instead of on
the next polling tick.

Three things keep that safe:

- **The token is the key.** Only a POST to the URL with the right token is accepted;
  everything else is refused. And your normal initiator allowlist still applies — a
  stranger emailing in is dropped exactly as before.
- **käsi stores the mail before it says "got it."** The message is written to the
  durable inbox first, and only then does käsi acknowledge. If the network hiccups
  and ForwardEmail retries, nothing is lost and nothing is processed twice.
- **It's the only public door.** Everything else about käsi stays private behind the
  host; this one endpoint faces the internet, and the unguessable token is what
  guards it.

Fastmail, by contrast, is polled every few seconds. Both feed the same task flow —
the webhook is just faster, and you can run both at once.

## How mail goes out

Replies leave through whichever mechanism you've set as the **sender**. Switch it in
settings and the change takes effect immediately — no restart. ForwardEmail signs
each reply with **DKIM** on your domain, so it reads as legitimate mail and lands in
the inbox rather than the spam folder.

## Nothing sends until you say so

A mechanism is **inert until you configure it**. It can't send or receive until its
credential is stored and you've switched it on. So a fresh install sends nothing and
receives no live mail by accident — turning on real mail is a deliberate act. Until
then the safe default is the spool: replies are written to files you can inspect,
and nothing leaves the machine.

## Limitations

- **One sender at a time.** käsi sends through a single active mechanism; there's no
  automatic fallback to a second provider yet.
- **The webhook is a public endpoint.** It's guarded by the token in its URL and by
  the allowlist, but it is, by design, the one part of käsi reachable from the
  internet. If you'd rather expose nothing, stick to a polled mechanism like
  Fastmail.
- **ForwardEmail sending needs their paid tier.** Receiving over the webhook is on
  their free plan; sending from your domain requires a paid plan.
- **ForwardEmail is the first webhook provider.** Fastmail (polled) and the dev
  spool are built in. exe.dev's own local mailbox and a plain SMTP sender fit the
  same shape and are planned, not yet built.

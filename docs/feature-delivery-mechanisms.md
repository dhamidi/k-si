# Delivery mechanisms

*bring your own mail provider*

käsi lives on email — it's how work arrives and how käsi answers. A **delivery
mechanism** is how that mail gets in and out: a provider that carries mail to käsi,
away from it, or both. Fastmail is built in. But you're not stuck with it — you can
add another provider yourself, in käsi's settings, without redeploying anything.

The one this guide walks through is **ForwardEmail**: enter a couple of credentials,
flip it on, and mail flows — käsi picks it up, and sends replies signed so they land
in the inbox.

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

On `/settings`, open the **ForwardEmail** mechanism and fill in:

- your **domain** (e.g. `decode.ee`),
- your ForwardEmail **API token** (for sending) and **IMAP password** (for
  receiving) — both stored as secrets, never shown again.

Flip **inbound** and **outbound** on, and you're done. There's no DNS record to
paste and nothing to expose — just credentials and two switches.

## How mail comes in

käsi **checks the mailbox every few seconds** and pulls anything new — over IMAP for
ForwardEmail, exactly as it does over JMAP for Fastmail. Both feed the same task
flow, and you can run both at once (a message that arrives at either shows up as a
task the same way).

Nothing about this faces the internet. käsi reaches *out* to the provider to fetch
mail; the provider never reaches *in*. So adding a mechanism opens no new door on
your machine.

## How mail goes out

Replies leave through whichever mechanism you've set as the **sender**. Switch it in
settings and the change takes effect immediately — no restart. ForwardEmail signs
each reply with **DKIM** on your domain, so it reads as legitimate mail and lands in
the inbox rather than spam.

Turning a sender on also checks that you have a **deliverable reply address** — so
you can't accidentally switch on a sender that would send mail nobody ever receives.

## Nothing sends until you say so

A mechanism is **inert until you configure it**. It can't send or receive until its
credential is stored and you've switched it on. So a fresh install sends nothing and
receives no live mail by accident — turning on real mail is a deliberate act. Until
then the safe default is the spool: replies are written to files you can inspect,
and nothing leaves the machine.

## Limitations

- **One sender at a time.** käsi sends through a single active mechanism; there's no
  automatic fallback to a second provider yet.
- **Inbound is polled, not instant.** käsi pulls mail every few seconds, so there's
  a short delay. A real-time **webhook** (the provider pushing mail the moment it
  arrives) would be faster — but it would require opening a public entry point on
  your machine, and a safe way to do that on this private deployment isn't solved
  yet, so it's deliberately deferred. Polling needs nothing exposed.
- **ForwardEmail needs their paid tier.** Sending from your domain and having an
  IMAP mailbox to poll both require a paid plan; plain forwarding is free.
- **ForwardEmail is the first added provider.** Fastmail (JMAP) and the dev spool
  are built in. exe.dev's own local mailbox and a plain SMTP sender fit the same
  shape and are planned, not yet built.

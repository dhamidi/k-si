# 02 — Object model (MIME)

käsi uses **MIME** as its internal object model. Anything that crosses a
boundary or gets archived — inbound mail, outbound replies, agent inputs and
outputs, stored artifacts — is a MIME message or MIME part. This document
explains why, and how the business objects relate to it.

## Why MIME

The problem domain is email, and email is MIME. Rather than parse email into a
bespoke schema and translate back, we keep the MIME representation as the native
form and let business objects *reference* it.

MIME buys us, for free:

- **Headers as an open key/value space.** We can attach our own metadata as
  `X-Kasi-*` headers (task id, route, causation) without inventing a side
  channel. Standard headers (`From`, `Subject`, `In-Reply-To`, `References`,
  `Message-ID`) give us threading and identity out of the box.
- **Multipart bodies.** Text plus arbitrary attachments (a PDF invoice, images,
  more nested messages) is exactly what `multipart/mixed` already models. No
  need to invent an attachment container.
- **Content typing.** Every part is self-describing (`Content-Type`,
  `Content-Disposition`, filename, charset, transfer encoding). Laying parts out
  as files and packing files back into parts is mechanical.
- **Nesting.** `message/rfc822` lets a MIME message contain another verbatim —
  useful for forwards (the invoice email you forwarded is preserved intact) and
  for archiving a whole exchange as one object.
- **Durability across builds.** MIME is a frozen, standard wire format. Stored
  bytes remain meaningful regardless of how käsi's code changes.

The standard library carries the weight (`net/mail`, `mime`,
`mime/multipart`, `mime/quotedprintable`), keeping with the dependency-light
goal.

## MIME messages vs. runtime messages

Two different things share the word "message." Keep them distinct (see the
glossary in [00](./00-vision.md)):

- A **MIME message** is an email-shaped *object* — content. It lives in the
  inbox/outbox tables and in archives, and is laid out into workspaces.
- A **runtime message** (`Msg`) is a TEA *event* — a fact fed to the reducer. It
  lives in the message log.

They meet at the edges: an inbound MIME message in the inbox causes a
`route-email` runtime message that *references* it by id (not by value — we
don't inline whole PDFs into the log), while carrying the routing facts inline so
the handler stays pure ([01](./01-architecture.md)).

## The business objects

The business objects are ordinary Go structs held in the model
([01](./01-architecture.md)). MIME is how their *content* is represented and
stored; the structs hold identity, state, and references.

### Task

The central object. One task = one email thread = one agent session (see
[05](./05-agents-and-tasks.md)). A task references:

- the **originating MIME message** (inbox row id),
- the **thread key** (`Message-ID` / `References`) used to thread replies,
- the **route** that selected its task template,
- its **participants** — the initiator plus any addresses CC'd in by an
  authorised sender, who may all interact with this task ([04](./04-email.md)),
- its **workspace** path,
- an ordered list of **agent runs** and their transcripts,
- the **outbound MIME messages** it has produced (outbox row ids),
- its **status** (`open`, `awaiting-user`, `awaiting-agent`, `done`).

The task struct is small; the heavy content (the email, attachments,
transcripts) lives in SQLite and on disk, referenced by id/path.

### File

käsi maintains **files**: attachments extracted from inbound mail, artifacts
produced by agent runs, and anything worth keeping. Files are MIME parts. Each
is stored with its content type, filename, and a content hash; the model holds
lightweight file records (id, hash, size, type, owning task) and the bytes are
archived in SQLite (see [03](./03-persistence.md)). Extracting an inbound
attachment to `in/invoice.pdf` and packing `out/receipt.pdf` back into a reply
are the same operation in two directions.

### Skill

A reusable instruction/prompt bundle provisioned into agent runs (see
[07](./07-skills-and-tools.md)). Represented as content (Markdown + metadata),
it can itself be carried as a MIME part, but is primarily a registry entry in
the model plus content stored durably in SQLite ([03](./03-persistence.md)).
Skills come from two origins: authored in the web UI, or **written by an agent
during a task** and stored so later runs can use them.

### Tool

A CLI program an agent run may use, provisioned via mise (see
[07](./07-skills-and-tools.md)). A tool is a registry entry (name, version,
mise spec); it is not MIME — it is a capability, not content.

### UI request

When an agent needs structured input, file uploads, or a **secret** that
shouldn't be pasted into email, it raises a **UI request** instead of asking in
the reply body ([05](./05-agents-and-tasks.md), [08](./08-web-ui.md)). A UI
request references:

- its **task** and the **agent run** that raised it,
- a **form spec** — the fields to collect, each with a name, label, type
  (`text` / `longtext` / `choice` / `file` / `secret`), and required flag,
- an unguessable **token** for its capability link ([04](./04-email.md)),
- its **status** (`pending`, `answered`, `expired`),
- once answered, references to the collected inputs: file archive ids for
  uploads, and `secret://` URLs for secret fields — **never plaintext**
  ([06](./06-secrets.md)).

The form spec is content the agent authored; the request record is a model entry
plus a durable row ([03](./03-persistence.md)). It is the object behind the
"prep a request → get a link → answer on the web" loop.

### Route / task template

The mapping from an email local part to a task template, and the template
itself (prompt + skills + tools). Configuration held in the model, editable
from the web UI (see [08](./08-web-ui.md)). Not MIME.

## How content flows through MIME

The lifecycle of a task's content, all in MIME terms:

1. **Inbound.** Fastmail hands us raw RFC 5322 bytes. We parse to a MIME message
   and store it in the inbox. Attachments are recognised as parts.
2. **Lay-in.** When the task's workspace is created, the message's text part(s)
   and attachment parts are written into `in/` as files (`in/body.txt`,
   `in/invoice.pdf`, …). The agent sees plain files, not MIME.
3. **Agent run.** The harness reads `in/`, does work, writes results into
   `out/`.
4. **Harvest.** Whatever the agent left in `out/` is read back: each file
   becomes a MIME part. The text becomes the reply body; other files become
   attachments.
5. **Assemble reply.** We build an outbound MIME message: our body, the
   harvested attachments, threading headers (`In-Reply-To`, `References`) tying
   it to the original, and `X-Kasi-*` metadata. It goes into the outbox.
6. **Archive.** The inbound message, the harvested `out/` parts, and the agent
   transcript are archived (see [03](./03-persistence.md)) so the workspace can
   later be deleted without losing anything.

Every step above is a MIME operation, which is why the object model pulls its
weight: the same representation serves the wire, the archive, and the agent's
file tree.

## käsi's header conventions

Custom headers namespaced `X-Kasi-*` carry our metadata on both inbound-derived
and outbound messages:

| Header | Meaning |
|--------|---------|
| `X-Kasi-Task` | Task id this message belongs to |
| `X-Kasi-Route` | The local part / route that selected the task template |
| `X-Kasi-Agent-Run` | The agent run id that produced an outbound message |
| `X-Kasi-Cause` | The inbound message id that caused this one (audit trail) |

Standard headers do the rest: `Message-ID` identifies each message, and
`In-Reply-To` / `References` thread replies so they land in the same email
conversation the user started (see [04](./04-email.md)).

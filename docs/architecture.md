# Architecture

## Scope and verification boundary

WhatsCLI is a single Go terminal process with a real, implemented unofficial WhatsApp adapter, not an offline transport stub. The code paths described below exist; this does **not** mean they have been exercised against WhatsApp. No live account pairing or real text/document sends have occurred. Synthetic transport tests and local SQLite checks do not establish upstream protocol compatibility, complete history sync, reliable delivery, account safety, or actual reconnect/unlink behavior.

`go.mod` declares `go 1.26.0`, selects `toolchain go1.27.2`, and pins `go.mau.fi/whatsmeow v0.0.0-20261030195514-56e5ad0581ce`. Linux is the current target because `internal/privatefs` uses Linux-specific `O_PATH` filesystem operations without a portability implementation. Terminal/SSH usability and low resource use are goals, not broad compatibility or performance guarantees.

## Components

| Component | Implemented responsibility |
| --- | --- |
| `cmd/whats-cli/main.go` | Parse `-offline`, `-state-dir`, `-db`; prepare private state; load settings; open history; construct transport/runtime/model; run Bubble Tea alternate screen; handle shutdown signals |
| `internal/config/config.go` | Versioned persisted consent and mouse setting; bounded JSON reading; temporary-file write, sync, and rename |
| `internal/app/model.go` | Onboarding, pairing QR, connection state, inbox/chat pages, composer, command palette, revision view, file preview/confirmation, settings, help, logout confirmation |
| `internal/app/runtime.go` | Consent gate, transport lifecycle, independent event persistence, coalesced UI notifications, pending/outcome send persistence, shutdown coordination |
| `internal/messaging/client.go` | Transport contract and typed lifecycle/message events |
| `internal/messaging/whatsmeow.go` | Lazy session store, QR lifecycle, reconnect delegation, direct/group text sending, upstream events/history normalization, logout and closure |
| `internal/messaging/normalize.go` | Unwrap supported envelopes; normalize text, edits, revocations, source identities/times, and media placeholders |
| `internal/messaging/attachments*.go` | Host-local regular-file inspection, bounded reads, document upload/send |
| `internal/history/{history,sqlite,migrations,legacy}.go` | Account-aware history contract, transactional ingestion, scoped pagination, schema migration, original local API compatibility |
| `internal/privatefs/privatefs.go` | Private state directory/file preparation and symlink/nonregular/hardlink rejection |
| `internal/ui/{styles,text,qr}.go` | Styles, terminal-text sanitization, size-aware untruncated QR rendering |

## Startup, consent, and session lifecycle

The default state directory is `whats-cli` beneath `os.UserConfigDir()`. It holds `settings.json`, `session.db`, and `history.db`; `-state-dir` relocates all three, while `-db` overrides history only. Paths always belong to the execution host, including when used through SSH.

Startup secures the state directory and opens/migrates history **before** displaying onboarding. Creating the real messaging adapter itself neither touches the session store nor contacts the service. `-offline` substitutes a client that rejects connect/send/logout and never opens a session store.

Consent version 1 and the mouse preference are persisted. On a first run, welcome → disclaimer → acceptance → successful settings save enables connection. The UI hides connection/history commands before consent and guards history loads; the runtime independently rejects connect, send, and logout without acceptance. Help/settings/quit remain available. This is an application network-consent gate, not authentication.

A returning accepted user starts in the inbox and automatically attempts connection. The adapter opens/upgrades the whatsmeow SQLite store lazily, obtains the first stored device, and either reconnects that identity or obtains a QR channel for an unpaired device. QR refresh/expiry/failure events update runtime state; QR success alone is not treated as authenticated readiness. The `Connected` event marks the runtime online and clears the QR. Cached account identity survives upstream deletion of the device record on logout. A revoked session can be initialized afresh; restarting to pair again is the UI's guidance.

The adapter enables whatsmeow's automatic reconnection rather than running its own reconnect loop. Manual `connect`/`r` paths exist. Quitting disconnects/closes without logout; confirmed logout requests upstream unlinking but does not remove retained history or revoke stored consent. None of these live lifecycle paths is service-verified.

In offline mode the model currently remains in the `local` legacy namespace; there is no offline account selector for linked-account history. Account switching in the live model clears loaded chat/message/revision state and the draft.

## Event flow and shutdown

The adapter emits lifecycle events and normalized history events through a **128-entry bounded channel**. Admitted producers block rather than deliberately discard history when the queue is full. This is in-process backpressure, not a durable spool: a crash or storage failure can still lose events.

A runtime ingestion goroutine applies incoming events transactionally before notifying the UI. A single-slot wake channel coalesces redraw requests; the model adds a short 75 ms delay and reloads selected pages. Persistence is not coupled to drawing every event. A storage failure sets a visible sticky error and disables new sends; there is no durable automatic replay queue for failed persistence.

Shutdown stops admission, cancels adapter network operations, and waits for admitted producers while the runtime continues draining events. After the event stream closes and operations finish, runtime context and history close. The model also supports explicit send cancellation and quit; cancellation cannot establish that a send was not delivered.

## Normalization and sends

Normalization accepts direct chats (`s.whatsapp.net` and `lid`) and groups (`g.us`); status/broadcast/newsletter targets are excluded. History sync iterates conversations/messages supplied by upstream and uses the same normalized event path as live input. Push names can title direct inbound chats; group titles do not use a sender's push name. There is no independent full contact directory or complete-chat fetch UI.

Text, extended text, supported nested wrappers, edit protocols, and revoke protocols/stubs map to local events using source identifiers and timestamps. Media are represented by document names, captions, or placeholders; reactions appear as text placeholders. This is not media downloading, rich rendering, disappearing-message enforcement, or reconstruction of unseen content.

The runtime generates an outgoing ID, saves `pending`, then invokes the adapter. On success it reconciles the returned result/self-echo and saves `sent`; transport errors/cancellation are conservatively `unknown`, with a warning to check before retrying. A pending write failure prevents the network send. An incoming persistence failure blocks subsequent sends. The repository supports more status values, but delivered/read/played receipts currently map only to `sent` in the adapter, not a delivered/read UX.

The UI captures the recipient for each operation, rejects stale account/chat/page responses, preserves older-page cursors during refresh, and refuses recipient changes while a nonempty draft or send is pending. Composer `Ctrl+U` clears the draft. Settings/help/palette overlays restore their originating screen. Mouse is optional and maps wheel events to existing up/down handling; it is not click-based navigation.

## History schema and retention

History uses `modernc.org/sqlite`, one database connection, WAL journaling, foreign keys, and a five-second busy timeout. Versioned migration uses `PRAGMA user_version=1`, rejects newer schemas, and transactionally migrates the original conversations/messages/revisions into the reserved `local` account namespace.

- `account_conversations`: account/chat key, title, and last-updated timestamp.
- `account_messages`: account/chat/message key, current body and source timestamps, sender, original-seen flag, edited/deleted state, direction, and delivery status.
- `account_revisions`: retained original/edit bodies, observation-order revision numbers, source timestamps, and event identities.
- `history_events`: scoped event identities for transactional deduplication.

Duplicate events/self-echoes do not duplicate original revisions. Current content resolves by source timestamp with deterministic ties, not arrival order. A permanent revoke marker does not erase prior plaintext; text received later is still retained as a revision, while content after the revoke's source timestamp is not promoted to current body. An edit/revoke may arrive before an original and create a placeholder. Revision pages are ordered by decreasing **observation number**, not necessarily chronological edit time.

Conversation/message pages use timestamp-plus-ID cursors in descending order; revision pages use number cursors. The UI requests 50 conversations/messages or 20 revisions at a time; repository limits are positive and capped at 200. Older pages replace the view rather than building an unbounded transcript. Original unlinked repository methods remain bounded compatibility wrappers.

## Attachments and deferred features

Attachment selection is a literal path on this host, not an OS picker or SSH file-transfer mechanism. Preview displays destination, basename, and byte size; explicit confirmation calls document sending. Reads are capped at 10 MiB, reject final symlinks/nonregular files, verify the opened inode against the preceding `Lstat`, and detect size changes during reading. Upload uses whatsmeow's document path and a detected MIME type. Plaintext and encrypted copies can both occupy memory. Preview and sending reopen separately; see [security](security.md) for limits rather than assuming immutable preview contents.

Deferred: calls, status publishing, broadcasts/newsletters, rich sticker/media handling, downloads/opening, new-chat creation, contact/group administration, outgoing edit/revoke actions, full-text search, full delivery/read semantics, offline linked-account selection, retention purge/expiry, encrypted local storage, and complete-history recovery. See [testing](testing.md) for the boundary between current local checks and future authorized live acceptance.

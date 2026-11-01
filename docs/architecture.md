# Architecture

## Scope and verification boundary

WhatsCLI is a single Go terminal process with a real, implemented unofficial WhatsApp adapter, not an offline transport stub. The code paths described below exist; this does **not** mean they have been exercised against WhatsApp. No live account pairing or real text/document sends have occurred. Synthetic transport tests and local SQLite checks do not establish upstream protocol compatibility, complete history sync, reliable delivery, account safety, or actual reconnect/unlink behavior.

`go.mod` declares `go 1.26.0`, selects `toolchain go1.27.2`, and pins `go.mau.fi/whatsmeow v0.0.0-20261030195514-56e5ad0581ce`. Linux is the current target because `internal/privatefs` uses Linux-specific `O_PATH` filesystem operations without a portability implementation. Terminal/SSH usability and low resource use are goals, not broad compatibility or performance guarantees.

## Components

| Component | Implemented responsibility |
| --- | --- |
| `cmd/whats-cli/main.go` | Parse `-offline`, `-demo`, `-version`, `-state-dir`, `-db`; prepare private state; load settings; open history; construct transport/runtime/model; run Bubble Tea alternate screen; handle shutdown signals |
| `internal/config/config.go` | Versioned persisted consent and mouse setting; bounded JSON reading; temporary-file write, sync, and rename |
| `internal/app/model.go` | Onboarding, pairing QR, connection state, inbox/chat pages, composer, command palette, revision view, file preview/confirmation, settings, help, logout confirmation |
| `internal/app/workspace.go` | Neon three-pane workspace, dated bubbles, search/navigation, shared cell geometry, optional mouse targets and pairing layout |
| `internal/app/newchat.go` | Consent/draft-guarded local chat creation and upstream Meta AI identity selection |
| `internal/app/demo.go` | Explicit synthetic data and demo model for a disposable database |
| `internal/app/runtime.go` | Consent gate, transport lifecycle, independent event persistence, coalesced UI notifications, pending/outcome send persistence, shutdown coordination |
| `internal/updater/updater.go` | Compare embedded SemVer with the latest stable GitHub Release; invoke the verified install script on explicit request |
| `internal/messaging/client.go` | Transport contract and typed lifecycle/message events |
| `internal/messaging/whatsmeow.go` | Lazy session store, QR lifecycle, reconnect delegation, direct/group text sending, upstream events/history normalization, logout and closure |
| `internal/messaging/normalize.go` | Unwrap supported envelopes; normalize text, edits, revocations, source identities/times, and media placeholders |
| `internal/messaging/attachments*.go` | Host-local regular-file inspection, bounded reads, document upload/send |
| `internal/history/{history,sqlite,migrations,legacy}.go` | Account-aware history contract, transactional ingestion, scoped pagination, schema migration, original local API compatibility |
| `internal/privatefs/privatefs.go` | Private state directory/file preparation and symlink/nonregular/hardlink rejection |
| `internal/ui/{canvas,theme,avatar,doodle,qr,text}.go` | Cell canvas with explicit fg/bg per cell (truecolor/256), palette, circular avatars (photo or initials), wallpaper, half-block QR, terminal-text sanitization |
| `internal/app/{view,workspace,overlays,explorer,mouse,keys,slash}.go` | Screen renderers that also register hit regions, file browser and upload queue, mouse routing, keyboard and slash commands |
| `internal/webpair` | Loopback-only web page for QR pairing when the terminal cannot show it |
| `internal/messaging/{avatar,bundle}.go` | Allow-listed profile-picture download; folder inspection and zip archiving |

## Startup, consent, and session lifecycle

The default state directory is `whats-cli` beneath `os.UserConfigDir()`. It holds `settings.json`, `session.db`, and `history.db`; `-state-dir` relocates all three, while `-db` overrides history only. Paths always belong to the execution host, including when used through SSH.

Startup secures the state directory and opens/migrates history **before** displaying onboarding. Creating the real messaging adapter itself neither touches the session store nor contacts the service. `-offline` substitutes a client that rejects connect/send/logout and never opens a session store.

Consent version 1, mouse preference, and reduced-motion preference are persisted. On a first run, welcome → disclaimer → acceptance → successful settings save enables connection. The UI hides connection/history commands before consent and guards history loads; the runtime independently rejects connect, send, and logout without acceptance. Help/settings/quit remain available. This is an application network-consent gate, not authentication.

A returning accepted user starts in the inbox and automatically attempts connection. The adapter opens/upgrades the whatsmeow SQLite store lazily, obtains the first stored device, and either reconnects that identity or obtains a QR channel for an unpaired device. QR refresh/expiry/failure events update runtime state; QR success alone is not treated as authenticated readiness. The `Connected` event marks the runtime online and clears the QR. Cached account identity survives upstream deletion of the device record on logout. A revoked session can be initialized afresh; restarting to pair again is the UI's guidance.

The adapter enables whatsmeow's automatic reconnection rather than running its own reconnect loop. Manual `connect`/`r` paths exist. Quitting disconnects/closes without logout; confirmed logout requests upstream unlinking but does not remove retained history or revoke stored consent. None of these live lifecycle paths is service-verified.

In offline mode the model currently remains in the `local` legacy namespace; there is no offline account selector for linked-account history. Account switching in the live model clears loaded chat/message/revision state and the draft.

## Event flow and shutdown

The adapter emits lifecycle events and normalized history events through a **128-entry bounded channel**. Admitted producers block rather than deliberately discard history when the queue is full. This is in-process backpressure, not a durable spool: a crash or storage failure can still lose events.

A runtime ingestion goroutine applies incoming events transactionally before notifying the UI. A single-slot wake channel coalesces redraw requests; the model adds a short 75 ms delay and reloads selected pages. Persistence is not coupled to drawing every event. A storage failure sets a visible sticky error and disables new sends; there is no durable automatic replay queue for failed persistence.

Shutdown stops admission, cancels adapter network operations, and waits for admitted producers while the runtime continues draining events. After the event stream closes and operations finish, runtime context and history close. The model also supports explicit send cancellation and quit; cancellation cannot establish that a send was not delivered.

## Normalization and sends

Normalization accepts direct chats (`s.whatsapp.net` and `lid`), groups (`g.us`), and bots (`bot`); status/broadcast/newsletter targets are excluded. Meta AI's older direct and newer bot identities are supported and titled explicitly. Bot-invoke messages use the existing unwrap/history path; upstream handles bot metadata and encryption. History sync iterates conversations/messages supplied by upstream and uses the same normalized event path as live input. Push names can title direct inbound chats; group titles do not use a sender's push name. There is no independent full contact directory or complete-chat fetch UI.

Text, extended text, supported nested wrappers, edit protocols, revoke protocols/stubs, replies, and reactions map to local events using source identifiers and timestamps. Reactions are projected onto their target messages. Status broadcasts are stored separately and listed only while recent; older server updates are not fetched. Media are represented by document names, captions, or placeholders; story media are not downloaded. This is not rich media rendering, disappearing-message enforcement, or reconstruction of unseen content.

The runtime generates an outgoing ID using the supported client method, saves `pending`, then invokes the adapter. On success it reconciles the returned result/self-echo and saves `sent`; transport errors/cancellation are conservatively `unknown`, with a warning to check before retrying. A pending write failure prevents the network send. An incoming persistence failure blocks subsequent sends. Delivered receipts map to `delivered`; read/played receipts map to `read`. Existing monotonic storage status ordering prevents older receipts from downgrading a message. Receipt mappings are synthetically tested, not live-certified.

Replies include WhatsApp quote context and local sender/snippet metadata. `Ctrl+R` targets the selected message; `Ctrl+E` opens the emoji picker for a reaction. Text statuses can be posted after explicit review, and incoming status updates seen while connected are cached for 24 hours. Terminal connection state is not the account's personal online presence. These sends are covered with synthetic/local tests only; no live account was used to verify service behavior.

The UI captures the recipient for each operation, rejects stale account/chat/page responses, preserves older-page cursors during refresh, and refuses recipient changes while a nonempty draft or send is pending, including asynchronous chat creation. Composer `Ctrl+U` clears the draft. Settings/help/palette/new-chat overlays restore their originating screen. At 90×20 or larger the layout has a navigation rail, filtered chat list, and conversation pane; long lists/transcripts show scroll indicators. Optional mouse click/hover/wheel targets share renderer geometry; narrow screens retain wheel navigation. Keyboard alternatives remain available. Profile photos render as 8×6 half-block pixels; initials remain the fallback.

`-demo` overrides state with a temporary directory and selects the offline adapter. Synthetic messages are seeded only there. Local demo sends bypass the real transport and remain explicitly labelled; no fabricated AI responses are generated. The temporary directory is removed at exit. `-version` exits before opening any state.

## History schema and retention

History uses `modernc.org/sqlite`, one database connection, WAL journaling, foreign keys, and a five-second busy timeout. Versioned migration uses `PRAGMA user_version=3`, rejects newer schemas, and transactionally migrates the original conversations/messages/revisions into the reserved `local` account namespace.

- `account_conversations`: account/chat key, title, and last-updated timestamp.
- `account_messages`: account/chat/message key, current body and source timestamps, sender, original-seen flag, edited/deleted state, direction, and delivery status.
- `account_revisions`: retained original/edit bodies, observation-order revision numbers, source timestamps, and event identities.
- `account_statuses`: account-scoped status text and source time; queries and new updates prune entries older than 24 hours.

Release versions come from the root `VERSION` file. Stable tags use `vX.Y.Z`; the release workflow validates the tag against `VERSION`, runs Go checks, and publishes GitHub release notes. The installer and in-app checker follow the latest stable release by default; `WHATSCLI_REF=main` opts into development builds.
- `account_reactions`: latest reaction per account/chat/target/sender; removals delete the projection.
- `history_events`: scoped event identities for transactional deduplication.

Duplicate events/self-echoes do not duplicate original revisions. Current content resolves by source timestamp with deterministic ties, not arrival order. A permanent revoke marker does not erase prior plaintext; text received later is still retained as a revision, while content after the revoke's source timestamp is not promoted to current body. An edit/revoke may arrive before an original and create a placeholder. Revision pages are ordered by decreasing **observation number**, not necessarily chronological edit time.

Conversation/message pages use timestamp-plus-ID cursors in descending order; revision pages use number cursors. The UI requests 50 conversations/messages or 20 revisions at a time; repository limits are positive and capped at 200. Older pages replace the view rather than building an unbounded transcript. Original unlinked repository methods remain bounded compatibility wrappers.

## Attachments and deferred features

Attachment selection is a literal path on this host, not an OS picker or SSH file-transfer mechanism. Preview displays destination, basename, and byte size; explicit confirmation calls document sending. Reads are capped at 64 MiB, reject final symlinks/nonregular files, verify the opened inode against the preceding `Lstat`, and detect size changes during reading. Upload uses whatsmeow's document path and a detected MIME type. Plaintext and encrypted copies can both occupy memory. Preview and sending reopen separately; see [security](security.md) for limits rather than assuming immutable preview contents.

Deferred: calls, status media downloads, newsletter/broadcast support, rich sticker/media handling, downloads/opening, contact/group administration, outgoing edit/revoke actions, full-history text search, group-wide receipt semantics, offline linked-account selection, encrypted local storage, and complete-history recovery. New direct-chat creation is local and does not verify account existence; chat search is limited to the loaded page. See [testing](testing.md) for the boundary between current local checks and future authorized live acceptance.

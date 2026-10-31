# WhatsCLI

A keyboard-first Go terminal client with an implemented unofficial WhatsApp transport, local retained history, and host-local document sending. Intended for interactive terminals, including SSH.

## Status: implemented, not live-verified

This is early software, **not an offline-only scaffold**, and **not a live-compatibility certification**. The current code implements consent-gated connection, terminal pairing QR codes, persisted device sessions, incoming/history event handling, conversation and message views, text/document sends, retained revisions and revocation markers, and confirmed logout.

**No live account pairing or real message/file sends have been performed.** Automated tests use synthetic events, temporary databases, and fake clients. Actual WhatsApp pairing, session restoration, reconnects, history availability, sends, and unlinking remain unverified against the service. WhatsCLI is independent and not endorsed by WhatsApp or Meta; unofficial clients may lead to account restrictions. Do not rely on it for critical communication.

## Requirements and run

The module declares **`go 1.26.0`** and **`toolchain go1.27.2`**. Use the selected Go 1.27.2 toolchain; an older Go 1.22 installation is not sufficient. `go.mod`/`go.sum` pin dependencies, including `go.mau.fi/whatsmeow v0.0.0-20261030195514-56e5ad0581ce`.

Linux is the current implementation target: private state-file handling uses Linux-specific filesystem APIs. Other operating systems and broad SSH/terminal compatibility are not established. No WhatsApp API key is required. Initial dependency/toolchain downloads may require network access even when the app runs with `-offline`.

```sh
# Safe local UI exploration: no WhatsApp network operations.
go run ./cmd/whats-cli -offline

# Prefer a disposable private directory for a smoke check.
scratch=$(mktemp -d)
go run ./cmd/whats-cli -offline -state-dir "$scratch"
```

The live-mode invocation is `go run ./cmd/whats-cli`. **Without `-offline`, accepting consent can initiate a connection; saved consent causes automatic connection on subsequent launches.** Run live mode only when you intend to authorize that behavior.

| Flag | Meaning |
| --- | --- |
| `-offline` | Disable all WhatsApp network operations; do not open a device-session database |
| `-state-dir PATH` | Directory for `settings.json`, `session.db`, and, by default, `history.db` on the execution host |
| `-db PATH` | Override only the history database location; settings/session stay under `-state-dir` |

Default state directory: `whats-cli` beneath Go's `os.UserConfigDir()`; on Linux, normally `$XDG_CONFIG_HOME/whats-cli` or `$HOME/.config/whats-cli`. State directories and history are prepared before onboarding; the real transport is lazy and does not create device credentials or contact WhatsApp until connection is allowed.

`-offline` currently displays the `local` legacy/unlinked history namespace, not a selectable cache of previously linked accounts. It is useful for onboarding/UI checks but is not a complete offline archive browser.

## Consent, linking, and sessions

On first use: `Enter` opens the disclaimer, `Space` checks acceptance, then `Enter` saves the versioned consent before enabling connection. Failed settings persistence leaves connection disabled. Before acceptance, the palette exposes only help, settings, and quit; connection and history-loading paths are gated too. This is a network-consent gate, not authentication or protection from someone using your OS account.

With consent already saved, startup opens the inbox and attempts to restore/connect the saved device session. When pairing is needed, the UI renders a QR for WhatsApp's **Linked devices → Link a device** flow. The QR is a credential: never share it, screenshot it, or include it in logs. `f` toggles fullscreen; small terminals show size guidance rather than a cropped QR. `r` retries, `i` shows available history, and `q` cancels by quitting. `Esc` only leaves fullscreen; it does not cancel a pending connection.

Quitting preserves the login. The `logout` palette command asks for confirmation before requesting unlinking; local history is retained. Pairing after logout may require restarting. Service-side behavior remains unverified.

## Keyboard controls

| Context | Key | Action |
| --- | --- | --- |
| Globally | `Ctrl+C` | Cancel any send and quit |
| Outside composer/path input | `q` | Quit |
| Outside composer/path input | `/` or `Ctrl+P` | Open command palette |
| Palette | Type, `Backspace`, `Up`/`Down`, `Enter` | Filter, select, run; `Esc` returns to the originating screen |
| Inbox | `Up`/`Down`, `Enter` | Select/open an existing conversation |
| Inbox/chat | `p`, `r` | Load an older page / return to newest |
| Inbox/chat | `s` | Settings |
| Chat | `Up`/`Down`, `Enter` | Select message / inspect retained text versions |
| Chat | `i`, `a`, `Esc` | Compose / choose a host-local file / return to inbox |
| Composer | `Enter`, `Ctrl+J`, `Backspace` | Send text / add newline / remove last rune |
| Composer | `Ctrl+U`, `Esc` | Clear draft / leave composer while preserving draft |
| During send | `Ctrl+X` | Request send cancellation; delivery may remain uncertain |
| Revisions | `p`, `r`, `Esc` | Older observed versions / latest page / return |
| Settings | `m` | Toggle and persist optional mouse-wheel navigation |
| Scrollable screens | Arrows, `PgUp`/`PgDown` | Scroll content |

Palette commands: `conversations`, `connect`, `attach`, `help`, `settings`, `logout`, `quit`. These are palette entries, not shell-style commands typed into a message. `q`, `/`, and `s` are ordinary text in the composer. A nonempty draft or pending send prevents switching to another recipient; clear the draft with **`Ctrl+U` while in the composer** before changing chats. Composer input is capped at 4,096 runes.

Mouse is off by default. The persisted setting enables wheel events mapped to keyboard navigation, not a clickable graphical UI. All actions remain keyboard-accessible.

## Messages, files, and local retention

- Existing direct and group chats are populated from received events and whatever history WhatsApp supplies. Titles can fall back to chat identifiers. There is no new-chat/contact-picker UI.
- Incoming text, edits, revocations, and supported history events are stored account/chat/message-scoped. History can be incomplete: content never received cannot be recovered. Revision numbers follow local observation order, not necessarily edit chronology.
- Text sends require a connected account. The runtime saves a pending entry before sending, reconciles the returned result/self-echo, and records uncertain outcomes as `unknown`. **Check WhatsApp before retrying an uncertain send; cancellation is not proof of non-delivery.** Receipt handling currently does not distinguish delivered/read states in the live adapter.
- `a` or palette `attach` accepts a literal host-local file path (spaces supported; do not add shell quotes). Preview shows destination, filename, and size; a second `Enter` confirms sending **as a document**, capped at **10 MiB (10,485,760 bytes)**. Regular-file checks reject final symlinks/devices and bound reads; this is not a general file picker or remote transfer facility.
- Over SSH, attachment paths refer to the remote execution host. Copy a laptop file with SCP/SFTP first. There is no automatic media downloading or opening. Received media generally appear as placeholders/captions, not accessible files.
- History and device credentials are **not encrypted at rest**. Edits preserve earlier received text; revocations retain previously received plaintext and a deletion marker. Unlinking does not purge history. There is no expiry, purge UI, or secure-erasure feature. See [security and retention](docs/security.md), including filesystem and attachment-preview limits.

## Deliberately out of scope

No calls, status publishing, broadcast/newsletter support, rich sticker/media rendering or sticker sending, media downloads/opening, new-chat creation, contact/group management, outgoing edit/revoke controls, full-text search, or complete-history recovery. There is no full delivered/read UX or automatic retry guarantee. Performance, account safety, and cross-platform terminal compatibility are not certified.

## Development and verification

```sh
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/whats-cli
```

The current `go test ./...` run is confirmed passing. Race, vet, build, and offline PTY checks are not recorded as passed here until separately confirmed; none establishes live service compatibility. No account credentials are needed for the automated suite. See [architecture](docs/architecture.md) and [testing](docs/testing.md) for implemented boundaries, coverage, and remaining acceptance checks. Keep databases, session material, QR codes, private logs, and media out of Git.

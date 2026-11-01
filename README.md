# WhatsCLI

A keyboard-first Go terminal client with an implemented unofficial WhatsApp transport, local retained history, and host-local document sending. Intended for interactive terminals, including SSH.

## Neon workspace and quick install

The native TUI now has a neon-green navigation rail, searchable chat list, dated
incoming/outgoing message bubbles, a right-aligned send control, and a persistent
composer. It follows the supplied reference within terminal-cell constraints:
there is no bitmap wallpaper, photographic avatar, or pixel-perfect GUI claim.
Use a true-color terminal at **130×42 or larger** for the roomy three-pane layout;
90×20 enables the compact three-pane workspace, and smaller terminals use a
single-pane layout. No patched/Nerd Font is required.

For a fresh **Linux / WSL2** installation (requires Git and Go with automatic
toolchain downloading enabled):

```sh
curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh | sh
```

**That URL uses `main`; it will include this installer only after these changes
are pushed/merged there.** Nothing in this change publishes to GitHub. For this
checkout, use `sh install.sh --local` now.

The installer downloads source into **`~/whatscli`**, verifies Go modules, builds
`~/whatscli/bin/whatscli`, and creates `~/.local/bin/whatscli` if no command already
exists. It needs no sudo, never edits your shell profile, never launches the live
client, and refuses to overwrite an existing checkout. If needed, add
`export PATH="$HOME/.local/bin:$PATH"` to your shell profile.

Inspect remote scripts before executing them if you prefer:

```sh
curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh -o /tmp/whatscli-install.sh
less /tmp/whatscli-install.sh
sh /tmp/whatscli-install.sh
```

`WHATSCLI_DIR`, `WHATSCLI_BIN_DIR`, and `WHATSCLI_REF` optionally select absolute
install/command paths and a branch or tag. Set them on the **`sh` process**, not
just on `curl`. Example:

```sh
curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh |
  WHATSCLI_DIR="$HOME/whatscli" WHATSCLI_REF=main sh
```

Explore safely before linking:

```sh
whatscli -demo      # Synthetic chats; disposable state; no WhatsApp connection
whatscli -version
whatscli           # Real client; first-run consent and linked-device QR
```

`-demo` never opens your real state directory, ignores `-state-dir`, and rejects
`-db`. Demo sends only add labelled synthetic local messages; they are not real
delivery acknowledgements or AI replies. Demo mouse support starts enabled.

To update a managed source checkout without replacing your files automatically:

```sh
cd ~/whatscli
git pull --ff-only             # Resolve any local work yourself if Git refuses
sh install.sh --local         # Atomic binary replacement after a successful build
```

To uninstall, remove the installed command symlink and checkout you created.
Settings/history under your user config directory are separate and are **not**
deleted automatically. Do not delete those files unless you intend to erase
locally retained history and device credentials.

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
| `-demo` | Safe, disposable synthetic conversations; no real transport or account state |
| `-version` | Print the application version and exit without opening state |

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
| Inbox/chat | `Tab` | Switch between chat list and the open conversation |
| Inbox/chat | `Ctrl+F` | Search the current loaded page by chat title/number; `Esc` clears |
| Inbox/chat | `n` | Open a direct conversation using an international phone number |
| Inbox/chat | `?` | Open keyboard help |
| Inbox/chat | `p`, `r` | Load an older page / return to newest |
| Inbox/chat | `s` | Settings |
| Chat | `Up`/`Down`, `Enter` | Select message / inspect retained text versions |
| Chat | `i`, `a`, `Esc` | Compose / choose a host-local file / return to inbox |
| Composer | `Enter`, `Ctrl+J`, `Backspace` | Send text / add newline / remove last rune |
| Composer | `Ctrl+U`, `Esc` | Clear draft / leave composer while preserving draft |
| During send | `Ctrl+X` | Request send cancellation; delivery may remain uncertain |
| Revisions | `p`, `r`, `Esc` | Older observed versions / latest page / return |
| Settings | `m`, `t` | Toggle optional mouse support / reduced motion |
| Scrollable screens | Arrows, `PgUp`/`PgDown` | Scroll content |

Palette commands: `conversations`, `new chat`, `meta ai`, `connect`, `attach`, `help`, `settings`, `logout`, `quit`. These are palette entries, not shell-style commands typed into a message. `q`, `/`, `n`, and `s` are ordinary text in the composer. A nonempty draft or pending send prevents switching to another recipient; clear the draft with **`Ctrl+U` while in the composer** before changing chats. Composer input is capped at 4,096 runes.

Mouse is off by default outside demo mode. On the wide workspace, enabling it
adds hover highlights, chat selection, navigation-rail buttons, search focus,
composer focus, attachment selection, send clicks, and wheel navigation.
Small single-pane screens retain wheel navigation. All actions remain
keyboard-accessible; `Tab`/`i`/`a`/`n`/the palette are the mouse alternatives.
Settings `t` disables the sending/pairing spinner; no flashing animation is used.

## Meta AI

Open the palette with `/`, filter to **`meta ai`**, and press Enter. This opens
an existing Meta AI chat when available or creates a local chat using
whatsmeow's `types.NewMetaAIJID`. It sends nothing until you compose and send.
Both the older direct-user and newer `@bot` identities are accepted on receive;
bot-invoke responses use the same retained-history pipeline as normal messages.
The pinned whatsmeow client supplies its native bot metadata and encryption.

**There is no separate LLM API key, simulated AI backend, or guarantee that Meta
will enable this for your account.** Real replies require a paired, connected
account with Meta AI available in its region. Live Meta AI exchanges have not
been verified here. Check uncertain sends in WhatsApp before retrying.

## Messages, files, and local retention

- Direct/group/bot chats are populated from received events and whatever history WhatsApp supplies. Titles can fall back to chat identifiers. `n` creates a local direct-chat entry from an international number; it does not verify that the recipient has WhatsApp. There is no full contact-directory picker.
- Incoming text, edits, revocations, and supported history events are stored account/chat/message-scoped. History can be incomplete: content never received cannot be recovered. Revision numbers follow local observation order, not necessarily edit chronology.
- Text sends require a connected account. The runtime saves a pending entry before sending, reconciles the returned result/self-echo, and records uncertain outcomes as `unknown`. **Check WhatsApp before retrying an uncertain send; cancellation is not proof of non-delivery.** Service receipt events now distinguish `sent`, `delivered`, and `read` in retained history and bubble labels. Real receipt behavior remains live-unverified; group receipts do not certify that every member read a message.
- `a` or palette `attach` accepts a literal host-local file path (spaces supported; do not add shell quotes). Preview shows destination, filename, and size; a second `Enter` confirms sending **as a document**, capped at **10 MiB (10,485,760 bytes)**. Regular-file checks reject final symlinks/devices and bound reads; this is not a general file picker or remote transfer facility.
- Over SSH, attachment paths refer to the remote execution host. Copy a laptop file with SCP/SFTP first. There is no automatic media downloading or opening. Received media generally appear as placeholders/captions, not accessible files.
- History and device credentials are **not encrypted at rest**. Edits preserve earlier received text; revocations retain previously received plaintext and a deletion marker. Unlinking does not purge history. There is no expiry, purge UI, or secure-erasure feature. See [security and retention](docs/security.md), including filesystem and attachment-preview limits.

## Deliberately out of scope

No calls, status publishing, broadcast/newsletter support, rich sticker/media rendering or sticker sending, media downloads/opening, contact/group management, outgoing edit/revoke controls, full-history message search, or complete-history recovery. Chat search filters only the currently loaded page, not the entire archive. There is no automatic retry guarantee. Performance, account safety, and cross-platform terminal compatibility are not certified.

## Development and verification

```sh
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/whats-cli
python3 -m unittest discover -s tests -p '*_test.py'
sh -n install.sh
python3 tests/terminal_smoke.py ./whats-cli
```

The automated suite uses no real accounts. Installer tests stub Git/Go and never
download or replace a real installation. GitHub Actions runs the commands above
on Linux. See [architecture](docs/architecture.md) and [testing](docs/testing.md)
for implemented boundaries and live acceptance checks. Keep databases, session
material, QR codes, private logs, and media out of Git.

Figma's supervised HTTP preview is an **import-status page**, not the terminal
application. See [.figma/make/SETUP_NOTES.md](.figma/make/SETUP_NOTES.md) for why
static web deployment is unsupported for this native executable.

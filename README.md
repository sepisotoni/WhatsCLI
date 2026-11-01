# WhatsCLI

A keyboard-first Go terminal client for WhatsApp with a neon, fully filled true-colour UI, optional mouse support, local retained history, and file/folder uploads. Built for interactive terminals, including SSH.

> **Unofficial and not live-verified.** WhatsCLI is independent and not endorsed by WhatsApp or Meta; unofficial clients may lead to account restrictions. **No live pairing, send, profile-picture fetch or Meta AI exchange has been performed by the automated tests.** Use a spare account and do not rely on it for critical communication.

## Install (Linux / WSL2)

```sh
curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh | sh
```

Requires Git, curl, and Go (the pinned 1.27.2 toolchain downloads automatically). On first run it installs the latest stable GitHub Release into **`~/whatscli`**, builds `~/whatscli/bin/whatscli`, links `~/.local/bin/whatscli`, never needs sudo, and never edits your shell profile. Re-running the same `curl | sh` command updates an existing clean WhatsCLI checkout from the expected GitHub origin; other existing directories are not overwritten. Pin a release with `WHATSCLI_REF=v1.2.3`, or install the development branch with `WHATSCLI_REF=main`. In a checkout use `sh install.sh --local`.

Beta builds are opt-in and do not change the default stable channel:

```sh
curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh | sh -s -- --beta
```

For an existing clean install, rerun that command to update to the latest published beta. This channel currently requires `python3` to select the newest prerelease from GitHub.

Try the real UI safely first:

```sh
whatscli -demo       # synthetic chats, disposable state, never touches WhatsApp
whatscli             # live: consent screen, then the linked-device QR
```

Run `whatscli version` to see the installed version and `whatscli update` to update it. Beta builds continue following the beta channel; stable builds follow stable releases. The installer skips checkout and rebuilding when that exact version is already installed. You can also use `/update` (or **Check for updates** in Settings), rerun the stable install command above, or run `sh ~/whatscli/install.sh --beta --update` to explicitly switch to the latest beta. Updates require a clean checkout with the expected GitHub origin and replace the binary only after a successful build. Updating does not skip or alter the usual welcome, agreement, or device-pairing flow. The root `VERSION` file is the next stable version; pushing `vX.Y.Z-beta.N` publishes a GitHub prerelease, while pushing the matching `vX.Y.Z` tag publishes a stable release.

## The interface

The UI is drawn on a cell canvas where **every cell has an explicit background**, so panels, bubbles and the doodle wallpaper stay filled instead of falling back to the terminal's default colour. It follows the reference mock-ups: navigation rail with unread badge, chat list with round avatars, header, dated bubbles with ticks, composer with emoji/attach/send, the welcome, disclaimer and *Link a device* screens.

- **Colour over SSH:** `-colors auto` (default) assumes 24-bit colour even when `COLORTERM` is not forwarded. Use `-colors 256` for limited terminals.
- **Size:** roomy at 130×42 and up; the three-pane layout starts at 100 columns, two panes at 72, and one pane below that.
- **Avatars:** profile pictures are fetched from WhatsApp's CDN (HTTPS and an allow-listed host only), reduced to 6×6 half-block pixels and cached for the session. Contacts without artwork get coloured circles with their initials. Turn photos off in Settings to fetch nothing.
- **Terminal limits:** a terminal cannot reproduce bitmap wallpaper, vector icons or true circles; icons are Unicode glyphs and the avatars are deliberately low resolution. Use a font with box-drawing and block elements (any common monospace font); emoji need an emoji-capable font.

## Linking a device

Every launch begins at **Welcome**, then continues through the agreement to **Link a device**. Returning users see their saved agreement pre-accepted and confirm it before WhatsCLI reconnects. The QR uses the lowest error-correction level and half-block cells, so a normal WhatsApp code (≈60×30 cells) is shown **inline** in a 130×42 terminal. If the terminal is too small it switches to a full-screen QR automatically, or tells you the size it needs. The code is a credential: never share it.

**Need help?** After two minutes without pairing, a *Need help? Open web QR* button appears (key `h`, or `/web`). It opens a **loopback-only** port with an unguessable URL, rejects foreign `Host` headers, sends no-store headers and shuts itself down after 15 minutes or when pairing succeeds. Over SSH, forward the port (the command is shown) and open the URL in your local browser. The terminal QR is always the primary route.

## Keyboard and mouse

Everything works without a mouse. Mouse input is enabled by default for onboarding (including existing installs); it can be disabled in Settings. Hover highlights, click chats/buttons/messages, double-click a message for its versions, and wheel scrolling.

| Context | Keys |
| --- | --- |
| Welcome | `↑`/`↓`/`←`/`→` or `Tab` choose · `Enter`/`Space` select · `q` quit |
| Global | `Ctrl+C` quit · `Ctrl+P` palette · `Tab` list ↔ chat · `Ctrl+F` search · `n` new chat · `e` archive · `s` settings · `?` help |
| Chat | `↑`/`↓` select message · `Ctrl+R` reply · `Ctrl+E` react · `i` type · `a` file browser · `Enter` message versions · `p`/`r` older/latest |
| Message box | `Enter` send · `Ctrl+J` newline · `Ctrl+U` clear · `Esc` stop typing |
| Link screen | `f` full screen · `h` web QR · `r` retry · `Esc` back |

### Slash commands

Type `/` in a chat for a live list (↑↓ choose, `Tab` completes): `/help /statuses /files /new <number> /meta /search <text> /archive /versions /older /latest /emoji /settings /update /connect /qr /web /mouse /clear /logout /quit`. Commands never send anything to WhatsApp. To send a message that really starts with a slash, begin it with `//`.

Replies quote the selected message; reactions are attached to the selected message. Both require a selected message (`↑`/`↓`), then `Ctrl+R` or `Ctrl+E`. The footer shows WhatsCLI's connection state, not your personal WhatsApp online presence. `/statuses` shows updates received while WhatsCLI is connected and can post text statuses. Older status updates are not fetched, and media stories are not downloaded.

### Files and folders

The `+` control, `a`, or `/files` opens the built-in file browser: browse, filter (`/`), sort (`s`), show hidden (`.`), type a path (`Ctrl+L`), multi-select files **and folders** (`Space`, `a`), then upload (`u`). WhatsApp has no folder message, so folders are zipped on the fly (symlinks are skipped) and sent as `.zip` documents. Every file type is sent as a document, up to **64 MiB** each, one at a time, stopping at the first failure. Paths always refer to the machine running WhatsCLI; over SSH copy files there first.

### Settings

Mouse support, reduced motion, profile photos, chat wallpaper, 12-hour clock, terminal bell, link/reconnect, unlink (confirmed), the file browser, and on-demand update checking.

## Meta AI

`/meta` (or the palette, or the rail's chat menu) opens Meta AI using whatsmeow's native bot identity and encryption. There is no separate API key and **no guarantee Meta enables it for your account or region**; real replies are unverified here. Check uncertain sends in WhatsApp before retrying.

## State, privacy and flags

| Flag | Meaning |
| --- | --- |
| `-demo` | Synthetic data in a temporary directory; never connects |
| `-offline` | No WhatsApp network operations |
| `-colors auto\|truecolor\|256` | Colour depth |
| `-state-dir PATH`, `-db PATH` | Settings/session/history directory; history database override |
| `-version` | Print the version |

State lives in `whats-cli` under your config directory (`settings.json`, `session.db`, `history.db`, `ui-state.json` for read marks and archived chats). History and device credentials are **not encrypted**; edits and revocations received are retained. A dropped SSH connection (SIGHUP) exits cleanly and keeps your login. Unread badges count messages that arrive after you last opened a chat; history synced on first login starts as read. See [docs/security.md](docs/security.md).

## Development and verification

```sh
go test ./... && go vet ./... && go test -race ./...
go build ./cmd/whats-cli
python3 -m unittest discover -s tests -p 'install_test.py'
python3 tests/terminal_smoke.py ./whats-cli          # SSH-style PTY checks
WHATSCLI_SNAPSHOT_DIR=/tmp/snap go test ./internal/app -run TestSnapshots
python3 tools/ansi2png.py /tmp/snap/workspace.ansi workspace.png   # compare with the mock-ups
go test ./internal/app ./internal/ui -run '^$' -bench . -benchmem
```

Figma's supervised HTTP preview is an import-status page, not the terminal client; see [.figma/make/SETUP_NOTES.md](.figma/make/SETUP_NOTES.md). See also [architecture](docs/architecture.md), [testing](docs/testing.md) and [BUGS.md](BUGS.md).

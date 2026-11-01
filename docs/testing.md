# Testing

## Verification status

Automated checks cover the renderer (every cell filled, no mid-row colour resets, exact widths from 30×10 to 200×60), the three reference screens, mouse hit-testing against the rendered regions, slash commands, the file browser and upload queue, unread/archive logic, settings wiring, folder zipping, avatar host rules, the web-pairing server, and SSH-style PTY sessions (24-bit colour without `COLORTERM`, live resizes, SGR mouse, SIGHUP).

**No live account pairing, real sends, real profile-picture downloads, real Meta AI exchange, or real SSH daemon have been exercised.** Synthetic fixtures and an offline PTY cannot establish WhatsApp compatibility, delivery, history completeness or account safety.

## Toolchain and local checks

The module declares **`go 1.26.0`**, **`toolchain go1.27.2`**, and the pinned whatsmeow pseudo-version `v0.0.0-20261030195514-56e5ad0581ce`. Use the selected Go 1.27.2 toolchain. Do not use the former Go 1.22 baseline as a compatibility claim. Linux is the present implementation target; `privatefs` uses Linux-specific APIs.

Run from the repository root:

```sh
go version
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/whats-cli
python3 -m unittest discover -s tests -p '*_test.py'
sh -n install.sh
python3 tests/terminal_smoke.py ./whats-cli
```

The race detector requires a supported platform/toolchain and its native compiler prerequisites. The automated suite uses fake clients, synthetic upstream events/QR data, and temporary files/databases. Some session tests generate synthetic device identity material locally but never pair it or connect to a service. No WhatsApp account, real device session, API key, or existing private history is required. Initial module/toolchain downloads may require network access; test logic does not connect to WhatsApp.

Do not continue through onboarding in the default live-mode app to test a local UI change: confirming the saved agreement begins connection. Use `-offline` and a disposable state directory.

## Existing coverage

| Test file | Scope checked |
| --- | --- |
| `internal/app/model_test.go` | Acceptance waits for successful save; failed save leaves network gated; pre-consent palette restrictions; Unicode filtering/backspace; composer treats shortcuts as text; stale page rejection; refresh keeps older cursors; nested overlays return correctly; recipient guard and `Ctrl+U`; minimum-size guidance and scrolling |
| `internal/app/workspace_test.go`, `size_test.go` | Layout bounds on every screen at many sizes; reference content; no background resets; hover/click/wheel against rendered regions; slash commands; settings; pairing/QR sizing (inline up to 230-byte payloads at 130×42); help button timer; unread/archive; explorer and upload plan; frame-size budget; benchmarks |
| `internal/messaging/recipients_test.go` | International-number validation; both upstream Meta AI identities; bot-invoke reply normalization; delivered/read/played receipt mapping |
| `tests/install_test.py` | Paths with spaces, safe update of an existing trusted checkout, stable-release lookup and ref validation, existing-command preservation, failed-build cleanup, symlink/unsupported-OS rejection, successful local rebuild, and preservation of the prior binary on failed rebuild |
| `internal/updater/updater_test.go` | GitHub stable-release response validation, SemVer ordering, and network error handling |
| `internal/app/runtime_test.go` | Runtime consent gating; fake-client text send/pending/self-echo reconciliation; admitted events drain at shutdown; closed runtime rejects operations; logout gating; incoming storage error becomes visible |
| `internal/config/config_test.go` | Settings/consent/mouse round trip; private save permissions; symlink and trailing JSON rejection |
| `internal/history/sqlite_test.go` | Original storage API preserves original/edit bodies and deletion marker across reopen |
| `internal/history/events_test.go` | Out-of-order originals/edits/revokes; permanent tombstones; scoped replay deduplication; account/chat isolation; stable pagination; outgoing echoes/status and revision pages; malformed-event rejection and rollback; legacy migration/reopen; file modes and symlink/sidecar rejection; literal filenames; closed-store errors; bounded reads and equal-time edit ties |
| `internal/messaging/whatsmeow_test.go` | Lazy constructor/cancelled connect without credentials; accepted-producer draining; concurrent idempotent close; synthetic QR refresh/expiry/error/cancel; local modernc session round trip; synthetic history sync; cached identity after upstream device deletion; fresh local session after revocation; direct/group target validation |
| `internal/messaging/normalize_test.go` | Text/nested edits, revocations, unsupported target handling, history/live edit identity consistency, and group-title behavior |
| `internal/messaging/attachments_test.go` | Host-local file metadata, spaces/size limit, symlink/nonregular rejection, nonblocking FIFO rejection, and safe-open final-symlink behavior |
| `internal/privatefs/privatefs_test.go` | New private directories/files, chmod of target files/directories, unchanged existing ancestors, symlink/hardlink/nonregular rejection |
| `internal/ui/text_test.go` | Terminal escape/control/bidi sanitization and QR size behavior without raw payload fallback |

Model tests invoke Bubble Tea state transitions directly; they are not end-to-end terminal tests. Fake send tests do not exercise a real document upload or service acknowledgement. QR tests use synthetic strings; they do not prove scanability with an actual linked-device QR and phone. Session round trips prove local serialization, not live session reuse. These are targeted cases, not exhaustive fault, filesystem, or security certification.

## Offline terminal smoke procedure

Use disposable state, never a real session or history directory. On Linux:

```sh
scratch=$(mktemp -d)
go run ./cmd/whats-cli -offline -state-dir "$scratch"
```

1. Confirm welcome → `Enter` → disclaimer. Without acceptance, `Enter` must not continue. Open the palette and confirm only help/settings/quit are available; `Esc` should restore the originating screen.
2. `Space`, then `Enter`: settings must save before continuation. Offline connection fails without producing a real QR or opening `session.db`. Do not interpret the waiting pairing view as an available live session.
3. Use `i` to reach the empty inbox. Filter the palette, try an unmatched filter and Unicode/backspace, run settings/help/conversations, and check nested `Esc` returns. Choose `connect` only in this offline process to check the controlled failure path.
4. Toggle mouse with settings `m`; check optional wheel navigation and keyboard alternatives. In scrollable views try arrows and `PgUp`/`PgDown`.
5. Confirm `q` quits outside input modes and `Ctrl+C` quits globally. Relaunch with the same `-offline -state-dir`: accepted consent/mouse should persist and the inbox should appear without onboarding. The offline connection attempt must still fail without service access.
6. Check the disposable directory for `settings.json` and `history.db`, directory/file modes, and absence of a real device-session database. A custom `-db` should move history only, not settings/session. Do not publish database contents.
7. Repeat at narrow widths/heights and in an SSH interactive terminal; check resize guidance, scrolling, mouse setting, alternate-screen restoration, interruption, and exit behavior. A single successful PTY/SSH run is not broad terminal compatibility certification.

A fresh offline database has no chats. Use `n` to create an empty local direct-chat entry, or `-demo` for disposable synthetic conversations and local-only demo sends. `-offline` currently stays in the `local` legacy account namespace, with no linked-account archive selector. Never seed real messages or credentials into committed fixtures.

After quitting, remove only the disposable directory you created when no longer needed. Do not delete a real state/history directory. No live account access is authorized by this smoke procedure.

## Remaining acceptance checks

These are **future checks requiring separate explicit authorization**, not completed work:

- Real spare-account QR scanability, expiry/retry/cancel, unsupported verification steps, linked-device visibility, and restart using the saved session.
- Network interruption/reconnect and upstream permanent-disconnect behavior; confirmed logout, server-side unlink verification, restart/re-pair, and account isolation in real events.
- Direct/group/bot text receive/send with harmless content; pending/result/self-echo ordering; failures/timeouts/cancellation and duplicate-avoidance guidance. Verify delivered/read receipts live; current tests establish mapping/storage behavior only, not all-member group-read semantics.
- Meta AI replies on an explicitly authorized paired account where Meta enables the feature. Neither new-chat creation nor a synthetic bot reply establishes account/region availability.
- History-sync availability and gaps; replay deduplication; originals/edits/revokes arriving out of order; only actually received text retained. Do not promise a full account archive.
- Explicit document preview/confirmation and a harmless host-local send, including over SSH; cancellation, limits, and recipient display. Never test with private files or infer immutable preview binding from the current independent reopen design.

Local gaps also include full network-send fault injection, attachment preview/send mutation races, exhaustive storage/disk-full/recovery and shutdown interleavings, crash durability, hostile writable-directory races/ACLs, broad filesystems/platforms, grapheme-aware input and Unicode spoofing, many terminal/SSH combinations, and performance/memory under large history sync. Queue backpressure and targeted tests are not an exactly-once/crash-recovery guarantee. Calls, rich stickers, and media story downloads remain unsupported; synthetic status, reply, and reaction tests do not verify live WhatsApp service behavior.

For any future live check, record exactly what was attempted and observed, keep real credentials/QR/history/media out of Git and diagnostics, and distinguish that record from automated/offline results.

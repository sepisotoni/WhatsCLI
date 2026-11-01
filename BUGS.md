# BUGS.md — WhatsCLI audit backlog

> Static inspection plus previous audit results, 2026-10-31. Some items are **confirmed by static analysis**, while others are **potential risks requiring reproduction**. Not an exhaustive line-by-line audit. This document does not mean tests were rerun.

| ID | Priority | Status | File(s) | Finding / resolution |
|---|---|---|---|---|
| WC-001 | Medium | **Fixed** | `internal/messaging/whatsmeow.go` | Deprecated `GenerateMessageID` replaced by a local generator that mirrors the upstream scheme and takes no operation lock, so it cannot stall behind `Connect`. |
| WC-002 | Medium | **Fixed** | `internal/config/config.go` | `Lstat`+`Open` race replaced by `O_NOFOLLOW` open and descriptor `fstat` validation (also used for the new `ui-state.json`). |
| WC-003 | Medium | Open | `internal/app/runtime.go` | Shutdown stress tests (blocked sends, DB delays, concurrent Connect/Close/Logout) still to be written; deadlock not confirmed. |
| WC-004 | Low | **Fixed** | `internal/ui` | Unused `Panel` helper removed with the old lipgloss styles. |
| WC-005 | Low | **Fixed** | `internal/ui/text_test.go` | The bidi override is now written as the escape `\u202e` instead of a literal character. |
| WC-006 | Low | Open | `internal/app` | Model is large; now split by screen file (`keys`, `view`, `workspace`, `overlays`, `explorer`). Further splitting optional. |
| WC-007 | Medium | **Fixed** | `internal/app/model.go` | Found in this sweep: a new chat's first message was marked read on first sight; read marks now start at the latest synced message unless it just arrived. |
| WC-008 | Low | **Fixed** | `internal/ui/canvas.go` | Found in this sweep: `TailFit` was quadratic for 4096-character drafts; now a single backwards scan. |
| WC-009 | Low | **Fixed** | `internal/app/mouse.go` | Found in this sweep: repeated mouse-motion reports within one cell re-ran hit testing. |

## Existing safeguards worth preserving
- Explicit user consent before WhatsApp connection.
- Attachment size limit, regular-file checks, and file identity validation.
- Pending outgoing messages persisted before send and warnings on uncertain delivery.
- SQLite transaction-based history and private session files.

## Verification
- The previous full-Go audit reported `go test ./...` and `go vet ./...` passing, with dead-code/static-analysis findings. Re-run on current HEAD to confirm.
- Add targeted filesystem swap and shutdown stress tests before refactoring.
- Avoid blindly re-sending messages after delivery uncertainty.

No application source was modified by this audit backlog.

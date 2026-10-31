# BUGS.md — WhatsCLI audit backlog

> Static inspection plus previous audit results, 2026-10-31. Some items are **confirmed by static analysis**, while others are **potential risks requiring reproduction**. Not an exhaustive line-by-line audit. This document does not mean tests were rerun.

| ID | Priority | Confidence | File(s) | Finding / next action |
|---|---|---|---|---|
| WC-001 | Medium | Static-analysis finding | `internal/messaging/whatsmeow.go:319` | `whatsmeow.GenerateMessageID()` was flagged deprecated. Migrate to supported client-based generation while preserving stable IDs for pending sends and before-connect behavior. Verify against installed whatsmeow version. |
| WC-002 | Medium | Code-level race risk | `internal/config/config.go` (`Load`) | `Lstat(path)` is followed by `os.Open(path)`: a path swap could cause a different file/symlink to be opened. Use no-follow descriptor-based file opening plus post-open `fstat` validation, reusing `internal/privatefs` where suitable. No exploit reproduced. |
| WC-003 | Medium | Reliability test needed | `internal/app/runtime.go` (`Close`, `Send`) | Shutdown waits on transport/event producers and accepted operations; test blocked network sends, DB delays, event backpressure, concurrent Connect/Close/Logout. Potential stall; **deadlock not confirmed**. |
| WC-004 | Low | Dead-code scan finding | `internal/ui/styles.go:14` | `Panel` was reported unreachable by dead-code analysis. Confirm intended use before deleting or wiring up. |
| WC-005 | Low | Static-analysis finding | `internal/ui/text_test.go:9,11` | Literal Unicode U+202E bidi override in tests was flagged. Express as `\u202e` or an explicit escaped literal with a clarifying comment. |
| WC-006 | Low | Maintainability | `internal/app/model.go` | Large UI model; consider splitting by screens/actions only after functional tests. Not an identified functional bug. |

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

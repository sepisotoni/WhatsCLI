# Security and privacy

## Scope, consent, and account risk

The code implements a live unofficial WhatsApp adapter, but **no live account pairing or real message/file sending has been performed**. Pairing, saved-session restoration, reconnects, history sync, sending, and unlinking are not service-verified. WhatsCLI is independent and not endorsed by WhatsApp or Meta. Account restrictions, service-policy risks, protocol changes, data loss, and unreliable delivery remain possible; do not use it for critical communication.

Consent is versioned and persisted in `settings.json`. Connection is enabled only after acceptance has been saved successfully; the UI restricts pre-consent commands/history loading and the runtime gates connect/send/logout. The real adapter constructor is lazy: no session files, device credentials, or WhatsApp connection are created until connection is invoked. State-directory preparation and history opening/migration do happen before consent.

Saved acceptance enables an **automatic connection attempt on later live-mode launches**. Use `-offline` to prohibit WhatsApp network operations; do not assume restarting will show a fresh disclaimer. Consent is not authentication, filesystem access control, or a sandbox. Anyone with control of the same OS account can inspect/change settings, history, and credentials. There is no in-app consent-revocation editor.

## Credentials and pairing material

`session.db` contains sensitive linked-device identity/key material, without application-level encryption. Quitting preserves the session. Confirmed `logout` requests unlinking and requires pairing again; it does not erase local retained history, backups, or stored consent. A failed unlink may leave the device linked: verify linked devices in the official app rather than assuming local failure messages prove revocation.

A pairing QR is a credential, not a harmless onboarding image. It is rendered only for local display, with fullscreen/size guidance, and is not printed as a raw fallback string. The adapter uses no-op upstream logging and surfaces generic operation errors instead of raw QR/error payloads. Do not record, screenshot, share, or paste a QR into an issue. Terminal recordings, screen sharing, remote host administrators, and a compromised terminal can still expose it. `q`/`Ctrl+C` stop the process; pairing-screen `Esc` only exits fullscreen and does not cancel connection.

No WhatsApp API key is needed for the application or synthetic tests. Dependency/toolchain downloads can use the network independently of `-offline`.

## Plaintext history and deliberate retention

Conversation titles, identifiers, sender metadata, message bodies, timestamps, delivery outcomes, received revisions, and deletion markers are stored without application-level encryption. Session and history are separate databases, but both belong to the execution host.

- An edit preserves earlier received text, not just the newest body.
- A revoke/deletion marker preserves received plaintext and revisions; it is **not erasure**. The UI explicitly marks locally retained deletions and exposes retained versions.
- Only content actually supplied to this client can be retained. Unseen originals, missing revisions, or expired/unavailable content cannot be reconstructed.
- Supported ephemeral/view-once wrappers are normalized into ordinary retained text or placeholders. There is no local disappearing-message expiry or view-once enforcement; media are not downloaded automatically.
- Logging out retains local history. There is no retention deadline, purge UI, secure-erasure feature, or encrypted database.

Use an encrypted disk/home directory and protected backups; assess whether retaining other people's edited, revoked, or disappearing content is appropriate. WhatsApp transport encryption does not encrypt this local history. SQLite WAL/journal files, backups, snapshots, swap, and terminal records can retain content too. Deleting one database file is not a secure-erasure procedure.

## Filesystem protections and honest limits

Default state lives in `whats-cli` beneath `os.UserConfigDir()` (normally `$XDG_CONFIG_HOME/whats-cli` or `$HOME/.config/whats-cli` on Linux). `-state-dir` changes settings/session/default history together; `-db` changes history only. Choose a dedicated private directory, not a repository checkout or shared directory.

The current `privatefs` implementation is Linux-specific. It walks directory components with no-follow directory opens, creates missing directories with mode `0700`, and rejects symlink components. `EnsureDir` changes the target directory to `0700` but does not chmod existing ancestors. Startup uses this for the state directory; settings saves also secure their target directory.

`EnsureFile` prepares database files **before** SQLite opens them: `0600`, regular file, one hard link, no final symlink. Existing history/session `-wal`, `-shm`, and `-journal` sidecars are checked similarly before opening; history sidecars are also checked after initialization. Settings writes use a private temporary file, file sync, and rename. These protections are stronger than simply chmodding a database after opening, but are not comprehensive isolation:

- `-db` with an existing external parent does **not** harden that parent; newly missing parents are created private. Existing ancestors remain unchanged.
- No ownership/ACL audit is performed. Permission bits are not encryption and cannot defend against root, same-user attackers, a compromised host, or exposed backups.
- SQLite and configuration loading subsequently reopen paths. An attacker able to replace entries in writable parent directories can race checks; the code explicitly relies on a trusted private directory. It is not a fully race-free hostile-filesystem sandbox.
- Sidecar checks cover files present at defined checkpoints, not continuous monitoring of every later SQLite-created sidecar. Keep the whole directory private.
- Configuration loading uses `Lstat` plus open rather than a fully descriptor-relative no-follow traversal. Saving syncs the temporary file, not the containing directory; crash durability is not guaranteed.
- Modes and symlink rejection do not certify behavior on all filesystems, containers, network mounts, or non-Linux systems.

Over SSH, the remote host stores credentials/history and reads attachments. SSH secures the terminal connection, not that host's storage, backups, screen recordings, or administrator access.

## Text, recipients, sends, and attachments

Display helpers strip terminal escape sequences, C0/C1 controls, and selected bidirectional overrides/isolate controls, preserving ordinary Unicode. Chat rows use a single-line form and revision details preserve safe newlines. This reduces terminal-command injection; it is not comprehensive anti-spoofing, grapheme-aware editing, or a Unicode security audit. Raw incoming text is stored, not rewritten by this display sanitization.

A nonempty composer draft or pending send prevents switching chats, helping prevent accidental recipient changes. `Ctrl+U` clears a draft while composing; `Esc` leaves it intact. Outgoing IDs are persisted as pending before network sends. Cancellation/failure is treated conservatively as delivery unknown, not definitely unsent; check the official app before retrying. There is no automatic exactly-once delivery guarantee. An incoming storage failure is visible and blocks future sends, but failed incoming events are not durably queued for replay.

Files are sent as documents only after a preview and explicit confirmation, with a **10 MiB** limit. Paths are literal host-local paths; over SSH, transfer laptop files with SCP/SFTP first. Inspection rejects final symlinks, devices, FIFOs, and oversized files; Unix opening uses `O_NOFOLLOW`/`O_NONBLOCK`, checks identity against `Lstat`, and bounded reads detect length changes. There is no automatic media downloading or opening, archive extraction, or execution.

Limits: intermediate attachment-path symlinks are not prohibited; hard links are not rejected for attachments. Preview and send reopen the path independently and are not bound by a content hash or retained descriptor, so replacement/modification between preview and confirmation can send changed content. Equal-length changes during reading are not detected. Select files from a trusted stable location, re-preview after changes, and do not treat confirmation as an immutable-content guarantee. Plaintext/encrypted attachment buffers remain in process memory; no explicit zeroization is implemented. MIME sniffing and regular-file checks do not establish that document content is safe for recipients.

## Keep runtime material out of Git and diagnostics

Never commit device/session credentials, pairing QR material, history databases and sidecars, private configuration, populated environment files, private logs, media, or backups. `.gitignore` covers common environment/database/WAL/shared-memory patterns and `state/`, `sessions/`, `media/`, and `logs/`, but not every custom name or journal/backup suffix; it cannot untrack already committed content. Placeholder-only examples are the exception, not permission to embed real values.

Review staged paths and content. Keep secrets out of shell history, screenshots, issue attachments, and test fixtures. Tests use synthetic data and temporary state, not a real account. If exposed, unlink the affected device or rotate/revoke the credential through its issuer; deleting a local copy or adding an ignore rule is not remediation for an already exposed secret.

# whats-cli

A keyboard-first Go terminal app foundation for WhatsApp-style messaging, designed to work over SSH and on low-resource computers.

## Current status

The first-run flow follows the supplied mockups: welcome, explicit disclaimer consent, then a device-linking information screen. The app currently has a local conversation view, a filterable command palette, a settings screen, and durable SQLite message history. It does **not** connect to WhatsApp, generate a pairing QR code, or claim message delivery works.

Message edits are stored as revisions and deletions as markers; this preserves local records instead of erasing them. Local history is not encrypted in this initial version. The database is created with restrictive local file/directory permissions, but use an encrypted disk or home directory if message content is sensitive.

## Run

```sh
go run ./cmd/whats-cli
```

The database defaults to the user's config directory under `whats-cli/history.db`. Override it with `-db /path/to/history.db`.

## Keyboard controls

- `Enter` — move from welcome to consent; on consent, continue only after acceptance
- `Space` — toggle the consent checkbox
- `i` — open local inbox from the not-connected device-linking screen
- `/` — open and filter the command palette; use arrows and Enter
- `s` — settings from the inbox; Esc returns
- `q` — quit

Mockup inspection uses `inspect_images.py` and reads `GEMINI_API_KEY` from the local environment. Keep API keys out of source files and shell history; the project `.env` file is ignored by Git.

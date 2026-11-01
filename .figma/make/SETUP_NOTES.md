# Initial import setup

## Environment

No application API keys or other required environment inputs were found.
WhatsApp authentication uses an explicitly consented linked-device QR flow,
not a developer API key. No populated environment files were inspected.

## Native application versus browser preview

This is a Linux Go/Bubble Tea terminal executable. It requires a terminal/PTY;
it does not produce a deployable static website or use the generated
Payload/Next.js template.

- `install` verifies modules and compiles `bin/whatscli` using Go 1.27.2.
- `dev` uses `exec` to run a small Python HTTP **import-status page** on `$PORT`.
  It never launches the live client, opens device credentials, or exposes history.
  Passing HTTP bootstrap checks is not a live WhatsApp test.
- `./bin/whatscli -demo` runs the actual TUI with synthetic conversations in an
  automatically removed temporary state directory. It never constructs a real
  WhatsApp adapter. Demo sends are local synthetic events.
- `deploy` is intentionally absent: no supported static output was identified.
  `figma make verify-deploy` reports the missing script. Publishing native binaries
  is not the Figma static deployment contract. A separate website or web-terminal
  host would be a separate deployment approach requiring user confirmation.
- `figma make verify-deploy-preview --skip` records the unsupported native
  application preview deployment as skipped.

No subagent capability was available; deployment and preview instructions were
loaded independently alongside repository reads. Deployment builds were not
started while bootstrap was pending.

## Acceptance boundary

Run the commands in README.md for local verification. Synthetic tests and an
offline PTY run cannot certify real WhatsApp pairing, real sends, or Meta AI
availability. Meta AI uses the pinned upstream bot JID and native bot encryption;
it is not a fabricated LLM response or a separate API-key integration. Meta can
restrict the feature by account, country, or protocol changes.

The one-command installer URL becomes usable after `install.sh` is published to
the repository's `main` branch. This work does not push code or publish releases.

#!/usr/bin/env python3
"""Safe import-status page for a native terminal application, not a web client."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PAGE = b"""<!doctype html><html lang="en"><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>WhatsCLI - native terminal client</title>
<style>
:root{color-scheme:dark;font-family:ui-monospace,monospace;background:#00120d;color:#e0f5ec}
body{max-width:760px;margin:12vh auto;padding:32px}
main{border:1px solid #00e99a;border-radius:18px;padding:40px}
h1{color:#00e99a;font-size:40px}p{line-height:1.8;color:#a0b9ad}
pre{padding:20px;background:#00251a;overflow:auto;border-radius:8px}
small{color:#6fa28b}a{color:#00e99a}
</style><main><small>NATIVE GO APPLICATION / IMPORT STATUS</small>
<h1>&gt;_ WhatsCLI</h1><p>Your conversations. Your terminal.</p>
<p>This repository is a native, keyboard-first terminal client, not a browser
WhatsApp client. This status page does not connect to WhatsApp or display private
messages. Run the actual application in a Linux terminal:</p>
<pre>./bin/whatscli -demo\n\n# Live mode (asks for consent before connecting)\n./bin/whatscli</pre>
<p>The safe demo uses synthetic conversations and disposable local state.
Press Tab to move between panes, i to compose, and / for commands.</p>
<p>Web publishing is not configured: the native executable requires a terminal
and is distributed through the repository installer instead.</p></main></html>"""


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ("/", "/index.html"):
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(PAGE)))
        self.send_header("Content-Security-Policy",
                         "default-src 'none'; style-src 'unsafe-inline'")
        self.end_headers()
        self.wfile.write(PAGE)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, required=True)
    args = parser.parse_args()
    ThreadingHTTPServer(("0.0.0.0", args.port), Handler).serve_forever()

#!/usr/bin/env python3
"""Send the project's UI mockups to Gemini and print a visual description."""

import base64
import mimetypes
import os
from pathlib import Path
import sys

import requests

IMAGE_PATHS = [
    Path("/home/sepisotoni/projects/WhatsCLI/ChatGPT Image Oct 31, 2026, 12_32_07 PM.png"),
    Path("/home/sepisotoni/projects/WhatsCLI/ChatGPT Image Oct 31, 2026, 12_32_45 PM.png"),
    Path("/home/sepisotoni/projects/WhatsCLI/ChatGPT Image Oct 31, 2026, 12_33_02 PM.png"),
]
MODEL = os.environ.get("GEMINI_MODEL", "gemini-2.5-flash")
API_URL = f"https://generativelanguage.googleapis.com/v1beta/models/{MODEL}:generateContent"


def main():
    api_key = os.environ.get("GEMINI_API_KEY")
    if not api_key:
        sys.exit("GEMINI_API_KEY is not set. Set it in your shell, then run this script again.")

    missing = [str(path) for path in IMAGE_PATHS if not path.is_file()]
    if missing:
        sys.exit("Missing image file(s):\n" + "\n".join(missing))

    parts = [{
        "text": (
            "Inspect these UI mockups. For each image, describe the screen, readable text, "
            "colors and layout, then infer the apparent flow across the images. Distinguish "
            "what is clearly visible from uncertain details; do not guess unreadable text."
        )
    }]
    for path in IMAGE_PATHS:
        mime_type = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
        parts.append({"text": f"\nImage: {path.name}"})
        parts.append({
            "inline_data": {
                "mime_type": mime_type,
                "data": base64.b64encode(path.read_bytes()).decode("ascii"),
            }
        })

    try:
        response = requests.post(
            API_URL,
            headers={"x-goog-api-key": api_key},
            json={"contents": [{"parts": parts}]},
            timeout=180,
        )
        response.raise_for_status()
        result = response.json()
    except requests.RequestException as exc:
        sys.exit(f"Gemini request failed: {exc}")
    except ValueError:
        sys.exit("Gemini returned a response that was not valid JSON.")

    try:
        text = result["candidates"][0]["content"]["parts"]
        print("\n".join(part["text"] for part in text if "text" in part))
    except (KeyError, IndexError, TypeError):
        sys.exit("Gemini response did not contain a text answer:\n" + str(result))


if __name__ == "__main__":
    main()

#!/bin/sh
# Run with: curl -fsSL https://raw.githubusercontent.com/sepisotoni/WhatsCLI/main/install.sh | sh
set -eu

fail() { printf '\033[31m  error\033[0m  %s\n' "$*" >&2; exit 1; }
step() { printf '\033[32m  >\033[0m %s\n' "$*"; }

[ "$(uname -s)" = Linux ] || fail "WhatsCLI currently supports Linux only (including WSL2)."
command -v git >/dev/null 2>&1 || fail "Install git using your system package manager, then retry."
command -v go >/dev/null 2>&1 || fail "Install Go 1.26+ from go.dev/dl, then retry. Go automatically selects the pinned 1.27.2 toolchain."
command -v mktemp >/dev/null 2>&1 || fail "mktemp is required."

directory=${WHATSCLI_DIR:-"$HOME/whatscli"}
bindir=${WHATSCLI_BIN_DIR:-"$HOME/.local/bin"}
ref=${WHATSCLI_REF:-}
channel=stable
action=install

build() {
    go mod download
    go mod verify
    version=${ref#v}
    if [ "$ref" = "main" ] || [ -z "$ref" ]; then
        version=$(sed -n '1p' VERSION)
    fi
    printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-beta\.(0|[1-9][0-9]*))?$' ||
        fail "Release version must be stable X.Y.Z or beta X.Y.Z-beta.N."
    mkdir -p bin
    # Rename instead of writing over a running executable.
    artifact=$(mktemp bin/.whatscli-build.XXXXXX)
    if go build -mod=readonly -trimpath -ldflags "-X whatscli/internal/updater.BuildVersion=$version" -o "$artifact" ./cmd/whats-cli; then
        chmod 755 "$artifact"
        mv -f "$artifact" bin/whatscli
    else
        rm -f "$artifact"
        fail "Compilation failed; the previous executable was not replaced."
    fi
}

resolve_ref() {
    if [ -z "$ref" ]; then
        command -v curl >/dev/null 2>&1 || fail "curl is required to find the latest stable GitHub release."
        if [ "$channel" = "beta" ]; then
            command -v python3 >/dev/null 2>&1 || fail "python3 is required to select the latest beta release."
            response=$(curl -fsSL --max-time 15 -H 'Accept: application/vnd.github+json' \
                'https://api.github.com/repos/sepisotoni/WhatsCLI/releases?per_page=100') ||
                fail "Could not find the latest beta GitHub release."
            ref=$(printf '%s' "$response" | python3 -c '
import json, re, sys
releases = json.load(sys.stdin)
pattern = re.compile(r"^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)-beta\.(?:0|[1-9][0-9]*)$")
for release in releases:
    tag = release.get("tag_name", "")
    if release.get("prerelease") and not release.get("draft") and pattern.fullmatch(tag):
        print(tag)
        break
') || fail "Could not parse the beta release list."
            [ -n "$ref" ] || fail "No published beta releases were found."
        else
            response=$(curl -fsSL --max-time 15 -H 'Accept: application/vnd.github+json' \
                https://api.github.com/repos/sepisotoni/WhatsCLI/releases/latest) ||
                fail "Could not find the latest stable GitHub release."
            ref=$(printf '%s\n' "$response" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p')
        fi
    fi
    case "$ref" in
        main) ;;
        v*)
            printf '%s\n' "$ref" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-beta\.(0|[1-9][0-9]*))?$' ||
                fail "WHATSCLI_REF must be main or a stable/beta release tag such as v1.2.3 or v1.3.0-beta.1."
            ;;
        *) fail "WHATSCLI_REF must be main or a stable/beta release tag." ;;
    esac
}

update() {
    case "$directory" in /*) ;; *) fail "WHATSCLI_DIR must be an absolute path." ;; esac
    [ ! -L "$directory" ] || fail "Refusing a symlink installation directory."
    [ -d "$directory/.git" ] || fail "$directory is not a WhatsCLI Git checkout."
    origin=$(git -C "$directory" remote get-url origin)
    case "$origin" in
        https://github.com/sepisotoni/WhatsCLI.git|git@github.com:sepisotoni/WhatsCLI.git) ;;
        *) fail "Unexpected origin URL; refusing to update this checkout." ;;
    esac
    [ -z "$(git -C "$directory" status --porcelain)" ] || fail "Checkout has local changes; preserve or commit them before updating."
    resolve_ref
    if [ "$ref" != main ] && [ -x "$directory/bin/whatscli" ]; then
        target_version=${ref#v}
        installed_version=$("$directory/bin/whatscli" -version 2>/dev/null | sed -n 's/^WhatsCLI //p')
        if [ "$installed_version" = "$target_version" ]; then
            step "Already up to date (WhatsCLI $target_version); existing files were left unchanged."
            return 0
        fi
    fi
    step "Checking out WhatsCLI $ref"
    if [ "$ref" = main ]; then
        git -C "$directory" fetch origin main || fail "Could not fetch main; the installed executable was not replaced."
        git -C "$directory" checkout --detach FETCH_HEAD || fail "Could not check out main; the installed executable was not replaced."
    else
        git -C "$directory" fetch --depth 1 origin "refs/tags/$ref:refs/tags/$ref" ||
            fail "Could not fetch release $ref; the installed executable was not replaced."
        git -C "$directory" checkout --detach "$ref" ||
            fail "Could not check out release $ref; the installed executable was not replaced."
    fi
    step "Building the updated client"
    if (cd "$directory" && build); then
        step "Updated successfully to $ref: $directory/bin/whatscli"
    else
        fail "Source is checked out at $ref, but the build failed; the prior executable remains installed."
    fi
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --beta) channel=beta ;;
        --update) action=update ;;
        --local) action=local ;;
        *) fail "Unknown option: $1. Use --beta, --update, or --local." ;;
    esac
    shift
done

if [ "$action" = local ]; then
    [ "$channel" = stable ] || fail "--beta cannot be combined with --local."
    [ -f go.mod ] && grep -q '^module whatscli$' go.mod || fail "Run --local from the WhatsCLI repository root."
    step "Building this checkout without pulling or changing your source"
    build
    step "Ready: ./bin/whatscli -demo"
    exit 0
fi

if [ "$action" = update ]; then
    [ "$(uname -s)" = Linux ] || fail "WhatsCLI currently supports Linux only (including WSL2)."
    update
    exit 0
fi

case "$directory" in /*) ;; *) fail "WHATSCLI_DIR must be an absolute path." ;; esac
case "$bindir" in /*) ;; *) fail "WHATSCLI_BIN_DIR must be an absolute path." ;; esac
if [ "$#" -eq 0 ] && [ -e "$directory" ]; then
    step "Existing installation found; updating it"
    update
    exit 0
fi
[ ! -L "$directory" ] || fail "Refusing a symlink installation directory."
[ ! -e "$directory" ] || fail "$directory already exists; nothing was changed. See README.md for safe update instructions."

printf '\n\033[1;32m  WhatsCLI\033[0m  Your conversations. Your terminal.\n\n'
resolve_ref
mkdir -p "$(dirname "$directory")" "$bindir"
work=$(mktemp -d "$(dirname "$directory")/.whatscli-install.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
step "Downloading WhatsCLI $ref from sepisotoni/WhatsCLI"
git clone --depth 1 --branch "$ref" -- https://github.com/sepisotoni/WhatsCLI.git "$work/source"
step "Verifying dependencies and compiling the native client"
(
    cd "$work/source"
    build
)
[ ! -e "$directory" ] || fail "Installation directory appeared during the build; refusing to overwrite it."
mv "$work/source" "$directory"
if [ -e "$bindir/whatscli" ] || [ -L "$bindir/whatscli" ]; then
    step "Existing $bindir/whatscli left unchanged."
else
    ln -s "$directory/bin/whatscli" "$bindir/whatscli"
fi
step "Installed in $directory"
printf '\n  Safe demo: "%s/bin/whatscli" -demo\n  Live mode: "%s/bin/whatscli"\n' "$directory" "$directory"
case ":${PATH:-}:" in
    *":$bindir:"*) ;;
    *) printf '\n  Add to your shell profile: export PATH="%s:$PATH"\n' "$bindir" ;;
esac
printf '\n  Unofficial client. Read the consent notice before linking an account.\n'

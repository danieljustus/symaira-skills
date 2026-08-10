#!/bin/bash
set -euo pipefail

# Navigation context: start from client folder
cd "$(dirname "$0")"

# Auto-detect Xcode developer directory if active toolchain points to CLI tools
if [ -d "/Applications/Xcode-beta.app" ]; then
    export DEVELOPER_DIR="/Applications/Xcode-beta.app/Contents/Developer"
elif [ -d "/Applications/Xcode.app" ]; then
    export DEVELOPER_DIR="/Applications/Xcode.app/Contents/Developer"
fi
echo "Using DEVELOPER_DIR=${DEVELOPER_DIR:-default}"

# App marketing version: the release tag is the single source of truth
# (see docs/versioning.md). CI passes APP_VERSION from the tag. Local
# builds override the project default only when checked out at an exact
# release tag; feature-branch descriptions are not valid app versions.
if [ -z "${APP_VERSION:-}" ]; then
    APP_VERSION="$(git describe --tags --exact-match 2>/dev/null | sed 's/^v//' || true)"
fi
echo "=== App marketing version: ${APP_VERSION:-<project default>} ==="

# Optional code signing identity (set by CI for signed releases)
CODESIGN_IDENTITY="${CODESIGN_IDENTITY:-}"
if [ -n "$CODESIGN_IDENTITY" ]; then
    echo "=== Signing identity: ${CODESIGN_IDENTITY} ==="
else
    echo "=== No signing identity set — building unsigned (ad-hoc) ==="
fi

echo "=== 1. Building Go symskills Binary ==="
cd ..
CGO_ENABLED=0 go build -ldflags "-s -w" -o symskills ./cmd/symskills

if [ -n "$CODESIGN_IDENTITY" ]; then
    echo "Signing Go binary with identity: $CODESIGN_IDENTITY"
    codesign --force --timestamp --options runtime -s "$CODESIGN_IDENTITY" symskills
    echo "Go binary signature verification:"
    codesign -dvvv symskills 2>&1 | head -5
fi

cd client

echo "=== 2. Generating Xcode Project ==="
if ! command -v xcodegen &> /dev/null; then
    echo "ERROR: xcodegen is not installed. Install via: brew install xcodegen"
    exit 1
fi
xcodegen generate

echo "=== 3. Cleaning Build Directory ==="
rm -rf build

echo "=== 4. Archiving App with xcodebuild ==="
XCODEBUILD_FLAGS=(
    -project Symskills.xcodeproj
    -scheme Symskills
    -configuration Release
    -archivePath build/Symskills.xcarchive
    archive
    CODE_SIGN_IDENTITY="-"
    CODE_SIGN_STYLE="Manual"
)
if [ -n "${APP_VERSION:-}" ]; then
    XCODEBUILD_FLAGS+=(MARKETING_VERSION="$APP_VERSION")
fi
xcodebuild "${XCODEBUILD_FLAGS[@]}"

echo "=== 5. Signing App Bundle ==="
APP_BUNDLE="build/Symskills.xcarchive/Products/Applications/Symskills.app"

if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: Application build failed, could not find $APP_BUNDLE"
    exit 1
fi

echo "=== 5a. Verifying App Marketing Version ==="
BUILT_VERSION="$(plutil -extract CFBundleShortVersionString raw -o - "$APP_BUNDLE/Contents/Info.plist" 2>/dev/null || true)"
echo "Built app version: ${BUILT_VERSION:-unknown}"
if [ -n "${APP_VERSION:-}" ]; then
    if [ "$BUILT_VERSION" != "$APP_VERSION" ]; then
        echo "ERROR: Built app version '$BUILT_VERSION' does not match release version '$APP_VERSION'." >&2
        echo "Fix the version derivation (see docs/versioning.md) before publishing." >&2
        exit 1
    fi
    echo "App version matches: $APP_VERSION"
fi

if [ -n "$CODESIGN_IDENTITY" ]; then
    echo "Signing with identity: $CODESIGN_IDENTITY"
    codesign --deep --force --timestamp --options runtime \
        -s "$CODESIGN_IDENTITY" \
        "$APP_BUNDLE"
    echo "Signing verification:"
    codesign -dvvv "$APP_BUNDLE" 2>&1 | head -5
else
    echo "Skipping code signing (ad-hoc build)"
fi

echo "=== 6. Packaging into DMG ==="
echo "Creating branded DMG with drag-to-Applications window..."
rm -f build/Symskills.dmg
"${SCRIPT_DIR:-.}/../scripts/create-symaira-dmg.sh" \
    "$APP_BUNDLE" \
    build/Symskills.dmg \
    "Symskills"

echo "=== Packaging Complete! ==="
echo "DMG created successfully: client/build/Symskills.dmg"

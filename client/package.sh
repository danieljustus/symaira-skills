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
xcodebuild "${XCODEBUILD_FLAGS[@]}"

echo "=== 5. Signing App Bundle ==="
APP_BUNDLE="build/Symskills.xcarchive/Products/Applications/Symskills.app"

if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: Application build failed, could not find $APP_BUNDLE"
    exit 1
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

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

echo "=== 1. Building Go symskills Binary ==="
cd ..
CGO_ENABLED=0 go build -ldflags "-s -w" -o symskills cmd/symskills/main.go
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
xcodebuild -project Symskills.xcodeproj \
           -scheme Symskills \
           -configuration Release \
           -archivePath build/Symskills.xcarchive \
           archive \
           CODE_SIGN_IDENTITY="-" \
           CODE_SIGN_STYLE="Manual"

echo "=== 5. Packaging into DMG ==="
APP_SRC="build/Symskills.xcarchive/Products/Applications/Symskills.app"

if [ ! -d "$APP_SRC" ]; then
    echo "ERROR: Application build failed, could not find $APP_SRC"
    exit 1
fi

DMG_STAGE="build/dmg_stage"
rm -rf "$DMG_STAGE"
mkdir -p "$DMG_STAGE"

echo "Copying App to staging..."
cp -R "$APP_SRC" "$DMG_STAGE/"

echo "Creating Applications symlink..."
ln -s /Applications "$DMG_STAGE/Applications"

echo "Creating DMG..."
rm -f build/Symskills.dmg
hdiutil create -volname "Symskills" \
               -srcfolder "$DMG_STAGE" \
               -ov \
               -format UDZO \
               build/Symskills.dmg

echo "=== Packaging Complete! ==="
echo "DMG created successfully: client/build/Symskills.dmg"

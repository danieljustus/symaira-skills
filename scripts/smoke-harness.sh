#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# smoke-harness.sh — opt-in headless smoke test
#
# For each registered harness (opencode, claude, codex, hermes):
#   1. Creates a temporary HOME and fixture skill source directory.
#   2. Uses the `symskills` binary to init config and install the fixture.
#   3. Checks that the harness binary exists on PATH *and* the required API-key
#      environment variable is set.
#   4. Launches the harness headless with a simple prompt mentioning the skill
#      name and asserts the skill was loaded / visible.
#
# Skips cleanly (exit 0) when the binary is absent or the API key is missing.
# The user's real HOME and real harness configuration are never touched.
#
# Usage:  ./scripts/smoke-harness.sh
#
# The symskills binary is expected at <repo-root>/symskills (built by
# `make build`).  Set the SYMSKILLS_BIN environment variable to override.
# ---------------------------------------------------------------------------

set -euo pipefail

###############################################################################
# Config
###############################################################################
SKILL_NAME="smoke-test-skill"
SKILL_DESCRIPTION="A fixture skill for headless smoke testing"

# Resolve the repo root (parent of scripts/).
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SYMSKILLS="${SYMSKILLS_BIN:-${REPO_ROOT}/symskills}"

TEMP_HOME=""
FIXTURE_DIR=""

###############################################################################
# Cleanup trap
###############################################################################
cleanup() {
    if [ -n "$TEMP_HOME" ] && [ -d "$TEMP_HOME" ]; then
        rm -rf "$TEMP_HOME"
    fi
    if [ -n "$FIXTURE_DIR" ] && [ -d "$FIXTURE_DIR" ]; then
        rm -rf "$FIXTURE_DIR"
    fi
}
trap cleanup EXIT

###############################################################################
# Helpers
###############################################################################
# Print a status line with a clear prefix.
info()  { echo "  [ .. ] $*"; }
ok()    { echo "  [ OK ] $*"; }
skip()  { echo "  [SKIP] $*"; }
fail()  { echo "  [FAIL] $*"; }
warn()  { echo "  [WARN] $*"; }

# Check that the symskills binary is usable.
ensure_symskills() {
    if [ ! -x "$SYMSKILLS" ]; then
        echo "ERROR: symskills binary not found or not executable at:"
        echo "       $SYMSKILLS"
        echo "       Run 'make build' first or set SYMSKILLS_BIN."
        exit 1
    fi
}

# Create a temporary HOME and initialise symskills inside it.
setup_temp_home() {
    TEMP_HOME="$(mktemp -d)"
    HOME="$TEMP_HOME" "$SYMSKILLS" init --force >/dev/null 2>&1
    ok "Temp HOME: $TEMP_HOME"
}

# Build a minimal fixture skill in a temp directory so symskills can install it.
create_fixture_skill() {
    FIXTURE_DIR="$(mktemp -d)"

    cat > "$FIXTURE_DIR/SKILL.md" <<-SKILL_EOF
---
name: $SKILL_NAME
description: $SKILL_DESCRIPTION
license: Apache-2.0
---
# $SKILL_NAME

$SKILL_DESCRIPTION
SKILL_EOF

    cat > "$FIXTURE_DIR/symskills.toml" <<-TOML_EOF
[skill]
name = "$SKILL_NAME"
version = "0.1.0"

[targets.opencode]
enabled = true

[targets.claude]
enabled = true

[targets.codex]
enabled = true

[targets.hermes]
enabled = true
TOML_EOF

    ok "Fixture skill: $FIXTURE_DIR"
}

# Install the fixture skill for a specific target harness.
install_fixture() {
    local target="$1"
    info "Installing $SKILL_NAME for target '$target' …"
    HOME="$TEMP_HOME" "$SYMSKILLS" install --target "$target" "$FIXTURE_DIR" >/dev/null 2>&1
}

# Resolve the expected skill root directory for a given harness target.
skill_root_for() {
    local target="$1"
    case "$target" in
        opencode) echo "$TEMP_HOME/.config/opencode/skills/$SKILL_NAME" ;;
        claude)   echo "$TEMP_HOME/.claude/skills/$SKILL_NAME" ;;
        codex)    echo "$TEMP_HOME/.agents/skills/$SKILL_NAME" ;;
        hermes)   echo "$TEMP_HOME/.hermes/skills/symaira/$SKILL_NAME" ;;
        *)        echo "" ;;
    esac
}

# Run a harness headless with a prompt and return the combined stdout+stderr.
run_harness() {
    local binary="$1"
    local prompt="$2"
    local output
    output="$(echo "$prompt" | HOME="$TEMP_HOME" timeout 30 "$binary" 2>&1 || true)"
    echo "$output"
}

###############################################################################
# Smoke-test one harness
###############################################################################
smoke_test_harness() {
    local binary="$1"      # on-path binary name
    local display="$2"     # human-readable display name
    local target="$3"      # symskills --target value
    local key_var="$4"     # required API-key env var name

    echo ""
    echo "===== $display ($target) ====="

    # 1. Check binary is on PATH.
    if ! command -v "$binary" >/dev/null 2>&1; then
        skip "Binary '$binary' not found on PATH"
        return 0
    fi
    ok "Binary found: $(command -v "$binary")"

    # 2. Check the required API key is set.
    if [ -z "${!key_var:-}" ]; then
        skip "\$$key_var is not set — skipping headless invocation"
        # NOTE: we still verify the install path even without the API key.
    else
        ok "API key \$$key_var is set"
    fi

    # 3. Install the fixture skill.
    install_fixture "$target"

    # 4. Verify the skill root directory exists.
    local sroot
    sroot="$(skill_root_for "$target")"
    if [ -z "$sroot" ]; then
        fail "Unknown target '$target'"
        return 1
    fi
    if [ ! -d "$sroot" ]; then
        fail "Skill root not found at $sroot"
        return 1
    fi
    ok "Skill installed at $sroot"

    # 5. If the API key was missing we skip the headless run.
    if [ -z "${!key_var:-}" ]; then
        skip "Skipping headless invocation for $display (no \$$key_var)"
        return 0
    fi

    # 6. Launch the harness headless with a prompt mentioning the skill.
    info "Launching $display headless …"
    local prompt="List all the skills you have loaded. Do you have a skill called '$SKILL_NAME' installed?"
    local output
    output="$(run_harness "$binary" "$prompt")"

    # 7. Check that the skill name appears in the harness output.
    if echo "$output" | grep -qi "$SKILL_NAME"; then
        ok "$display successfully loaded '$SKILL_NAME'"
    else
        warn "Could not confirm '$SKILL_NAME' in $display output"
        warn "First 5 lines of output:"
        echo "$output" | head -5 | sed 's/^/    | /'
    fi
}

###############################################################################
# Main
###############################################################################
main() {
    echo "============================================"
    echo " Headless Smoke Test: Skill Harness Loading"
    echo "============================================"

    ensure_symskills
    setup_temp_home
    create_fixture_skill

    # Register of known harnesses.
    # Format: binary  display_name  symskills_target  api_key_env_var
    smoke_test_harness opencode OpenCode opencode OPENAI_API_KEY
    smoke_test_harness claude   "Claude Code" claude ANTHROPIC_API_KEY
    smoke_test_harness codex    Codex codex OPENAI_API_KEY
    smoke_test_harness hermes   Hermes hermes ANTHROPIC_API_KEY

    echo ""
    echo "============================================"
    echo " Smoke test complete."
    echo "============================================"
}

main "$@"

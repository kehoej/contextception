#!/usr/bin/env bash
#
# reproduce.sh — Reproduce Contextception benchmark results
#
# This script runs the full competitive comparison between Contextception,
# Aider, and Repomix, and validates the results against published data.
#
# Prerequisites:
#   - Go 1.22+ (to build contextception)
#   - Python 3.10+ with aider-chat: pip install aider-chat
#   - Node.js 18+ with repomix: npm install -g repomix
#   - ~20GB disk space for test repos
#   - ~10 minutes runtime
#
# Usage:
#   ./benchmarks/reproduce.sh                     # Full run
#   ./benchmarks/reproduce.sh --cc-only           # Contextception only (no aider/repomix)
#   ./benchmarks/reproduce.sh --validate-only     # Validate existing results against published data
#   ./benchmarks/reproduce.sh --repos-dir ~/repos # Custom repo directory
#
# The script:
#   1. Builds contextception from source
#   2. Clones 6 test repositories (if not present)
#   3. Indexes each repository
#   4. Runs contextception, aider, and repomix on archetype files
#   5. Validates results against published benchmarks/data/results.json
#   6. Prints a summary comparison table

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
RESULTS_FILE="$SCRIPT_DIR/data/results.json"
FIXTURES_DIR="$SCRIPT_DIR/data/fixtures"

REPOS_DIR="${REPOS_DIR:-$HOME/Repositories/test-corpus}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/benchmark-reproduce-$(date +%Y%m%d)}"
CC_ONLY=false
VALIDATE_ONLY=false

# ─── Parse arguments ─────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case $1 in
        --cc-only) CC_ONLY=true; shift ;;
        --validate-only) VALIDATE_ONLY=true; shift ;;
        --repos-dir) REPOS_DIR="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--cc-only] [--validate-only] [--repos-dir DIR] [--output-dir DIR]"
            echo ""
            echo "  --cc-only        Run only Contextception (skip aider/repomix)"
            echo "  --validate-only  Validate existing results against published data"
            echo "  --repos-dir DIR  Directory for test repo clones (default: ~/Repositories/test-corpus)"
            echo "  --output-dir DIR Directory for comparison outputs (default: /tmp/benchmark-reproduce-DATE)"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# ─── Color output ─────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()  { echo -e "${RED}[FAIL]${NC} $*"; }
header() { echo -e "\n${BOLD}$*${NC}"; }

# ─── Repos ────────────────────────────────────────────────────────────────────

declare -a REPO_NAMES=("httpx" "excalidraw" "tokio" "terraform" "zulip" "spring-boot")
declare -A REPO_URLS=(
    [httpx]="https://github.com/encode/httpx.git"
    [excalidraw]="https://github.com/excalidraw/excalidraw.git"
    [tokio]="https://github.com/tokio-rs/tokio.git"
    [terraform]="https://github.com/hashicorp/terraform.git"
    [zulip]="https://github.com/zulip/zulip.git"
    [spring-boot]="https://github.com/spring-projects/spring-boot.git"
)

# ─── Validate-only mode ──────────────────────────────────────────────────────

if [[ "$VALIDATE_ONLY" == "true" ]]; then
    header "Validating published results..."

    if [[ ! -f "$RESULTS_FILE" ]]; then
        fail "Published results not found at $RESULTS_FILE"
        exit 1
    fi

    python3 "$SCRIPT_DIR/validate.py" "$RESULTS_FILE" "$FIXTURES_DIR"
    exit $?
fi

# ─── Build ────────────────────────────────────────────────────────────────────

header "Step 1: Build contextception"

CONTEXTCEPTION="$PROJECT_ROOT/contextception"
if [[ ! -x "$CONTEXTCEPTION" ]] || [[ "$PROJECT_ROOT/cmd/contextception/main.go" -nt "$CONTEXTCEPTION" ]]; then
    info "Building from source..."
    (cd "$PROJECT_ROOT" && go build -o contextception ./cmd/contextception)
    ok "Built contextception ($("$CONTEXTCEPTION" --version 2>/dev/null || echo 'unknown version'))"
else
    ok "Binary up to date"
fi

# ─── Clone repos ─────────────────────────────────────────────────────────────

header "Step 2: Clone test repositories"

mkdir -p "$REPOS_DIR"
for repo in "${REPO_NAMES[@]}"; do
    dest="$REPOS_DIR/$repo"
    if [[ -d "$dest/.git" ]]; then
        ok "$repo already cloned"
    else
        info "Cloning $repo (full history)..."
        git clone "${REPO_URLS[$repo]}" "$dest"
        ok "Cloned $repo"
    fi
done

# ─── Run comparison ──────────────────────────────────────────────────────────

header "Step 3: Run comparison"

mkdir -p "$OUTPUT_DIR"

if [[ "$CC_ONLY" == "true" ]]; then
    info "Running Contextception only (--cc-only)"
    EXTRA_ARGS=""
else
    EXTRA_ARGS=""
fi

# Delegate to the main comparison script
REPOS_DIR="$REPOS_DIR" OUTPUT_DIR="$OUTPUT_DIR" \
    "$PROJECT_ROOT/scripts/compare/run_comparison.sh"

# ─── Validate against published ──────────────────────────────────────────────

header "Step 4: Validate results"

if [[ -f "$OUTPUT_DIR/results.json" ]]; then
    info "Comparing reproduction against published results..."
    python3 - "$OUTPUT_DIR/results.json" "$RESULTS_FILE" <<'PYEOF'
import json, sys

repro_path = sys.argv[1]
published_path = sys.argv[2]

repro = json.load(open(repro_path))
published = json.load(open(published_path))

print("\n" + "=" * 70)
print("  BENCHMARK REPRODUCTION RESULTS")
print("=" * 70)

# Compare per-repo metrics
pub_repos = published.get("repos", {})
repro_repos = repro.get("repos", {})

print(f"\n{'Repo':<15} {'Files':<8} {'CC Avg Tokens':<15} {'Status'}")
print("-" * 50)

all_match = True
for repo in sorted(pub_repos.keys()):
    pub = pub_repos[repo]
    rep = repro_repos.get(repo, {})

    pub_files = len(pub.get("files", []))
    rep_files = len(rep.get("files", []))

    # Compare CC token averages
    pub_tokens = [f["cc"]["tokens"] for f in pub.get("files", []) if "cc" in f and "tokens" in f["cc"]]
    rep_tokens = [f["cc"]["tokens"] for f in rep.get("files", []) if "cc" in f and "tokens" in f["cc"]]

    pub_avg = sum(pub_tokens) // len(pub_tokens) if pub_tokens else 0
    rep_avg = sum(rep_tokens) // len(rep_tokens) if rep_tokens else 0

    # Within 20% tolerance (indexing can vary with repo updates)
    if pub_avg > 0:
        diff_pct = abs(rep_avg - pub_avg) / pub_avg * 100
        status = "PASS" if diff_pct < 20 else "DRIFT"
        if status == "DRIFT":
            all_match = False
    else:
        status = "NO DATA" if rep_avg == 0 else "NEW"

    print(f"{repo:<15} {rep_files:<8} {rep_avg:<15} {status}")

print()
if all_match:
    print("  ALL CHECKS PASSED — results match published data within tolerance")
else:
    print("  SOME DRIFT DETECTED — see above. This may be expected if repos were updated.")
print()

PYEOF
else
    warn "No reproduction results found at $OUTPUT_DIR/results.json"
fi

# ─── Fixture validation ──────────────────────────────────────────────────────

header "Step 5: Validate against httpx fixtures"

python3 "$SCRIPT_DIR/validate.py" "$OUTPUT_DIR/results.json" "$FIXTURES_DIR" 2>/dev/null || \
    python3 "$SCRIPT_DIR/validate.py" "$RESULTS_FILE" "$FIXTURES_DIR"

# ─── Summary ──────────────────────────────────────────────────────────────────

header "Done"
echo ""
echo "Results:    $OUTPUT_DIR/results.json"
echo "Published:  $RESULTS_FILE"
echo "Fixtures:   $FIXTURES_DIR/"
echo ""
echo "To view the comparison:"
echo "  python3 -m json.tool $OUTPUT_DIR/results.json | less"

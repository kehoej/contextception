#!/usr/bin/env bash
#
# run_comparison.sh — Run contextception, aider, and repomix on test repos
# and collect outputs for competitive comparison scoring.
#
# Uses `contextception archetypes` to auto-select 10 test files per repo.
#
# Prerequisites:
#   - contextception binary built: go build -o contextception ./cmd/contextception
#   - aider installed: pip install aider-chat
#   - repomix installed: npm install -g repomix
#   - Test repos cloned (with full history for git signals)
#
# Usage:
#   ./scripts/compare/run_comparison.sh [--repos-dir ~/Repositories/test-corpus] [--output-dir /tmp/compare-results]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONTEXTCEPTION="$PROJECT_ROOT/contextception"
EXTRACTOR="$SCRIPT_DIR/extract_aider_map.py"

REPOS_DIR="${REPOS_DIR:-$HOME/Repositories/test-corpus}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/compare-results}"

# Parse CLI args
while [[ $# -gt 0 ]]; do
    case $1 in
        --repos-dir) REPOS_DIR="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

mkdir -p "$OUTPUT_DIR"

# ─── Color output ─────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()  { echo -e "${RED}[FAIL]${NC} $*"; }

# ─── Repo definitions ───────────────────────────────────────────────────────
# Format: name|language|clone_url (empty url = already in REPOS_DIR)

declare -a REPOS=(
    "httpx|python|https://github.com/encode/httpx.git"
    "zulip|python|https://github.com/zulip/zulip.git"
    "excalidraw|typescript|https://github.com/excalidraw/excalidraw.git"
    "terraform|go|"
    "spring-boot|java|"
    "tokio|rust|"
    "efcore|csharp|https://github.com/dotnet/efcore.git"
)

# ─── Preflight checks ────────────────────────────────────────────────────────

info "Preflight checks..."

if [[ ! -x "$CONTEXTCEPTION" ]]; then
    warn "contextception binary not found at $CONTEXTCEPTION"
    info "Building from source..."
    (cd "$PROJECT_ROOT" && go build -o contextception ./cmd/contextception)
    ok "Built contextception"
fi

if ! command -v aider &>/dev/null && ! python3 -c "import aider" &>/dev/null; then
    fail "aider not found. Install with: pip install aider-chat"
    HAVE_AIDER=false
else
    ok "aider available"
    HAVE_AIDER=true
fi

if ! command -v repomix &>/dev/null; then
    fail "repomix not found. Install with: npm install -g repomix"
    HAVE_REPOMIX=false
else
    ok "repomix available"
    HAVE_REPOMIX=true
fi

# ─── Clone repos ─────────────────────────────────────────────────────────────

clone_repo() {
    local name="$1"
    local url="$2"
    local dest="$REPOS_DIR/$name"
    if [[ -d "$dest/.git" ]]; then
        ok "$name already cloned at $dest"
    else
        info "Cloning $name (full history)..."
        git clone "$url" "$dest"
        ok "Cloned $name"
    fi
}

info "Ensuring test repos are cloned..."
for entry in "${REPOS[@]}"; do
    IFS='|' read -r name lang url <<< "$entry"
    if [[ -n "$url" ]]; then
        clone_repo "$name" "$url"
    else
        if [[ -d "$REPOS_DIR/$name/.git" ]]; then
            ok "$name already present at $REPOS_DIR/$name"
        else
            fail "$name not found at $REPOS_DIR/$name (no clone URL configured)"
        fi
    fi
done

# ─── Index & detect archetypes ───────────────────────────────────────────────

index_repo() {
    local repo="$1"
    local repo_dir="$REPOS_DIR/$repo"
    if [[ ! -f "$repo_dir/.contextception/index.sqlite" ]]; then
        info "Indexing $repo..."
        (cd "$repo_dir" && "$CONTEXTCEPTION" index 2>&1) | tail -5
    else
        ok "$repo already indexed"
    fi
}

detect_archetypes() {
    local repo="$1"
    local repo_dir="$REPOS_DIR/$repo"
    local archfile="$OUTPUT_DIR/archetypes_${repo}.json"

    info "Detecting archetypes for $repo..."
    (cd "$repo_dir" && "$CONTEXTCEPTION" archetypes 2>/dev/null) > "$archfile"

    local count
    count=$(python3 -c "import json; d=json.load(open('$archfile')); print(len(d) if d else 0)" 2>/dev/null || echo "0")
    ok "  $count archetypes detected → $archfile"
}

# ─── Run contextception ──────────────────────────────────────────────────────

run_contextception() {
    local repo="$1"
    local file="$2"
    local repo_dir="$REPOS_DIR/$repo"
    local outfile="$OUTPUT_DIR/cc_${repo}_$(echo "$file" | tr '/' '_' | tr '.' '_').json"

    info "Contextception: $repo / $file"

    # Analyze
    (cd "$repo_dir" && "$CONTEXTCEPTION" analyze --json "$file" 2>/dev/null) > "$outfile"

    local must_read_count
    must_read_count=$(python3 -c "import json; d=json.load(open('$outfile')); print(len(d.get('must_read', [])))" 2>/dev/null || echo "?")
    local output_size
    output_size=$(wc -c < "$outfile" | tr -d ' ')
    ok "  must_read=$must_read_count, output=${output_size}B → $outfile"
}

# ─── Run aider ────────────────────────────────────────────────────────────────

run_aider() {
    local repo="$1"
    local file="$2"
    local repo_dir="$REPOS_DIR/$repo"

    if [[ "$HAVE_AIDER" != "true" ]]; then
        warn "Skipping aider for $repo/$file (not installed)"
        return
    fi

    info "Aider: $repo / $file"

    python3 "$EXTRACTOR" "$repo_dir" "$file" \
        --budgets "4096,8192" \
        --output-dir "$OUTPUT_DIR" \
        2>&1 | while IFS= read -r line; do echo "  $line"; done
}

# ─── Run repomix ──────────────────────────────────────────────────────────────

run_repomix() {
    local repo="$1"
    local repo_dir="$REPOS_DIR/$repo"
    local outfile="$OUTPUT_DIR/repomix_${repo}.xml"

    if [[ "$HAVE_REPOMIX" != "true" ]]; then
        warn "Skipping repomix for $repo (not installed)"
        return
    fi

    # Only run once per repo (repomix dumps everything)
    if [[ -f "$outfile" ]]; then
        ok "Repomix: $repo already generated"
        return
    fi

    info "Repomix: $repo (full repo dump)..."
    if ! (cd "$repo_dir" && repomix --output "$outfile" --style xml . 2>&1) | tail -5; then
        warn "Repomix failed for $repo (may be too large)"
        return
    fi
    local size
    size=$(wc -c < "$outfile" | tr -d ' ')
    local token_est=$((size / 4))
    ok "  Output: ${size}B (~${token_est} tokens) → $outfile"
}

# ─── Main execution ──────────────────────────────────────────────────────────

REPO_COUNT=${#REPOS[@]}
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  Competitive Comparison: contextception vs aider vs repomix ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  Repos: $REPO_COUNT (10 archetype files each)                        ║"
echo "║  Tools: contextception, aider (4k/8k), repomix              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Phase 1: Index all repos
info "═══ Phase 1: Indexing ═══"
for entry in "${REPOS[@]}"; do
    IFS='|' read -r name lang url <<< "$entry"
    index_repo "$name"
done

# Phase 2: Detect archetypes for each repo
info ""
info "═══ Phase 2: Archetype Detection ═══"
declare -A ARCHETYPE_FILES
for entry in "${REPOS[@]}"; do
    IFS='|' read -r name lang url <<< "$entry"
    detect_archetypes "$name"
    ARCHETYPE_FILES[$name]="$OUTPUT_DIR/archetypes_${name}.json"
done

# Phase 3: Run repomix (once per repo, slow)
echo ""
info "═══ Phase 3: Repomix (baseline token count) ═══"
for entry in "${REPOS[@]}"; do
    IFS='|' read -r name lang url <<< "$entry"
    run_repomix "$name"
done

# Phase 4: Per-file analysis
echo ""
info "═══ Phase 4: Per-file analysis ═══"

for entry in "${REPOS[@]}"; do
    IFS='|' read -r name lang url <<< "$entry"
    archfile="${ARCHETYPE_FILES[$name]}"

    # Extract file list from archetypes JSON
    files=$(python3 -c "
import json, sys
data = json.load(open('$archfile'))
if data:
    for item in data:
        print(item['file'])
" 2>/dev/null)

    if [[ -z "$files" ]]; then
        warn "No archetypes detected for $name, skipping"
        continue
    fi

    echo ""
    info "────── $name ($lang) ──────"

    while IFS= read -r file; do
        [[ -z "$file" ]] && continue
        echo ""
        info "  → $file"
        run_contextception "$name" "$file"
        run_aider "$name" "$file"
    done <<< "$files"
done

# ─── Phase 5: Aggregate results ──────────────────────────────────────────────

echo ""
info "═══ Phase 5: Aggregating results ═══"

python3 - "$OUTPUT_DIR" "$REPOS_DIR" <<'PYEOF'
import json, os, sys, glob
from pathlib import Path

output_dir = sys.argv[1]
repos_dir = sys.argv[2]

results = {"repos": {}}

# Collect per-repo data
for archfile in sorted(glob.glob(os.path.join(output_dir, "archetypes_*.json"))):
    repo = Path(archfile).stem.replace("archetypes_", "")
    archetypes = json.load(open(archfile)) or []

    repo_data = {
        "archetype_count": len(archetypes),
        "files": [],
        "repomix_tokens": None,
    }

    # Get repomix token count
    repomix_file = os.path.join(output_dir, f"repomix_{repo}.xml")
    if os.path.exists(repomix_file):
        size = os.path.getsize(repomix_file)
        repo_data["repomix_tokens"] = size // 4

    for arch in archetypes:
        file_path = arch["file"]
        file_stem = file_path.replace("/", "_").replace(".", "_")

        file_data = {
            "file": file_path,
            "archetype": arch["archetype"],
            "indegree": arch.get("indegree", 0),
            "outdegree": arch.get("outdegree", 0),
        }

        # CC output
        cc_file = os.path.join(output_dir, f"cc_{repo}_{file_stem}.json")
        if os.path.exists(cc_file):
            try:
                cc = json.load(open(cc_file))
                cc_tokens = os.path.getsize(cc_file) // 4
                file_data["cc"] = {
                    "must_read_count": len(cc.get("must_read", [])),
                    "likely_modify_count": sum(len(v) for v in cc.get("likely_modify", {}).values()),
                    "test_count": len(cc.get("tests", [])),
                    "confidence": cc.get("confidence", 0),
                    "tokens": cc_tokens,
                    "total_files": len(cc.get("must_read", [])) + sum(len(v) for v in cc.get("likely_modify", {}).values()) + len(cc.get("tests", [])),
                }
            except Exception as e:
                file_data["cc"] = {"error": str(e)}

        # Aider outputs (4096 and 8192)
        for budget in [4096, 8192]:
            aider_file = os.path.join(output_dir, f"aider_{repo}_{file_stem}_results.json")
            if os.path.exists(aider_file):
                try:
                    aider = json.load(open(aider_file))
                    budget_data = aider.get("results", {}).get(str(budget), {})
                    if budget_data and "error" not in budget_data:
                        # Compute recall against CC must_read as ground truth
                        cc_file_path = os.path.join(output_dir, f"cc_{repo}_{file_stem}.json")
                        recall = None
                        precision = None
                        if os.path.exists(cc_file_path):
                            try:
                                cc = json.load(open(cc_file_path))
                                gt_files = set(e["file"] for e in cc.get("must_read", []))
                                aider_files = set(budget_data.get("files", []))
                                if gt_files:
                                    found = gt_files & aider_files
                                    recall = round(len(found) / len(gt_files), 3) if gt_files else 0
                                    precision = round(len(found) / len(aider_files), 3) if aider_files else 0
                            except:
                                pass

                        file_data[f"aider_{budget}"] = {
                            "file_count": budget_data.get("file_count", 0),
                            "tokens": budget_data.get("raw_length", 0) // 4,
                            "recall": recall,
                            "precision": precision,
                        }
                except Exception as e:
                    file_data[f"aider_{budget}"] = {"error": str(e)}

        repo_data["files"].append(file_data)

    results["repos"][repo] = repo_data

# Compute aggregates
total_cc_files = 0
total_cc_tokens = 0
total_aider_4k_recall = []
total_aider_8k_recall = []
total_aider_4k_precision = []
total_aider_8k_precision = []
file_count = 0

for repo, rdata in results["repos"].items():
    for f in rdata["files"]:
        file_count += 1
        if "cc" in f and "tokens" in f["cc"]:
            total_cc_files += f["cc"].get("total_files", 0)
            total_cc_tokens += f["cc"]["tokens"]
        if "aider_4096" in f and f["aider_4096"].get("recall") is not None:
            total_aider_4k_recall.append(f["aider_4096"]["recall"])
            total_aider_4k_precision.append(f["aider_4096"]["precision"])
        if "aider_8192" in f and f["aider_8192"].get("recall") is not None:
            total_aider_8k_recall.append(f["aider_8192"]["recall"])
            total_aider_8k_precision.append(f["aider_8192"]["precision"])

results["aggregate"] = {
    "total_files_analyzed": file_count,
    "total_repos": len(results["repos"]),
    "avg_cc_tokens": round(total_cc_tokens / file_count) if file_count else 0,
    "avg_aider_4k_recall": round(sum(total_aider_4k_recall) / len(total_aider_4k_recall), 3) if total_aider_4k_recall else None,
    "avg_aider_8k_recall": round(sum(total_aider_8k_recall) / len(total_aider_8k_recall), 3) if total_aider_8k_recall else None,
    "avg_aider_4k_precision": round(sum(total_aider_4k_precision) / len(total_aider_4k_precision), 3) if total_aider_4k_precision else None,
    "avg_aider_8k_precision": round(sum(total_aider_8k_precision) / len(total_aider_8k_precision), 3) if total_aider_8k_precision else None,
}

out_path = os.path.join(output_dir, "results.json")
with open(out_path, "w") as f:
    json.dump(results, f, indent=2)
print(f"Aggregated results → {out_path}")
print(f"  Repos: {len(results['repos'])}")
print(f"  Files analyzed: {file_count}")
PYEOF

# ─── Summary ──────────────────────────────────────────────────────────────────

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  Comparison complete                                        ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  Results directory: $OUTPUT_DIR"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

info "Files generated:"
ls -la "$OUTPUT_DIR"/ 2>/dev/null | tail -30

echo ""
info "Key output: $OUTPUT_DIR/results.json"

#!/usr/bin/env bash
# Parse Contextception JSON output using jq.
#
# Usage:
#   # Pipe analysis output
#   contextception analyze src/server.ts | bash parse_output.sh
#
#   # Pipe change report
#   contextception analyze-change main..HEAD | bash parse_output.sh
#
#   # From a file
#   bash parse_output.sh < output.json
#
#   # Extract specific fields
#   bash parse_output.sh must-read < output.json
#   bash parse_output.sh blast-radius < output.json
#   bash parse_output.sh tests < output.json
#   bash parse_output.sh summary < output.json       # (change report only)
#   bash parse_output.sh test-gaps < output.json      # (change report only)
#
# Requires: jq (https://jqlang.github.io/jq/)

set -euo pipefail

if ! command -v jq &>/dev/null; then
    echo "Error: jq is required. Install with: brew install jq" >&2
    exit 1
fi

# Read all input into a variable so we can process it multiple times
INPUT=$(cat)

# Detect schema type
SCHEMA_TYPE=$(echo "$INPUT" | jq -r '
  if has("ref_range") then "change"
  elif has("subject") then "analysis"
  else "unknown"
  end
')

# Sub-command routing
CMD="${1:-overview}"

case "$CMD" in
  overview)
    if [ "$SCHEMA_TYPE" = "analysis" ]; then
      echo "$INPUT" | jq '{
        subject,
        schema_version,
        confidence,
        must_read_count: (.must_read | length),
        likely_modify_count: ([.likely_modify[]? | length] | add // 0),
        test_count: (.tests | length),
        direct_tests: ([.tests[] | select(.direct)] | length),
        blast_radius: .blast_radius.level,
        hotspot_count: (.hotspots // [] | length),
        circular_dep_count: (.circular_deps // [] | length),
        external_count: (.external | length)
      }'
    elif [ "$SCHEMA_TYPE" = "change" ]; then
      echo "$INPUT" | jq '{
        ref_range,
        schema_version,
        total_changed: .summary.total_files,
        added: .summary.added,
        modified: .summary.modified,
        deleted: .summary.deleted,
        high_risk_files: .summary.high_risk_files,
        blast_radius: .blast_radius.level,
        must_read_count: (.must_read | length),
        likely_modify_count: ([.likely_modify[]? | length] | add // 0),
        test_count: (.tests | length),
        test_gap_count: (.test_gaps // [] | length),
        coupling_count: (.coupling // [] | length),
        hidden_coupling_count: (.hidden_coupling // [] | length)
      }'
    else
      echo "Error: Could not detect schema type" >&2
      exit 1
    fi
    ;;

  must-read)
    echo "$INPUT" | jq -r '.must_read[] | [
      .file,
      (.direction // "-"),
      (.role // "-"),
      (if .stable then "stable" else "" end),
      (if .circular then "circular" else "" end)
    ] | join("\t")' | column -t -s$'\t'
    ;;

  blast-radius)
    if [ "$SCHEMA_TYPE" = "analysis" ]; then
      echo "$INPUT" | jq '.blast_radius // "none"'
    else
      echo "=== Aggregate ==="
      echo "$INPUT" | jq '.blast_radius'
      echo ""
      echo "=== Per File ==="
      echo "$INPUT" | jq -r '.changed_files[] | select(.blast_radius) | [
        .file, .blast_radius.level, .blast_radius.detail
      ] | join("\t")' | column -t -s$'\t'
    fi
    ;;

  tests)
    echo "=== Direct ==="
    echo "$INPUT" | jq -r '[.tests[] | select(.direct)] | .[].file'
    echo ""
    echo "=== Indirect ==="
    echo "$INPUT" | jq -r '[.tests[] | select(.direct | not)] | .[].file'
    ;;

  likely-modify)
    for tier in high medium low; do
      COUNT=$(echo "$INPUT" | jq -r ".likely_modify.\"$tier\" // [] | length")
      if [ "$COUNT" -gt 0 ]; then
        echo "=== $tier ==="
        echo "$INPUT" | jq -r ".likely_modify.\"$tier\"[] | .file"
        echo ""
      fi
    done
    ;;

  related)
    echo "$INPUT" | jq -r '.related | to_entries[] | .key as $type | .value[] | [$type, .file, (.signals | join(", "))] | join("\t")' | column -t -s$'\t'
    ;;

  summary)
    if [ "$SCHEMA_TYPE" != "change" ]; then
      echo "Error: 'summary' sub-command only works with change reports" >&2
      exit 1
    fi
    echo "$INPUT" | jq '.summary'
    ;;

  test-gaps)
    if [ "$SCHEMA_TYPE" != "change" ]; then
      echo "Error: 'test-gaps' sub-command only works with change reports" >&2
      exit 1
    fi
    echo "$INPUT" | jq -r '.test_gaps // [] | .[]'
    ;;

  coupling)
    if [ "$SCHEMA_TYPE" != "change" ]; then
      echo "Error: 'coupling' sub-command only works with change reports" >&2
      exit 1
    fi
    echo "$INPUT" | jq -r '(.coupling // [])[] | [.file_a, .direction, .file_b] | join("\t")' | column -t -s$'\t'
    ;;

  files)
    # List all unique files mentioned anywhere in the output
    echo "$INPUT" | jq -r '[
      .must_read[]?.file,
      (.likely_modify // {} | [.[]?[]?.file] | .[]),
      .tests[]?.file,
      (.related // {} | [.[]?[]?.file] | .[]),
      (.changed_files // [])[]?.file
    ] | unique | .[]'
    ;;

  json)
    # Pretty-print the raw JSON
    echo "$INPUT" | jq .
    ;;

  *)
    echo "Unknown sub-command: $CMD" >&2
    echo "Available: overview, must-read, blast-radius, tests, likely-modify, related, summary, test-gaps, coupling, files, json" >&2
    exit 1
    ;;
esac

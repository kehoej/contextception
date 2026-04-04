#!/usr/bin/env python3
"""Validate and parse Contextception JSON output against JSON Schema.

Usage:
    # Validate analysis output
    contextception analyze src/server.ts | python3 parse_output.py

    # Validate change report
    contextception analyze-change main..HEAD | python3 parse_output.py --type change

    # Validate from a file
    python3 parse_output.py --file output.json

    # Validate and print summary
    python3 parse_output.py --file output.json --summary

Requirements:
    pip install jsonschema
"""

import argparse
import json
import sys
from pathlib import Path

PROTOCOL_DIR = Path(__file__).resolve().parent.parent
ANALYSIS_SCHEMA_PATH = PROTOCOL_DIR / "analysis-schema.json"
CHANGE_SCHEMA_PATH = PROTOCOL_DIR / "change-schema.json"


def load_schema(schema_type: str) -> dict:
    """Load the appropriate JSON Schema file."""
    if schema_type == "analysis":
        path = ANALYSIS_SCHEMA_PATH
    elif schema_type == "change":
        path = CHANGE_SCHEMA_PATH
    else:
        raise ValueError(f"Unknown schema type: {schema_type}")

    if not path.exists():
        print(f"Error: Schema file not found: {path}", file=sys.stderr)
        print("Run 'go run ./cmd/gen-schema' from the repo root to generate.", file=sys.stderr)
        sys.exit(1)

    with open(path) as f:
        return json.load(f)


def validate(data: dict, schema: dict) -> list[str]:
    """Validate data against a JSON Schema. Returns list of error messages."""
    try:
        import jsonschema
    except ImportError:
        print(
            "Error: jsonschema package required. Install with: pip install jsonschema",
            file=sys.stderr,
        )
        sys.exit(1)

    validator = jsonschema.Draft202012Validator(schema)
    errors = []
    for error in sorted(validator.iter_errors(data), key=lambda e: list(e.path)):
        path = ".".join(str(p) for p in error.absolute_path) or "(root)"
        errors.append(f"  {path}: {error.message}")
    return errors


def detect_type(data: dict) -> str:
    """Auto-detect whether the output is an analysis or change report."""
    if "ref_range" in data:
        return "change"
    if "subject" in data:
        return "analysis"
    # Fall back to schema_version heuristic
    version = data.get("schema_version", "")
    if version.startswith("3"):
        return "analysis"
    if version.startswith("1"):
        return "change"
    return "analysis"


def print_analysis_summary(data: dict) -> None:
    """Print a human-readable summary of an AnalysisOutput."""
    print(f"Subject:      {data['subject']}")
    print(f"Schema:       v{data['schema_version']}")
    print(f"Confidence:   {data['confidence']:.2f}")

    if data.get("confidence_note"):
        print(f"              {data['confidence_note']}")

    print(f"Must-read:    {len(data['must_read'])} files")
    for entry in data["must_read"]:
        flags = []
        if entry.get("direction"):
            flags.append(entry["direction"])
        if entry.get("role"):
            flags.append(entry["role"])
        if entry.get("stable"):
            flags.append("stable")
        if entry.get("circular"):
            flags.append("circular")
        suffix = f" ({', '.join(flags)})" if flags else ""
        print(f"              - {entry['file']}{suffix}")

    # Likely modify
    total_lm = sum(len(v) for v in data["likely_modify"].values())
    print(f"Likely modify: {total_lm} files")
    for tier in ("high", "medium", "low"):
        entries = data["likely_modify"].get(tier, [])
        for entry in entries:
            print(f"              - [{tier}] {entry['file']}")

    # Tests
    direct = [t for t in data["tests"] if t["direct"]]
    indirect = [t for t in data["tests"] if not t["direct"]]
    print(f"Tests:        {len(direct)} direct, {len(indirect)} indirect")

    # Blast radius
    br = data.get("blast_radius")
    if br:
        frag = f", fragility={br['fragility']:.2f}" if br.get("fragility") else ""
        print(f"Blast radius: {br['level']} - {br['detail']}{frag}")

    # Hotspots
    hotspots = data.get("hotspots", [])
    if hotspots:
        print(f"Hotspots:     {len(hotspots)}")
        for h in hotspots:
            print(f"              - {h}")

    # Circular deps
    cycles = data.get("circular_deps", [])
    if cycles:
        print(f"Circular:     {len(cycles)} cycle(s)")

    # External
    print(f"External:     {len(data['external'])} imports")

    # Stats
    stats = data.get("stats")
    if stats:
        print(
            f"Index:        {stats['total_files']} files, "
            f"{stats['total_edges']} edges, "
            f"{stats['unresolved_count']} unresolved"
        )


def print_change_summary(data: dict) -> None:
    """Print a human-readable summary of a ChangeReport."""
    print(f"Ref range:    {data['ref_range']}")
    print(f"Schema:       v{data['schema_version']}")

    s = data["summary"]
    print(
        f"Changed:      {s['total_files']} files "
        f"(+{s['added']} ~{s['modified']} -{s['deleted']} R{s['renamed']})"
    )
    print(f"Indexed:      {s['indexed_files']}/{s['total_files']}")
    print(f"Test files:   {s['test_files']}")
    print(f"High risk:    {s['high_risk_files']}")

    br = data["blast_radius"]
    frag = f", fragility={br['fragility']:.2f}" if br.get("fragility") else ""
    print(f"Blast radius: {br['level']} - {br['detail']}{frag}")

    print(f"Must-read:    {len(data['must_read'])} files")
    total_lm = sum(len(v) for v in data["likely_modify"].values())
    print(f"Likely modify: {total_lm} files")

    direct = [t for t in data["tests"] if t["direct"]]
    indirect = [t for t in data["tests"] if not t["direct"]]
    print(f"Tests:        {len(direct)} direct, {len(indirect)} indirect")

    gaps = data.get("test_gaps", [])
    if gaps:
        print(f"Test gaps:    {len(gaps)}")
        for g in gaps:
            print(f"              - {g}")

    coupling = data.get("coupling", [])
    if coupling:
        print(f"Coupling:     {len(coupling)} pair(s)")

    hidden = data.get("hidden_coupling", [])
    if hidden:
        print(f"Hidden:       {len(hidden)} co-change partner(s)")


def main():
    parser = argparse.ArgumentParser(
        description="Validate Contextception JSON output against its JSON Schema."
    )
    parser.add_argument(
        "--type",
        choices=["analysis", "change", "auto"],
        default="auto",
        help="Schema type to validate against (default: auto-detect)",
    )
    parser.add_argument(
        "--file",
        type=str,
        help="Read JSON from file instead of stdin",
    )
    parser.add_argument(
        "--summary",
        action="store_true",
        help="Print a human-readable summary after validation",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Only output errors (exit code 0 = valid, 1 = invalid)",
    )
    args = parser.parse_args()

    # Read input
    if args.file:
        try:
            with open(args.file) as f:
                raw = f.read()
        except FileNotFoundError:
            print(f"Error: File not found: {args.file}", file=sys.stderr)
            sys.exit(1)
    else:
        if sys.stdin.isatty():
            print("Reading from stdin (pipe JSON or use --file)...", file=sys.stderr)
        raw = sys.stdin.read()

    # Parse JSON
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON: {e}", file=sys.stderr)
        sys.exit(1)

    # Detect or use specified type
    schema_type = args.type if args.type != "auto" else detect_type(data)

    # Load schema and validate
    schema = load_schema(schema_type)
    errors = validate(data, schema)

    if errors:
        print(f"INVALID ({schema_type} schema v{data.get('schema_version', '?')})")
        for err in errors:
            print(err)
        sys.exit(1)
    else:
        if not args.quiet:
            print(f"VALID ({schema_type} schema v{data.get('schema_version', '?')})")

    # Print summary if requested
    if args.summary:
        print()
        if schema_type == "analysis":
            print_analysis_summary(data)
        elif schema_type == "change":
            print_change_summary(data)

    sys.exit(0)


if __name__ == "__main__":
    main()

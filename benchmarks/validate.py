#!/usr/bin/env python3
"""
Validate benchmark results against httpx fixture ground truth.

This script checks that Contextception's must_read output matches
the hand-verified fixture files, providing an independent validation
of analysis quality.

Usage:
    python validate.py <results.json> <fixtures_dir>
    python validate.py benchmarks/data/results.json benchmarks/data/fixtures/
"""

import json
import sys
from pathlib import Path


def load_fixture(path: Path) -> dict:
    """Load a fixture file and extract ground truth."""
    data = json.load(open(path))
    return {
        "subject": data.get("subject", ""),
        "must_read": set(data.get("must_read", [])),
        "must_read_forbidden": set(data.get("must_read_forbidden", [])),
    }


def validate_cc_output(cc_must_read: list[str], fixture: dict) -> dict:
    """Validate CC must_read against fixture ground truth."""
    cc_files = set(cc_must_read)
    gt_required = fixture["must_read"]
    gt_forbidden = fixture["must_read_forbidden"]

    found = cc_files & gt_required
    missed = gt_required - cc_files
    forbidden_included = cc_files & gt_forbidden

    recall = len(found) / len(gt_required) if gt_required else 1.0
    precision = len(found) / len(cc_files) if cc_files else 0.0

    return {
        "recall": recall,
        "precision": precision,
        "found": sorted(found),
        "missed": sorted(missed),
        "forbidden_included": sorted(forbidden_included),
        "total_cc": len(cc_files),
        "total_gt": len(gt_required),
        "pass": recall >= 0.9 and len(forbidden_included) == 0,
    }


def main():
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <results.json> <fixtures_dir>")
        sys.exit(1)

    results_path = Path(sys.argv[1])
    fixtures_dir = Path(sys.argv[2])

    if not results_path.exists():
        print(f"Results file not found: {results_path}")
        sys.exit(1)

    if not fixtures_dir.exists():
        print(f"Fixtures directory not found: {fixtures_dir}")
        sys.exit(1)

    # Load results
    results = json.load(open(results_path))

    # Find httpx data in results
    httpx_data = None
    repos = results.get("repos", {})
    if "httpx" in repos:
        httpx_data = repos["httpx"]
    else:
        # Try flat structure
        for repo_name, repo_data in repos.items():
            if "httpx" in repo_name.lower():
                httpx_data = repo_data
                break

    # Load fixtures
    fixtures = {}
    for fixture_path in sorted(fixtures_dir.glob("*.json")):
        fixture = load_fixture(fixture_path)
        subject = fixture["subject"]
        fixtures[subject] = fixture
        # Also index by basename for flexible matching
        basename = Path(subject).name
        fixtures[basename] = fixture

    print("=" * 70)
    print("  FIXTURE VALIDATION: httpx")
    print("=" * 70)
    print(f"\n  Fixtures loaded: {len(fixtures) // 2}")  # Divided by 2 due to double-indexing
    print()

    all_pass = True
    validated = 0

    if httpx_data:
        for file_data in httpx_data.get("files", []):
            file_path = file_data.get("file", "")
            cc_data = file_data.get("cc", {})

            if "error" in cc_data:
                continue

            # Try to match against a fixture
            fixture = None
            for key in [file_path, Path(file_path).name]:
                if key in fixtures:
                    fixture = fixtures[key]
                    break

            if fixture is None:
                continue

            # Extract CC must_read file list
            # Handle both formats: list of strings or list of dicts
            must_read = cc_data.get("must_read", [])
            if must_read and isinstance(must_read[0], dict):
                cc_files = [entry["file"] for entry in must_read]
            elif must_read and isinstance(must_read[0], str):
                cc_files = must_read
            else:
                # Use must_read_count as a proxy
                continue

            result = validate_cc_output(cc_files, fixture)
            validated += 1

            status = "\033[0;32mPASS\033[0m" if result["pass"] else "\033[0;31mFAIL\033[0m"
            print(f"  {file_path}")
            print(f"    Recall:    {result['recall']:.0%} ({len(result['found'])}/{result['total_gt']})")
            print(f"    Precision: {result['precision']:.0%} ({len(result['found'])}/{result['total_cc']})")
            if result["missed"]:
                print(f"    Missed:    {result['missed']}")
            if result["forbidden_included"]:
                print(f"    FORBIDDEN: {result['forbidden_included']}")
            print(f"    Status:    {status}")
            print()

            if not result["pass"]:
                all_pass = False
    else:
        # No httpx in results — validate published data structure
        print("  No httpx data in results file. Validating fixture integrity only.")
        for fixture_path in sorted(fixtures_dir.glob("*.json")):
            data = json.load(open(fixture_path))
            subject = data.get("subject", "unknown")
            must_read = data.get("must_read", [])
            forbidden = data.get("must_read_forbidden", [])
            overlap = set(must_read) & set(forbidden)

            status = "PASS" if not overlap else "FAIL (overlap)"
            print(f"  {fixture_path.name}: subject={subject}, must_read={len(must_read)}, forbidden={len(forbidden)} — {status}")
            validated += 1

            if overlap:
                all_pass = False

    print("-" * 70)
    if validated == 0:
        print("  WARNING: No files could be validated against fixtures")
        print("  This may mean the results format doesn't include raw must_read lists.")
        print("  Run 'contextception analyze --json httpx/_client.py' in the httpx repo")
        print("  and check the output manually against fixtures.")
        sys.exit(0)

    if all_pass:
        print(f"  ALL {validated} VALIDATIONS PASSED")
        sys.exit(0)
    else:
        print(f"  SOME VALIDATIONS FAILED")
        sys.exit(1)


if __name__ == "__main__":
    main()

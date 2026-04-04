#!/usr/bin/env python3
"""
Extract Aider repo-map file lists for competitive comparison.

Uses aider's internal RepoMap class to programmatically generate
repo maps at different token budgets, then extracts the file list.

Usage:
    python extract_aider_map.py <repo_root> <subject_file> [--budget 2048,4096,8192] [--output-dir /tmp/compare-results]
"""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path


def extract_files_from_map(map_text: str) -> list[str]:
    """Extract file paths from aider's repo-map text output.

    Aider's map format lists files as lines ending with ':' followed
    by indented definitions. E.g.:
        httpx/_client.py:
            class Client:
                def get(...)
    """
    files = []
    for line in map_text.splitlines():
        line = line.rstrip()
        # File lines are not indented and end with ':'
        if line and not line.startswith(" ") and not line.startswith("\t") and line.endswith(":"):
            filepath = line[:-1].strip()
            # Skip lines that look like class/function definitions
            if not filepath.startswith(("class ", "def ", "function ", "const ", "export ")):
                files.append(filepath)
    return files


def run_aider_cli(repo_root: str, budget: int) -> str | None:
    """Run aider --show-repo-map via CLI and capture output."""
    cmd = [
        sys.executable, "-m", "aider",
        "--show-repo-map",
        "--map-tokens", str(budget),
        "--no-git",
        "--yes",
    ]
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=300,
            cwd=repo_root,
        )
        return result.stdout
    except (subprocess.TimeoutExpired, FileNotFoundError) as e:
        print(f"  Error running aider CLI: {e}", file=sys.stderr)
        return None


def run_aider_programmatic(repo_root: str, budget: int) -> str | None:
    """Use aider's internal RepoMap class directly."""
    try:
        from aider.io import InputOutput
        from aider.models import Model
        from aider.repomap import RepoMap
    except ImportError:
        print("  aider not importable, falling back to CLI", file=sys.stderr)
        return None

    try:
        io = InputOutput(yes=True)
        # Model object needed for token counting (local tiktoken, no API key required)
        model = Model("gpt-4o")
        rm = RepoMap(
            map_tokens=budget,
            root=repo_root,
            main_model=model,
            io=io,
            verbose=False,
        )

        # Collect all tracked files
        all_files = []
        for dirpath, _dirnames, filenames in os.walk(repo_root):
            # Skip hidden dirs and common non-source dirs
            rel_dir = os.path.relpath(dirpath, repo_root)
            if any(part.startswith(".") for part in rel_dir.split(os.sep)):
                continue
            if any(skip in rel_dir.split(os.sep) for skip in ["node_modules", "__pycache__", ".git", "venv", "dist", "build"]):
                continue
            for fname in filenames:
                full = os.path.join(dirpath, fname)
                all_files.append(full)

        repo_map = rm.get_repo_map(
            chat_files=[],
            other_files=all_files,
            force_refresh=True,
        )
        return repo_map
    except Exception as e:
        print(f"  Programmatic extraction failed: {e}", file=sys.stderr)
        return None


def extract_repo_map(repo_root: str, budget: int) -> tuple[str | None, list[str]]:
    """Extract repo map, trying programmatic API first, then CLI."""
    # Try programmatic first
    raw = run_aider_programmatic(repo_root, budget)
    if raw is None:
        raw = run_aider_cli(repo_root, budget)
    if raw is None:
        return None, []
    files = extract_files_from_map(raw)
    return raw, files


def compute_overlap(aider_files: list[str], ground_truth: list[str], repo_root: str) -> dict:
    """Compute recall and precision against ground truth must_read list."""
    # Normalize paths for comparison
    aider_set = set(aider_files)
    gt_set = set(ground_truth)

    found = aider_set & gt_set
    missed = gt_set - aider_set
    extra = aider_set - gt_set

    recall = len(found) / len(gt_set) if gt_set else 0
    precision = len(found) / len(aider_set) if aider_set else 0

    return {
        "recall": round(recall, 3),
        "precision": round(precision, 3),
        "found": sorted(found),
        "missed": sorted(missed),
        "extra_count": len(extra),
        "aider_total": len(aider_set),
        "ground_truth_total": len(gt_set),
    }


def main():
    parser = argparse.ArgumentParser(description="Extract Aider repo-map for competitive comparison")
    parser.add_argument("repo_root", help="Path to the repository root")
    parser.add_argument("subject_file", help="Subject file (repo-relative path, for ground truth matching)")
    parser.add_argument("--budgets", default="2048,4096,8192", help="Comma-separated token budgets")
    parser.add_argument("--output-dir", default="/tmp/compare-results", help="Output directory")
    parser.add_argument("--ground-truth", help="Path to ground truth JSON fixture file")
    args = parser.parse_args()

    budgets = [int(b) for b in args.budgets.split(",")]
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Load ground truth if provided
    ground_truth = []
    if args.ground_truth and os.path.exists(args.ground_truth):
        with open(args.ground_truth) as f:
            gt_data = json.load(f)
        ground_truth = gt_data.get("must_read", [])
        print(f"Ground truth: {len(ground_truth)} must_read files from {args.ground_truth}")

    repo_name = Path(args.repo_root).name
    subject_stem = args.subject_file.replace("/", "_").replace(".", "_")

    results = {}
    for budget in budgets:
        print(f"\n--- Aider repo-map @ {budget} tokens for {repo_name} ---")
        raw_map, files = extract_repo_map(args.repo_root, budget)

        if raw_map is None:
            print(f"  FAILED: could not generate repo map")
            results[budget] = {"error": "failed to generate"}
            continue

        print(f"  Files in map: {len(files)}")
        if files:
            print(f"  First 5: {files[:5]}")

        result = {
            "budget": budget,
            "files": files,
            "file_count": len(files),
            "raw_length": len(raw_map),
        }

        # Compare against ground truth
        if ground_truth:
            overlap = compute_overlap(files, ground_truth, args.repo_root)
            result["overlap"] = overlap
            print(f"  Recall: {overlap['recall']:.1%} ({len(overlap['found'])}/{overlap['ground_truth_total']})")
            print(f"  Precision: {overlap['precision']:.1%} ({len(overlap['found'])}/{overlap['aider_total']})")
            if overlap["missed"]:
                print(f"  Missed: {overlap['missed']}")

        results[budget] = result

        # Save raw map
        raw_path = output_dir / f"aider_{repo_name}_{subject_stem}_{budget}t.txt"
        with open(raw_path, "w") as f:
            f.write(raw_map)
        print(f"  Raw map saved: {raw_path}")

    # Save structured results
    output_path = output_dir / f"aider_{repo_name}_{subject_stem}_results.json"
    with open(output_path, "w") as f:
        json.dump({
            "tool": "aider",
            "repo": repo_name,
            "subject": args.subject_file,
            "ground_truth_file": args.ground_truth,
            "results": {str(k): v for k, v in results.items()},
        }, f, indent=2)
    print(f"\nResults saved: {output_path}")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Score C# repos against the grader rubric using the contextception CLI."""
import json
import subprocess
import sys
import os

BIN = os.path.join(os.path.dirname(__file__), "..", "bin", "contextception")
REPOS = {
    "jellyfin": os.path.expanduser("~/Repositories/test-corpus/jellyfin"),
    "efcore": os.path.expanduser("~/Repositories/test-corpus/efcore"),
    "avalonia": os.path.expanduser("~/Repositories/test-corpus/avalonia"),
    "orleans": os.path.expanduser("~/Repositories/test-corpus/orleans"),
    "roslyn": os.path.expanduser("~/Repositories/test-corpus/roslyn"),
}

def run(args, cwd):
    r = subprocess.run(args, capture_output=True, text=True, cwd=cwd, timeout=120)
    return r.stdout

def index_repo(repo_path):
    run([BIN, "index"], repo_path)

def get_archetypes(repo_path):
    out = run([BIN, "archetypes"], repo_path)
    return json.loads(out)

def analyze(repo_path, file):
    out = run([BIN, "analyze", "--json", file], repo_path)
    return json.loads(out)

def grade_output(data):
    """Simplified grading matching the Go grader logic."""
    scores = {}

    # must_read (weight 0.40)
    mr_score = 4.0
    if data["confidence"] < 0.8:
        mr_score -= 1.0
    elif data["confidence"] < 0.95:
        mr_score -= 0.5
    is_csharp = data["subject"].endswith(".cs")
    if data["must_read"]:
        eligible = sum(1 for e in data["must_read"] if e.get("direction", "") not in ("", "same_package") and not is_csharp)
        with_syms = sum(1 for e in data["must_read"] if e.get("direction", "") not in ("", "same_package") and not is_csharp and e.get("symbols"))
        with_dir = sum(1 for e in data["must_read"] if e.get("direction"))
        if eligible > 0 and with_syms / eligible < 0.3:
            mr_score -= 0.5
        if with_dir / len(data["must_read"]) < 0.5:
            mr_score -= 0.5
    scores["must_read"] = max(1.0, min(4.0, mr_score))

    # likely_modify (weight 0.20)
    lm_entries = sum(len(v) for v in data.get("likely_modify", {}).values())
    if lm_entries == 0:
        scores["likely_modify"] = 3.0
    else:
        lm_score = 4.0
        tiers = {"high": 0, "medium": 0, "low": 0}
        signals = set()
        for entries in data.get("likely_modify", {}).values():
            for e in entries:
                tiers[e.get("confidence", "low")] += 1
                for s in e.get("signals", []):
                    sig = s.split(":")[0] if ":" in s else s
                    signals.add(sig)
        if lm_entries >= 3 and (tiers["high"] == lm_entries or tiers["low"] == lm_entries):
            lm_score -= 0.5
        if len(signals) < 2 and lm_entries >= 3:
            lm_score -= 0.5
        scores["likely_modify"] = max(1.0, min(4.0, lm_score))

    # tests (weight 0.15)
    tests = data.get("tests", [])
    if not tests:
        from pathlib import PurePosixPath
        stem = PurePosixPath(data["subject"]).stem
        is_test = stem.endswith("Test") or stem.endswith("Tests") or stem.startswith("Test") or stem.endswith("Spec")
        if is_test:
            scores["tests"] = 4.0
        elif data.get("tests_note", "") == "no test files found in nearby directories":
            scores["tests"] = 3.0
        else:
            scores["tests"] = 2.0
    else:
        t_score = 4.0
        direct = sum(1 for t in tests if t.get("direct"))
        if direct == 0:
            t_score -= 1.0
        if len(tests) >= 2 and direct == 0:
            t_score -= 0.5
        scores["tests"] = max(1.0, min(4.0, t_score))

    # related (weight 0.15)
    rel_entries = sum(len(v) for v in data.get("related", {}).values())
    if rel_entries == 0:
        scores["related"] = 2.5
    else:
        r_score = 4.0
        useful = False
        for entries in data.get("related", {}).values():
            for e in entries:
                for s in e.get("signals", []):
                    if any(k in s for k in ["distance:2", "co_change", "structural", "hidden_coupling",
                                             "two_hop", "transitive_caller", "same_package", "high_churn",
                                             "hotspot", "imports", "imported_by"]):
                        useful = True
        if not useful and rel_entries >= 3:
            r_score -= 0.5
        scores["related"] = max(1.0, min(4.0, r_score))

    # blast_radius (weight 0.10)
    br = data.get("blast_radius")
    if not br:
        scores["blast_radius"] = 1.0
    else:
        br_score = 4.0
        if br.get("level") not in ("low", "medium", "high"):
            br_score -= 2.0
        if not br.get("detail"):
            br_score -= 1.0
        if br.get("level") == "high" and lm_entries < 3:
            br_score -= 0.5
        if br.get("level") == "low" and lm_entries > 10:
            br_score -= 0.5
        scores["blast_radius"] = max(1.0, min(4.0, br_score))

    overall = (scores["must_read"] * 0.40 + scores["likely_modify"] * 0.20 +
               scores["tests"] * 0.15 + scores["related"] * 0.15 +
               scores["blast_radius"] * 0.10)
    return scores, overall

def main():
    results = {}
    for name, path in REPOS.items():
        if not os.path.isdir(path):
            print(f"  SKIP {name} (not found at {path})")
            continue

        print(f"\n{'='*60}")
        print(f"  {name}")
        print(f"{'='*60}")

        index_repo(path)
        archetypes = get_archetypes(path)

        file_scores = []
        section_totals = {"must_read": 0, "likely_modify": 0, "tests": 0, "related": 0, "blast_radius": 0}

        for arch in archetypes:
            f = arch["file"]
            try:
                data = analyze(path, f)
                scores, overall = grade_output(data)
                file_scores.append((f, arch["archetype"], scores, overall))
                for k, v in scores.items():
                    section_totals[k] += v
            except Exception as e:
                print(f"  ERROR {f}: {e}")

        if not file_scores:
            print("  No files scored")
            continue

        n = len(file_scores)
        avg = sum(o for _, _, _, o in file_scores) / n

        # Print per-file breakdown
        for f, arch, scores, overall in sorted(file_scores, key=lambda x: x[3]):
            grade = "A" if overall >= 3.5 else "B" if overall >= 2.5 else "C" if overall >= 1.5 else "D"
            print(f"  {grade} {overall:.2f}  mr={scores['must_read']:.1f} lm={scores['likely_modify']:.1f} t={scores['tests']:.1f} r={scores['related']:.1f} br={scores['blast_radius']:.1f}  {os.path.basename(f)} [{arch}]")

        # Section averages
        print(f"\n  Sections:  mr={section_totals['must_read']/n:.2f}  lm={section_totals['likely_modify']/n:.2f}  t={section_totals['tests']/n:.2f}  r={section_totals['related']/n:.2f}  br={section_totals['blast_radius']/n:.2f}")

        grade = "A" if avg >= 3.5 else "B" if avg >= 2.5 else "C" if avg >= 1.5 else "D"
        print(f"  Overall: {avg:.2f} ({grade})")
        results[name] = avg

    print(f"\n{'='*60}")
    print(f"  AGGREGATE")
    print(f"{'='*60}")
    if results:
        total = sum(results.values()) / len(results)
        for name, score in sorted(results.items(), key=lambda x: x[1]):
            grade = "A" if score >= 3.5 else "B" if score >= 2.5 else "C" if score >= 1.5 else "D"
            print(f"  {grade} {score:.2f}  {name}")
        grade = "A" if total >= 3.5 else "B" if total >= 2.5 else "C" if total >= 1.5 else "D"
        print(f"\n  Aggregate: {total:.2f} ({grade})")

if __name__ == "__main__":
    main()

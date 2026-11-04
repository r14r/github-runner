#!/usr/bin/env python3
"""Guide verification and PR acceptance without redundant CI runs."""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys


def command(*args: str, capture: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, text=True, capture_output=capture, check=False)


def query_json(*args: str):
    result = command(*args)
    if result.returncode:
        raise RuntimeError(f"{' '.join(args)}: {result.stderr.strip() or result.stdout.strip()}")
    return json.loads(result.stdout)


def show(*args: str) -> None:
    print("  $ " + " ".join(args))


def execute(*args: str) -> bool:
    show(*args)
    result = subprocess.run(args, check=False)
    if result.returncode:
        print(f"  FAILED (exit {result.returncode}); fix this before accepting the PR.")
        return False
    return True


def check_label(pr: dict) -> str:
    checks = pr.get("statusCheckRollup") or []
    if not checks:
        return "NO CHECKS"
    bad = {"FAILURE", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "CANCELLED"}
    pending = {"PENDING", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "EXPECTED"}
    results = [(x.get("conclusion") or x.get("state") or x.get("status") or "").upper() for x in checks]
    if any(state in bad for state in results):
        return "FAIL"
    if any(state in pending or state not in {"SUCCESS", "NEUTRAL", "SKIPPED", "COMPLETED"} for state in results):
        return "PENDING"
    return "PASS"


def main() -> int:
    parser = argparse.ArgumentParser(description="Show or execute the next verification and PR review steps.")
    parser.add_argument("--run", action="store_true", help="Run local checks and query GitHub checks; confirm before merging current PR.")
    args = parser.parse_args()

    branch_result = command("git", "branch", "--show-current")
    if branch_result.returncode:
        print("ERROR: not inside a Git repository", file=sys.stderr)
        return 2
    branch = branch_result.stdout.strip()
    dirty = bool(command("git", "status", "--porcelain").stdout.strip())
    print(f"CURRENT BRANCH: {branch or '(detached HEAD)'}")
    print(f"WORKTREE: {'DIRTY (commit or stash changes before merging)' if dirty else 'clean'}")

    try:
        repo = query_json("gh", "repo", "view", "--json", "nameWithOwner")["nameWithOwner"]
        prs = query_json(
            "gh", "pr", "list", "--repo", repo, "--state", "open", "--limit", "100",
            "--json", "number,title,url,headRefName,baseRefName,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup",
        )
    except (RuntimeError, json.JSONDecodeError, KeyError) as exc:
        print(f"ERROR: cannot inspect GitHub PRs: {exc}", file=sys.stderr)
        return 2

    current = next((pr for pr in prs if pr["headRefName"] == branch), None)
    print(f"REPOSITORY: {repo}")
    print(f"OPEN PULL REQUESTS: {len(prs)}")
    if prs:
        for pr in prs:
            state = check_label(pr)
            merge_state = pr.get("mergeStateStatus") or "UNKNOWN"
            flags = ", ".join(filter(None, ["DRAFT" if pr.get("isDraft") else "", f"checks={state}", f"merge={merge_state}"]))
            mark = " (CURRENT)" if pr is current else ""
            print(f"  #{pr['number']} {pr['title']}{mark}")
            print(f"    {pr['headRefName']} -> {pr['baseRefName']} | {flags}")
            print(f"    {pr['url']}")
    else:
        print("  No open pull requests.")

    print("\nNEXT STEPS — local validation")
    print("  Note: 'just build' already executes 'just ci', including format, vet, tests and race tests.")
    if not args.run:
        show("git", "diff", "--check")
        show("just", "build")
        print("  Add --run to execute these checks.")
    else:
        if not execute("git", "diff", "--check"):
            return 1
        if not execute("just", "build"):
            return 1

    print("\nNEXT STEPS — pull requests")
    if not prs:
        print("  No PR to accept. Check 'git status' and push/commit changes if needed.")
        return 0
    for pr in prs:
        num = str(pr["number"])
        label = check_label(pr)
        print(f"  PR #{num}:")
        show("gh", "pr", "checks", num, "--repo", repo)
        if pr.get("isDraft"):
            print("    Draft: mark ready for review when finished.")
        if pr.get("mergeable") == "CONFLICTING" or pr.get("mergeStateStatus") == "DIRTY":
            print("    Resolve merge conflicts before accepting.")
        elif label == "FAIL":
            print("    Fix failing checks, push, and recheck.")
        elif label == "PENDING":
            print("    Wait for checks to finish; do not merge yet.")
        elif label == "NO CHECKS":
            print("    No CI results reported; verify branch checks and review manually.")
        if pr is current:
            show("gh", "pr", "merge", num, "--squash", "--delete-branch")
        else:
            print(f"    To validate locally: gh pr checkout {num} && just build")
            print("    Review and merge that PR separately after its checks pass.")

    if not args.run:
        return 0

    # Read-only remote checks for every open PR, including PRs on other branches.
    for pr in prs:
        num = str(pr["number"])
        print(f"\nGitHub checks for PR #{num}:")
        result = command("gh", "pr", "checks", num, "--repo", repo)
        print((result.stdout or result.stderr).rstrip() or "(No checks reported)")
        if result.returncode:
            print("  Checks not passing or not available; do not merge this PR yet.")

    if current is None:
        print("\nNo open PR matches the local branch; no merge attempted.")
        return 0
    fresh = query_json(
        "gh", "pr", "view", str(current["number"]), "--repo", repo,
        "--json", "isDraft,mergeable,mergeStateStatus,statusCheckRollup,reviewDecision",
    )
    state = check_label(fresh)
    if dirty or fresh.get("isDraft") or fresh.get("mergeable") != "MERGEABLE" or state != "PASS":
        print(f"\nPR #{current['number']} NOT merged: worktree/mergeability/review/CI need attention "
              f"(checks={state}, mergeable={fresh.get('mergeable')}).")
        return 0
    if fresh.get("reviewDecision") in {"REVIEW_REQUIRED", "CHANGES_REQUESTED"}:
        print("\nPR needs approval or requested changes; not merging.")
        return 0
    answer = input(f"\nMerge current PR #{current['number']} with --squash --delete-branch? [y/N]: ").strip().lower()
    if answer not in {"y", "yes"}:
        print("Merge skipped.")
        return 0
    return 0 if execute("gh", "pr", "merge", str(current["number"]), "--repo", repo, "--squash", "--delete-branch") else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (KeyboardInterrupt, EOFError):
        print("\nCancelled; no PR merged.", file=sys.stderr)
        sys.exit(130)

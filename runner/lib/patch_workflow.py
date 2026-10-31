#!/usr/bin/env python3
"""Dependency-free, deliberately restricted workflow job runner editor.

Supports only top-level jobs with a scalar runs-on; never rewrites dynamic,
multiline, or nested runner expressions. Plans and validates *all* files before
writing any of them. Git status should always be reviewed before PR creation.
"""
from __future__ import annotations

import argparse
from dataclasses import dataclass
from pathlib import Path
import re
import sys

JOB_RE = re.compile(r"^  ([A-Za-z0-9_.-]+):\s*(?:#.*)?$")
RUNS_ON_RE = re.compile(r"^(    )runs-on:\s*(.*?)\s*$")


@dataclass
class Plan:
    path: Path
    before: str
    after: str
    found: set[str]
    changed: int


def plan_patch(path: Path, label: str, allow_cross_os: bool,
               selected_jobs: set[str] | None, os_name: str) -> Plan:
    p = path.as_posix()
    if not p.startswith('.github/workflows/') and '/.github/workflows/' not in p:
        raise ValueError(f"Not a GitHub Actions workflow: {path}")
    if path.suffix not in {'.yml', '.yaml'} or path.is_symlink():
        raise ValueError(f"Unsafe or unsupported workflow path: {path}")
    before = path.read_text(encoding='utf-8')
    lines = before.splitlines(keepends=True)
    out = list(lines)
    inside_jobs = False
    current_job = None
    found: set[str] = set()
    changed = 0
    target = f'[self-hosted, {os_name}, X64, {label}]'
    for i, line in enumerate(lines):
        value = line.rstrip('\r\n')
        if value.startswith('jobs:') and re.fullmatch(r'jobs:\s*(?:#.*)?', value):
            inside_jobs = True
            current_job = None
            continue
        if not inside_jobs or not value.strip() or value.lstrip().startswith('#'):
            continue
        if not value.startswith(' '):
            inside_jobs = False
            current_job = None
            continue
        match = JOB_RE.fullmatch(value)
        if match:
            current_job = match.group(1)
            continue
        if not current_job or (selected_jobs is not None and current_job not in selected_jobs):
            continue
        # Only a job's direct runs-on (exactly four-space indentation).
        if not value.startswith('    ') or value.startswith('     '):
            continue
        run = RUNS_ON_RE.fullmatch(value)
        if not run:
            continue
        found.add(current_job)
        original = run.group(2).strip()
        if original == target:
            continue
        if not original or '${{' in original or original.startswith(('!', '*', '&')):
            raise ValueError(f'{path}:{i+1}: dynamic/multiline runs-on; edit manually')
        if '#' in original:
            raise ValueError(f'{path}:{i+1}: inline comments require manual review')
        if original.startswith('[') and not original.endswith(']'):
            raise ValueError(f'{path}:{i+1}: multiline runs-on; edit manually')
        lower = original.lower()
        previous_os = ('Windows' if 'windows' in lower or 'win-' in lower else
                       'macOS' if 'macos' in lower or 'osx' in lower else
                       'Linux' if 'ubuntu' in lower or 'linux' in lower else None)
        if previous_os is not None and previous_os != os_name and not allow_cross_os:
            raise ValueError(f'{path}:{i+1}: {previous_os} -> {os_name} requires --allow-cross-os')
        newline = '\r\n' if line.endswith('\r\n') else '\n' if line.endswith('\n') else ''
        out[i] = f'{run.group(1)}runs-on: {target}{newline}'
        changed += 1
    return Plan(path=path, before=before, after=''.join(out), found=found, changed=changed)


def patch_files(paths: list[Path], label: str, allow_cross_os: bool,
                selected_jobs: set[str] | None = None, os_name: str = 'macOS') -> int:
    if os_name not in {'Linux', 'macOS'} or not re.fullmatch(r'[A-Za-z0-9_.-]+', label):
        raise ValueError('Invalid runner OS/label')
    plans = [plan_patch(p, label, allow_cross_os, selected_jobs, os_name) for p in paths]
    found = set().union(*(p.found for p in plans))
    missing = (selected_jobs or set()) - found
    if missing:
        raise ValueError(f'Job(s) not found or not patchable: {", ".join(sorted(missing))}')
    if not found:
        raise ValueError('No supported job-level runs-on found')
    # All plans are prevalidated before the first file is touched.
    modified = []
    try:
        for p in plans:
            if p.after != p.before:
                p.path.write_text(p.after, encoding='utf-8')
                modified.append(p)
    except OSError:
        for p in modified:
            p.path.write_text(p.before, encoding='utf-8')
        raise
    total = sum(p.changed for p in plans)
    for p in plans:
        print(f'{p.path}: {p.changed} job(s) changed')
    return total


def patch_file(path: Path, label: str, allow_cross_os: bool,
               selected_jobs: set[str] | None = None, os_name: str = 'macOS') -> int:
    return patch_files([path], label, allow_cross_os, selected_jobs, os_name)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument('--label', required=True)
    parser.add_argument('--allow-cross-os', action='store_true')
    parser.add_argument('--os', choices=['macOS', 'Linux'], default='macOS')
    parser.add_argument('--job', action='append', default=[])
    parser.add_argument('workflow', type=Path, nargs='+')
    args = parser.parse_args()
    try:
        changed = patch_files(args.workflow, args.label, args.allow_cross_os,
                              set(args.job) if args.job else None, args.os)
    except (OSError, ValueError) as e:
        print(f'ERROR: {e}', file=sys.stderr)
        return 1
    if not changed:
        print('Already configured; no files changed')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

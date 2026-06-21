#!/usr/bin/env python3
"""PreToolUse hook: block `git commit` if staged changes contain hardcoded secrets/URLs.

Enforces Hard Rule #1 (env-only) mechanically. Reads the Claude Code hook JSON from
stdin; if the Bash command is a git commit, scans `git diff --cached` for secret-shaped
content and exits 2 (block) on a hit, otherwise exits 0 (allow).
"""
import json
import re
import subprocess
import sys

# (label, compiled pattern). Patterns are intentionally high-signal to avoid noise.
PATTERNS = [
    ("Google/Gemini API key", re.compile(r"AIza[0-9A-Za-z_\-]{20,}")),
    ("Anthropic API key", re.compile(r"sk-ant-[A-Za-z0-9\-]{20,}")),
    ("OpenAI-style key", re.compile(r"sk-[A-Za-z0-9]{32,}")),
    ("AWS access key id", re.compile(r"AKIA[0-9A-Z]{16}")),
    ("Postgres/Neon URL with password", re.compile(r"postgres(?:ql)?://[^\s:@]+:[^\s@]+@")),
    ("Inline secret assignment", re.compile(
        r"""(?i)(api[_-]?key|secret|token|password|passwd|client[_-]?secret)\s*[:=]\s*['"][^'"]{8,}['"]"""
    )),
]
# Placeholder values that are fine to commit (used in .env.example etc.).
PLACEHOLDER = re.compile(r"(?i)(your[_-]|example|placeholder|changeme|xxx|<[^>]+>|\$\{)")


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0  # never break the workflow on a parse error

    if payload.get("tool_name") != "Bash":
        return 0
    command = (payload.get("tool_input") or {}).get("command", "")
    if "git commit" not in command:
        return 0

    try:
        diff = subprocess.run(
            ["git", "diff", "--cached", "--unified=0"],
            capture_output=True, text=True, timeout=20,
        ).stdout
    except Exception:
        return 0

    findings = []
    for line in diff.splitlines():
        if not line.startswith("+") or line.startswith("+++"):
            continue  # only inspect added lines
        added = line[1:]
        if PLACEHOLDER.search(added):
            continue
        for label, pat in PATTERNS:
            if pat.search(added):
                findings.append(f"  - {label}: {added.strip()[:120]}")
                break

    if findings:
        msg = (
            "BLOCKED: staged changes look like they contain a hardcoded secret or "
            "credentialed URL (Hard Rule #1: env-only).\n"
            + "\n".join(findings)
            + "\nMove the value to env/Secret Manager (backend: internal/config; "
            "frontend: VITE_*). If this is a false positive, unstage the line or use a "
            "placeholder. Run /secret-scan for detail."
        )
        print(msg, file=sys.stderr)
        return 2  # exit code 2 => PreToolUse blocks the tool call

    return 0


if __name__ == "__main__":
    sys.exit(main())

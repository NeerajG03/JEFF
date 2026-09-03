#!/usr/bin/env python3
"""Audit JEFF files and integration surfaces for a new agent provider."""

from __future__ import annotations

import argparse
from pathlib import Path
import re
import sys


SURFACES = {
    "provider contract": r"AgentProvider|RegisterProvider|RegisteredAgents|AgentTool",
    "model routing": r"OwnsModel|InferBackend|IsValidModel|ModelExamples",
    "launch and resume": r"BuildLaunchArgs|BuildCurateArgs|ResumeSessionID|SupportsInlinePrompt",
    "workspace paths": r"ConfigDir|SkillsSubdir|CommandsSubdir|ContextFile",
    "hooks": r"HookDeliveryKey|RegisterDelivery|DeliveryKeys|TaskHooksStale",
    "provider-specific assumptions": r"AgentClaudeCode|AgentOpenCode|AgentGemini|\.claude|\.opencode|\.gemini",
}

INCLUDE_SUFFIXES = {".go", ".md", ".json", ".yaml", ".yml"}
SKIP_DIRS = {".git", "vendor", "node_modules"}


def source_files(root: Path):
    for path in root.rglob("*"):
        if not path.is_file() or path.suffix not in INCLUDE_SUFFIXES:
            continue
        if any(part in SKIP_DIRS for part in path.parts):
            continue
        yield path


def audit_surfaces(root: Path, provider: str | None = None) -> int:
    files = list(source_files(root))
    searches = dict(SURFACES)
    if provider:
        searches[f"provider match: {provider}"] = re.escape(provider)

    print("==================================================")
    print("JEFF Provider Surface Audit")
    print("==================================================")

    for label, pattern in searches.items():
        regex = re.compile(pattern, re.IGNORECASE)
        matches = []
        for path in files:
            try:
                text = path.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                continue
            count = len(regex.findall(text))
            if count:
                matches.append((path.relative_to(root), count))

        print(f"\n[{label}] {len(matches)} files")
        for path, count in sorted(matches, key=lambda item: str(item[0])):
            print(f"  {path} ({count})")

    if provider:
        print("\n--------------------------------------------------")
        print(f"Checking essential surfaces for provider: '{provider}'")
        print("--------------------------------------------------")
        agent_file = root / f"agent_{provider.lower()}.go"
        has_agent_file = agent_file.is_file()
        print(f"  • agent_{provider.lower()}.go exists: {'YES' if has_agent_file else 'NO'}")

        schema_file = root / "schemas" / "jeff-config.json"
        has_schema = False
        if schema_file.is_file():
            has_schema = f'"{provider.lower()}"' in schema_file.read_text(encoding="utf-8")
        print(f"  • registered in schemas/jeff-config.json: {'YES' if has_schema else 'NO'}")

        delivery_file = root / "hooks" / f"delivery_{provider.lower()}.go"
        has_delivery = delivery_file.is_file()
        print(f"  • hook delivery (hooks/delivery_{provider.lower()}.go): {'YES' if has_delivery else 'NO (or optional)'}")

    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Audit JEFF integration surfaces before or after adding an agent provider."
    )
    parser.add_argument("root", nargs="?", default=".", help="JEFF repository root")
    parser.add_argument(
        "--provider",
        help="provider name to audit (e.g. codex)",
    )
    args = parser.parse_args()

    root = Path(args.root).resolve()
    if not (root / "go.mod").is_file() or not (root / "agent.go").is_file():
        parser.error(f"{root} does not look like the JEFF repository root")

    return audit_surfaces(root, args.provider)


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Install the shared skill without modifying agent settings or credentials."""
import argparse
import os
from pathlib import Path
import shutil


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    target = parser.add_mutually_exclusive_group(required=True)
    target.add_argument('--agent', choices=['codex', 'opencode', 'claude-code'])
    target.add_argument('--dest', type=Path, help='custom skills root directory')
    parser.add_argument('--force', action='store_true', help='replace an existing SKILL.md')
    args = parser.parse_args()
    roots = {
        'codex': Path.home() / '.agents/skills',
        'opencode': Path(os.environ.get('XDG_CONFIG_HOME') or Path.home() / '.config') / 'opencode/skills',
        'claude-code': Path.home() / '.claude/skills',
    }
    destination = (args.dest or roots[args.agent]).expanduser() / 'remote-computer-use'
    if destination.is_symlink():
        parser.error('destination is a symlink; choose a different skills root')
    destination.mkdir(parents=True, exist_ok=True)
    output = destination / 'SKILL.md'
    source = Path(__file__).resolve().parents[1] / 'skills/remote-computer-use/SKILL.md'
    if output.is_symlink():
        parser.error('existing SKILL.md is a symlink')
    if output.exists() and not args.force:
        if output.read_bytes() == source.read_bytes():
            print(f'Already installed: {output}')
            return
        parser.error('SKILL.md already exists; use --force to replace it')
    shutil.copyfile(source, output)
    print(f'Installed: {output}')


if __name__ == '__main__':
    main()

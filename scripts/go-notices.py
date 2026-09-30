#!/usr/bin/env python3
"""Collect notices from the exact Go modules used by this checkout."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess

LICENSES = {
    'github.com/BurntSushi/toml': 'MIT',
    'github.com/goexlib/cgo': 'MIT',
    'github.com/ebitengine/purego': 'Apache-2.0',
    'golang.org/x/sys': 'BSD-3-Clause',
}


def collect(destination):
    go = os.environ.get('GO', 'go')
    modules = subprocess.check_output([go, 'list', '-m', '-json', 'all'], text=True)
    decoder = json.JSONDecoder()
    inventory = []
    while modules.strip():
        module, end = decoder.raw_decode(modules.lstrip())
        modules = modules.lstrip()[end:]
        if module.get('Main'):
            continue
        name = module['Path']
        if name not in LICENSES or 'Replace' in module:
            raise RuntimeError(f'review the new/replaced Go dependency license: {name}')
        source = Path(module['Dir'])
        dest = destination / name / module['Version']
        dest.mkdir(parents=True, exist_ok=True)
        found = []
        for f in source.iterdir():
            if f.is_file() and f.name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING', 'NOTICE', 'PATENTS', 'COPYRIGHT')):
                shutil.copyfile(f, dest / f.name)
                found.append(f.name)
        if not any(f.upper().startswith(('LICENSE', 'LICENCE', 'COPYING')) for f in found):
            raise RuntimeError(f'missing license text for {name}')
        inventory.append({'module': name, 'version': module['Version'], 'license': LICENSES[name], 'notices': found})
    goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], text=True).strip())
    toolchain = destination / 'Go'
    toolchain.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(goroot / 'LICENSE', toolchain / 'LICENSE')
    inventory.append({'module': 'Go standard library', 'version': subprocess.check_output([go, 'env', 'GOVERSION'], text=True).strip(), 'license': 'BSD-3-Clause'})
    (destination / 'inventory.json').write_text(json.dumps(inventory, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('destination', type=Path)
    collect(parser.parse_args().destination)

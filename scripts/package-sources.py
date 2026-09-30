#!/usr/bin/env python3
"""Package a verified native source collection with this commit's own sources."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('collection', type=Path)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]*', args.version):
        raise SystemExit('invalid version')
    root = Path(__file__).resolve().parents[1]
    collection = args.collection.resolve()
    index = json.loads((collection / 'index.json').read_text())
    targets = {'linux-amd64.tar.gz', 'linux-arm64.tar.gz', 'darwin-arm64.tar.gz', 'windows-amd64.zip'}
    expected = {f'remote-computer-use-{args.version}-{t}' for t in targets}
    if index.get('errors') or set(index['platforms']) != expected:
        raise SystemExit('incomplete source collection or platform/version mismatch')
    for package in index['packages'].values():
        for entry in package['files']:
            path = collection / entry['file']
            if not path.resolve().is_relative_to(collection) or hashlib.sha256(path.read_bytes()).hexdigest() != entry['sha256']:
                raise SystemExit('source hash mismatch')
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=root).strip():
        raise SystemExit('commit the release sources before packaging')
    stage = Path(tempfile.mkdtemp(prefix='rcu-release-sources-', dir=root / 'build'))
    package = stage / f'remote-computer-use-{args.version}-sources'
    package.mkdir()
    shutil.copy2(collection / 'index.json', package / 'index.json')
    for item in index['packages'].values():
        for entry in item['files']:
            dest = package / entry['file']
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(collection / entry['file'], dest)
    project = package / 'project'
    project.mkdir()
    files = subprocess.check_output(['git', 'ls-files', '-z'], cwd=root).decode().split('\0')
    for name in filter(None, files):
        dest = project / name
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(root / name, dest)
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    (package / 'PROJECT-COMMIT.txt').write_text(commit + '\n')
    go = os.environ.get('GO', 'go')
    modules = subprocess.check_output([go, 'list', '-m', '-json', 'all'], cwd=root, text=True)
    decoder = json.JSONDecoder()
    while modules.strip():
        module, end = decoder.raw_decode(modules.lstrip())
        modules = modules.lstrip()[end:]
        if not module.get('Main'):
            if 'Replace' in module:
                raise SystemExit('review source collection for replaced Go modules')
            shutil.copytree(module['Dir'], package / 'go-modules' / (module['Path'] + '@' + module['Version']), dirs_exist_ok=True)
    (package / 'README.md').write_text(
        '# Corresponding sources\n\n'
        'index.json maps each platform to exact native source packages and hashes.\n'
        'native/ contains upstream source plus distributor patches and build recipes.\n'
        'For Ubuntu, use dpkg-source -x on the .dsc in its directory. For MSYS2,\n'
        'unpack the .src.tar.zst and inspect PKGBUILD; VCS packages include a bare Git repository.\n'
        'Homebrew directories contain the installed formula, resources and patches.\n'
        'project/ contains the project source and native build/packaging scripts;\n'
        'go-modules/ contains the exact external Go module sources.\n'
        'Follow project/README.md and project/THIRD_PARTY.md for rebuilding and\n'
        'using ABI-compatible modified libraries via REMOTE_COMPUTER_LIB_DIR.\n'
        'Original upstream license notices remain part of each source package.\n'
    )
    name = f'remote-computer-use-{args.version}-sources'
    archive = Path(shutil.make_archive(str(root / 'dist' / name), 'gztar', stage, package.name))
    archive.with_name(archive.name + '.sha256').write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name + '\n')
    print('Created ' + str(archive))
    shutil.rmtree(stage)


if __name__ == '__main__':
    main()

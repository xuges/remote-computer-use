#!/usr/bin/env python3
"""Collect trusted, locally built ELF adapters and their runtime closure.

Release builder: Ubuntu 22.04 amd64. No system libraries are modified.
"""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
# Keep the loader and glibc family supplied by the host as one coherent ABI.
SYSTEM = re.compile(r'^(ld-linux.*|lib(c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so\..*)$')


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE).strip()


def dependencies(path):
    # ldd is only run on our trusted build output, never on downloaded input.
    output = run('ldd', str(path))
    if 'not found' in output:
        raise RuntimeError(f'unresolved dependency in {path.name}')
    for line in output.splitlines():
        match = re.match(r'\s*(\S+) => (/.*?) \(0x[0-9a-f]+\)', line)
        if match:
            yield match[1], Path(match[2]).resolve()


def package_for(path):
    candidates = [str(path)]
    if str(path).startswith('/usr/lib/'):
        candidates.append(str(path)[4:])
    elif str(path).startswith('/lib/'):
        candidates.append('/usr' + str(path))
    for candidate in candidates:
        try:
            return run('dpkg-query', '-S', candidate).splitlines()[0].rsplit(': ', 1)[0]
        except subprocess.CalledProcessError:
            pass
    raise RuntimeError(f'cannot identify license/source package for {path.name}')


def build(args):
    if platform.system() != 'Linux' or platform.machine() != 'x86_64':
        raise RuntimeError('release packaging currently supports Linux amd64 only')
    for command in ('ldd', 'readelf', 'patchelf', 'dpkg-query'):
        if not shutil.which(command):
            raise RuntimeError(f'missing build tool: {command}')
    adapters = [args.bin / f'libremote-computer-{p}.so' for p in ('vnc', 'rdp')]
    libraries = {p.name: p.resolve(strict=True) for p in adapters}
    for adapter in adapters:
        for name, source in dependencies(adapter):
            if SYSTEM.match(name):
                continue
            if name in libraries and libraries[name] != source:
                raise RuntimeError(f'conflicting SONAME: {name}')
            libraries[name] = source
    # Fail early if a future build accidentally pulls GUI/audio dependencies in.
    if any(re.match(r'lib(X11|wayland|SDL|pulse|asound)', n) for n in libraries):
        raise RuntimeError('runtime includes GUI/audio libraries; rebuild headless FreeRDP')
    args.output.mkdir(parents=True, exist_ok=True)
    args.notices.mkdir(parents=True, exist_ok=True)
    packages = {}
    inventory = []
    max_glibc = (0, 0)
    with tempfile.TemporaryDirectory(prefix='rcu-bundle-') as tmp:
        tmp = Path(tmp)
        for name, source in sorted(libraries.items()):
            if '/' in name or name in ('.', '..'):
                raise RuntimeError('invalid ELF library name')
            dest = tmp / name
            shutil.copyfile(source, dest)
            # Every object needs its own origin-relative RUNPATH for transitive deps.
            subprocess.run(['patchelf', '--set-rpath', '$ORIGIN', str(dest)], check=True)
            for version in re.findall(r'GLIBC_(\d+\.\d+(?:\.\d+)?)', run('readelf', '--version-info', str(dest))):
                max_glibc = max(max_glibc, tuple(map(int, version.split('.'))))
            if name.startswith('libremote-computer-'):
                owner = 'remote-computer-use'
            elif name.startswith(('libfreerdp', 'libwinpr')):
                owner = 'FreeRDP-3.25.0'
                shutil.copyfile(args.freerdp_source / 'LICENSE', args.notices / 'FreeRDP-LICENSE')
                packages[owner] = {'source': 'FreeRDP', 'version': '3.25.0', 'source_url': 'https://github.com/FreeRDP/FreeRDP/tree/3.25.0'}
            else:
                owner = package_for(source)
                if owner not in packages:
                    fields = run('dpkg-query', '-W', '-f=${Version}\t${source:Package}\t${source:Version}', owner).split('\t')
                    copyright_file = Path('/usr/share/doc') / owner.split(':')[0] / 'copyright'
                    shutil.copyfile(copyright_file, args.notices / (owner.replace(':', '_') + '-copyright'))
                    packages[owner] = {'version': fields[0], 'source': fields[1], 'source_version': fields[2], 'source_url': f'https://launchpad.net/ubuntu/+source/{fields[1]}/{fields[2]}'}
            inventory.append({'library': name, 'package': owner})
        if max_glibc > (2, 35):
            raise RuntimeError(f'build requires glibc {max_glibc}; rebuild on Ubuntu 22.04')
        files = []
        buf = io.BytesIO()
        with gzip.GzipFile(fileobj=buf, mode='wb', mtime=0, compresslevel=9) as zipped:
            with tarfile.open(fileobj=zipped, mode='w', format=tarfile.USTAR_FORMAT) as archive:
                for path in sorted(tmp.iterdir()):
                    data = path.read_bytes()
                    files.append({'name': path.name, 'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()})
                    entry = tarfile.TarInfo(path.name)
                    entry.size, entry.mode = len(data), 0o600
                    archive.addfile(entry, io.BytesIO(data))
        # Validate the relocated dependency closure before embedding it.
        for path in tmp.iterdir():
            for name, source in dependencies(path):
                if not SYSTEM.match(name) and source.parent != tmp:
                    raise RuntimeError(f'{path.name} still loads {name} outside the bundle')
        (args.output / 'runtime.tar.gz').write_bytes(buf.getvalue())
        (args.output / 'manifest.json').write_text(json.dumps({'format': 1, 'os': 'linux', 'arch': 'amd64', 'min_glibc': '2.35', 'files': files}, indent=2) + '\n')
    shutil.copytree('/usr/share/common-licenses', args.notices / 'common-licenses', dirs_exist_ok=True)
    (args.notices / 'inventory.json').write_text(json.dumps({'packages': packages, 'libraries': inventory}, indent=2) + '\n')
    (args.notices / 'README.txt').write_text('Native runtime libraries are dynamically loaded, not statically linked.\nCopyright/license texts and exact Ubuntu source package versions are included.\nSource URLs are in inventory.json. For Ubuntu sources, use apt-get source\nSOURCE=SOURCE_VERSION with matching deb-src repositories or the source URL.\nFreeRDP source is the pinned upstream 3.25.0 release (see build-freerdp.sh).\nOnly ELF RUNPATH metadata is adjusted; library code is not modified.\nREMOTE_COMPUTER_LIB_DIR can select replacement compatible libraries.\n')
    print(f'Bundled {len(libraries)} libraries, {len(buf.getvalue()) / 1048576:.1f} MiB compressed; glibc >= 2.35')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bin', type=Path, default=ROOT / 'bin')
    parser.add_argument('--output', type=Path, default=ROOT / 'internal/runtimebundle/payload')
    parser.add_argument('--notices', type=Path, default=ROOT / 'build/THIRD_PARTY_NOTICES')
    parser.add_argument('--freerdp-source', type=Path, default=ROOT / '.cache/FreeRDP-3.25.0')
    try:
        build(parser.parse_args())
    except (RuntimeError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f'bundle: {error}\n')

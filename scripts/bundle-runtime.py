#!/usr/bin/env python3
"""Collect trusted native adapters and their relocated runtime closure.

Run on the target platform: Ubuntu 22.04, macOS/Homebrew or Windows/MSYS2 UCRT64.
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
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
SYSTEM = re.compile(r'^(ld-linux.*|lib(c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so\..*)$')
OS = {'Linux': 'linux', 'Darwin': 'darwin', 'Windows': 'windows'}.get(platform.system())
# MSYS Python reports MSYS_NT, while UCRT Python reports Windows.
if platform.system().startswith(('MSYS', 'MINGW')):
    OS = 'windows'
ARCH = {'x86_64': 'amd64', 'AMD64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(platform.machine())


def run(*args):
    return subprocess.check_output(args, text=True, encoding='utf-8', stderr=subprocess.PIPE, env=os.environ | {'LC_ALL': 'C'}).strip()


def system_library(name):
    if OS == 'linux':
        return bool(SYSTEM.match(name))
    if OS == 'darwin':
        return name.startswith(('/usr/lib/', '/System/Library/'))
    return name.lower().startswith(('api-ms-', 'ext-ms-')) or (Path(os.environ['SYSTEMROOT']) / 'System32' / name).is_file()


def dependencies(path):
    if OS == 'linux':
        output = run('ldd', str(path))  # Only our own trusted build outputs.
        if 'not found' in output:
            raise RuntimeError(f'unresolved dependency in {path.name}')
        for line in output.splitlines():
            match = re.match(r'\s*(\S+) => (/.*?) \(0x[0-9a-f]+\)', line)
            if match and not system_library(match[1]):
                yield match[1], Path(match[2]).resolve()
    elif OS == 'darwin':
        output = run('otool', '-L', str(path))
        rpaths = re.findall(r'cmd LC_RPATH\n.*?\n\s*path (.*?) \(offset', run('otool', '-l', str(path)))
        for line in output.splitlines()[1:]:
            name = line.strip().split(' (compatibility version')[0]
            if system_library(name) or Path(name).name == path.name:
                continue
            if name.startswith('@loader_path/'):
                source = path.parent / name[len('@loader_path/'):]
            elif name.startswith('@rpath/'):
                candidates = [Path(p.replace('@loader_path', str(path.parent))) / name[len('@rpath/'):] for p in rpaths]
                candidates += [path.parent / Path(name).name]
                source = next((p for p in candidates if p.is_file()), None)
                if source is None:
                    raise RuntimeError(f'unresolved dependency {name} in {path.name}')
            else:
                source = Path(name)
            yield name, source.resolve(strict=True)
    else:
        for name in re.findall(r'DLL Name:\s*(\S+)', run('objdump', '-p', str(path))):
            if system_library(name):
                continue
            search = [path.parent, ROOT / '.cache/freerdp/bin'] + [Path(p) for p in os.get_exec_path()]
            source = next((p / name for p in search if (p / name).is_file()), None)
            if source is None:
                raise RuntimeError(f'unresolved dependency {name} in {path.name}')
            yield name, source.resolve()


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


def license_package(source, notices):
    if OS == 'linux':
        owner = package_for(source)
        fields = run('dpkg-query', '-W', '-f=${Version}\t${source:Package}\t${source:Version}', owner).split('\t')
        shutil.copyfile(Path('/usr/share/doc') / owner.split(':')[0] / 'copyright', notices / (owner.replace(':', '_') + '-copyright'))
        return owner, {'version': fields[0], 'source': fields[1], 'source_version': fields[2], 'source_url': f'https://launchpad.net/ubuntu/+source/{fields[1]}/{fields[2]}'}
    if OS == 'darwin':
        parts = source.parts
        index = parts.index('Cellar')
        owner, version = parts[index + 1:index + 3]
        prefix = Path(*parts[:index + 3])
        texts = [p for p in prefix.iterdir() if p.is_file() and re.match(r'(COPYING|LICENSE|LICENCE|NOTICE)', p.name, re.I)]
        formula = (prefix / '.brew' / (owner + '.rb')).read_text()
        url = re.search(r'^  url "([^"]+)"', formula, re.M)
        digest = re.search(r'^  sha256 "([0-9a-f]{64})"', formula, re.M)
        if not texts and url and digest:
            # Some bottles omit license files. Recover them from the exact,
            # checksum-verified source recorded in the installed formula.
            cache = ROOT / '.cache/license-sources'
            cache.mkdir(exist_ok=True)
            source_archive = cache / digest[1]
            if not source_archive.exists():
                with urllib.request.urlopen(url[1], timeout=90) as response:
                    source_archive.write_bytes(response.read())
            if hashlib.sha256(source_archive.read_bytes()).hexdigest() != digest[1]:
                raise RuntimeError(f'source checksum mismatch for {owner}')
            count = 0
            with tarfile.open(source_archive) as archive:
                for member in archive:
                    if member.isfile() and (re.match(r'(COPYING|LICENSE|LICENCE|NOTICE)', Path(member.name).name, re.I) or '/LICENSES/' in member.name):
                        (notices / (owner + '-' + str(count) + '-' + Path(member.name).name)).write_bytes(archive.extractfile(member).read())
                        count += 1
            if not count:
                raise RuntimeError(f'missing license text in source for {owner}')
        elif not texts:
            raise RuntimeError(f'missing license/source metadata for {owner}')
        for p in texts:
            shutil.copyfile(p, notices / (owner + '-' + p.name))
        return owner, {'version': version, 'source_url': url[1] if url else f'https://formulae.brew.sh/formula/{owner}', 'source_sha256': digest[1] if digest else None}
    # Query MSYS package ownership using a POSIX path even under native Python.
    posix = run('cygpath', '-u', str(source))
    owner = run('pacman', '-Qqo', posix)
    prefix = source.parent.parent
    license_dir = prefix / 'share/licenses' / owner.removeprefix('mingw-w64-ucrt-x86_64-')
    if not license_dir.is_dir():
        # Package directory conventions differ; use the exact installed file list.
        entries = run('pacman', '-Ql', owner).splitlines()
        texts = [Path(run('cygpath', '-w', line.split(' ', 1)[1])) for line in entries if '/share/licenses/' in line and not line.endswith('/')]
    else:
        texts = [p for p in license_dir.rglob('*') if p.is_file()]
    if not texts:
        # MSYS2 packages with standard licenses refer to the common collection.
        common = ROOT / 'scripts/licenses/common'
        if not common.is_dir():
            raise RuntimeError(f'missing standard license texts for {owner}')
        texts = [p for p in common.rglob('*') if p.is_file()]
    dest = notices / owner
    dest.mkdir(exist_ok=True)
    for i, p in enumerate(texts):
        shutil.copyfile(p, dest / (str(i) + '-' + p.name))
    (dest / 'package-info.txt').write_text(run('pacman', '-Qi', owner) + '\n')
    return owner, {'version': run('pacman', '-Q', owner).split(' ', 1)[1], 'source_url': f'https://packages.msys2.org/packages/{owner}'}


def build(args):
    if (OS, ARCH) not in {('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'arm64'), ('windows', 'amd64')}:
        raise RuntimeError('unsupported release target')
    extension = {'linux': 'so', 'darwin': 'dylib', 'windows': 'dll'}[OS]
    adapters = [args.bin / f'libremote-computer-{p}.{extension}' for p in ('vnc', 'rdp')]
    libraries = {p.name: p.resolve(strict=True) for p in adapters}
    edges = {}
    pending = list(libraries)
    while pending:
        name = pending.pop()
        edges[name] = list(dependencies(libraries[name]))
        for reference, source in edges[name]:
            dependency = Path(reference).name
            if dependency in libraries and libraries[dependency] != source:
                raise RuntimeError(f'conflicting library name: {dependency}')
            if dependency not in libraries:
                libraries[dependency] = source
                pending.append(dependency)
    if any(re.match(r'lib(X11|wayland|SDL|pulse|asound|gtk-)', n) for n in libraries):
        raise RuntimeError('runtime includes GUI/audio libraries; rebuild headless FreeRDP')
    args.output.mkdir(parents=True, exist_ok=True)
    args.notices.mkdir(parents=True, exist_ok=True)
    packages, inventory = {}, []
    minimum = (0, 0)
    with tempfile.TemporaryDirectory(prefix='rcu-bundle-') as tmp:
        tmp = Path(tmp).resolve()
        for name, source in sorted(libraries.items()):
            if not re.fullmatch(r'[A-Za-z0-9._+-]+', name):
                raise RuntimeError('invalid library name')
            dest = tmp / name
            shutil.copyfile(source, dest)
            if OS == 'linux':
                subprocess.run(['patchelf', '--set-rpath', '$ORIGIN', str(dest)], check=True)
                for version in re.findall(r'GLIBC_(\d+\.\d+(?:\.\d+)?)', run('readelf', '--version-info', str(dest))):
                    minimum = max(minimum, tuple(map(int, version.split('.'))))
            elif OS == 'darwin':
                run('install_name_tool', '-id', '@rpath/' + name, str(dest))
                for reference, _ in edges[name]:
                    run('install_name_tool', '-change', reference, '@loader_path/' + Path(reference).name, str(dest))
                # Remove build-machine search paths, then sign before hashing.
                for rpath in re.findall(r'cmd LC_RPATH\n.*?\n\s*path (.*?) \(offset', run('otool', '-l', str(dest))):
                    run('install_name_tool', '-delete_rpath', rpath, str(dest))
                run('codesign', '--force', '--sign', '-', str(dest))
                for block in run('otool', '-l', str(dest)).split('Load command '):
                    if re.search(r'cmd LC_BUILD_VERSION\b', block):
                        match = re.search(r'\bminos ([0-9.]+)', block)
                    elif re.search(r'cmd LC_VERSION_MIN_MACOSX\b', block):
                        match = re.search(r'\bversion ([0-9.]+)', block)
                    else:
                        continue
                    if match:
                        minimum = max(minimum, tuple(map(int, match[1].split('.'))))
            if name.startswith('libremote-computer-'):
                owner = 'remote-computer-use'
            elif name.startswith(('libfreerdp', 'libwinpr')):
                owner = 'FreeRDP-3.25.0'
                shutil.copyfile(args.freerdp_source / 'LICENSE', args.notices / 'FreeRDP-LICENSE')
                packages[owner] = {'source': 'FreeRDP', 'version': '3.25.0', 'source_url': 'https://github.com/FreeRDP/FreeRDP/tree/3.25.0'}
            else:
                owner, detail = license_package(source, args.notices)
                packages[owner] = detail
            inventory.append({'library': name, 'package': owner})
        if OS == 'linux' and minimum > (2, 35):
            raise RuntimeError(f'build requires glibc {minimum}; rebuild on Ubuntu 22.04')
        # Validate the relocated closure before embedding it.
        for path in tmp.iterdir():
            for _, source in dependencies(path):
                if source.parent != tmp:
                    raise RuntimeError(f'{path.name} still loads dependencies outside the bundle')
        files, buf = [], io.BytesIO()
        with gzip.GzipFile(fileobj=buf, mode='wb', mtime=0, compresslevel=9) as zipped:
            with tarfile.open(fileobj=zipped, mode='w', format=tarfile.USTAR_FORMAT) as archive:
                for path in sorted(tmp.iterdir()):
                    data = path.read_bytes()
                    files.append({'name': path.name, 'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()})
                    entry = tarfile.TarInfo(path.name)
                    entry.size, entry.mode = len(data), 0o600
                    archive.addfile(entry, io.BytesIO(data))
        metadata = {'format': 1, 'os': OS, 'arch': ARCH, 'files': files}
        if OS == 'linux':
            metadata['min_glibc'] = '2.35'
        elif OS == 'darwin':
            metadata['min_macos'] = '.'.join(map(str, minimum))
        (args.output / 'runtime.tar.gz').write_bytes(buf.getvalue())
        (args.output / 'manifest.json').write_text(json.dumps(metadata, indent=2) + '\n')
    if OS == 'linux':
        shutil.copytree('/usr/share/common-licenses', args.notices / 'common-licenses', dirs_exist_ok=True)
    (args.notices / 'inventory.json').write_text(json.dumps({'packages': packages, 'libraries': inventory}, indent=2) + '\n')
    (args.notices / 'README.txt').write_text('Native libraries are dynamically loaded. License texts, package versions and source URLs\nare in this directory. FreeRDP uses the pinned 3.25.0 upstream release.\nOnly loader metadata/signatures are adjusted; library code is not modified.\nREMOTE_COMPUTER_LIB_DIR can select replacement compatible libraries.\n')
    print(f'Bundled {len(libraries)} libraries for {OS}/{ARCH}, {len(buf.getvalue()) / 1048576:.1f} MiB compressed')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bin', type=Path, default=ROOT / 'bin')
    parser.add_argument('--output', type=Path, default=ROOT / 'internal/runtimebundle/payload')
    parser.add_argument('--notices', type=Path, default=ROOT / 'build/THIRD_PARTY_NOTICES')
    parser.add_argument('--freerdp-source', type=Path, default=ROOT / '.cache/FreeRDP-3.25.0')
    try:
        build(parser.parse_args())
    except (RuntimeError, OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f'bundle: {error}\n')

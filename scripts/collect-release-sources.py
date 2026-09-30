#!/usr/bin/env python3
"""Collect exact native source packages alongside release binaries.

Requires Python 3, network access, and zstd for inspecting MSYS2 source archives.
Never executes downloaded package recipes. A failed download/check is fatal.
"""
import argparse
import concurrent.futures
import hashlib
import io
import json
import re
from pathlib import Path
import shutil
import subprocess
import tarfile
import time
import urllib.parse
import urllib.request
import zipfile


def fetch(url, cache, expected=None):
    dest = cache / hashlib.sha256(url.encode()).hexdigest()
    if not dest.exists():
        candidates = [url]
        if url.startswith('https://ftpmirror.gnu.org/'):
            suffix = url.removeprefix('https://ftpmirror.gnu.org/').removeprefix('gnu/')
            candidates.append('https://ftp.gnu.org/gnu/' + suffix)
        for attempt in range(3):
            try:
                request = urllib.request.Request(candidates[min(attempt, len(candidates) - 1)], headers={'User-Agent': 'remote-computer-use-source-collector/0.1'})
                with urllib.request.urlopen(request, timeout=60) as response, dest.with_suffix('.part').open('wb') as out:
                    shutil.copyfileobj(response, out)
                dest.with_suffix('.part').replace(dest)
                break
            except Exception:
                if attempt == 2:
                    raise
                time.sleep(1 + attempt)
    digest = hashlib.sha256(dest.read_bytes()).hexdigest()
    if expected and digest != expected:
        raise RuntimeError(f'source checksum mismatch: {url}')
    return dest, digest


def inventory(path):
    if path.suffix == '.zip':
        with zipfile.ZipFile(path) as archive:
            return json.loads(archive.read(next(n for n in archive.namelist() if n.endswith('/THIRD_PARTY_NOTICES/inventory.json'))))
    with tarfile.open(path) as archive:
        return json.load(archive.extractfile(next(m for m in archive if m.name.endswith('/THIRD_PARTY_NOTICES/inventory.json'))))


def brew_recipes(path):
    if 'darwin-' not in path.name:
        return {}
    with tarfile.open(path) as archive:
        return {Path(m.name).name.removesuffix('-formula.rb'): archive.extractfile(m).read().decode()
                for m in archive if m.isfile() and m.name.endswith('-formula.rb')}


def brew_revision(owner, recipe, cache):
    def comparable(text):
        # Release checks/bottle hashes do not control source compilation.
        text = re.sub(r'^  (?:bottle|livecheck) do\n.*?^  end\n', '', text, flags=re.M | re.S)
        text = re.sub(r'^  #[^\n]*\n', '', text, flags=re.M)
        return re.sub(r'\n\s*\n', '\n', text).strip()
    api = json.loads(fetch(f'https://formulae.brew.sh/api/formula/{owner}.json', cache)[0].read_bytes())
    path = api['ruby_source_path']
    prefix = 'https://raw.githubusercontent.com/Homebrew/homebrew-core/' + api['tap_git_head'] + '/'
    original, _ = fetch(prefix + path, cache)
    if comparable(original.read_text()) == comparable(recipe):
        return prefix, path
    raise RuntimeError(f'cannot locate exact Homebrew build recipe revision: {owner}')


def collect_package(item, cache, output, recipes):
    key, owner, metadata = item
    directory = output / 'native' / key
    directory.mkdir(parents=True, exist_ok=True)
    files = []

    def save(url, expected=None, filename=None):
        src, digest = fetch(url, cache, expected)
        name = filename or Path(urllib.parse.unquote(urllib.parse.urlparse(url).path)).name
        if not name or Path(name).name != name:
            raise RuntimeError('invalid source filename')
        dest = directory / name
        shutil.copyfile(src, dest)
        files.append({'file': str(dest.relative_to(output)), 'url': url, 'sha256': digest})
        return dest

    if metadata.get('source_version'):
        query = urllib.parse.urlencode({'ws.op': 'getPublishedSources', 'source_name': metadata['source'], 'version': metadata['source_version'], 'exact_match': 'true'})
        url = 'https://api.launchpad.net/1.0/ubuntu/+archive/primary?' + query
        data = json.loads(fetch(url, cache)[0].read_bytes())
        if not data.get('entries'):
            raise RuntimeError(f'no exact Ubuntu source publication: {key}')
        urls = json.loads(fetch(data['entries'][0]['self_link'] + '?ws.op=sourceFileUrls', cache)[0].read_bytes())
        descriptor = save(next(u for u in urls if u.endswith('.dsc')))
        checksums = re.search(r'^Checksums-Sha256:\n((?: .+\n)+)', descriptor.read_text(), re.M)
        if not checksums:
            raise RuntimeError(f'missing source checksums: {key}')
        expected = {line.split()[2]: line.split()[0] for line in checksums[1].splitlines()}
        for url in urls:
            if url.endswith('.dsc'):
                continue
            name = Path(urllib.parse.unquote(urllib.parse.urlparse(url).path)).name
            if name not in expected:
                raise RuntimeError(f'unchecked Ubuntu source file: {name}')
            save(url, expected.pop(name))
        if expected:
            raise RuntimeError(f'incomplete Ubuntu source: {key}')
    elif owner.startswith('mingw-w64-ucrt-x86_64-'):
        base = owner.removeprefix('mingw-w64-ucrt-x86_64-')
        base = {'gcc-libs': 'gcc', 'gettext-runtime': 'gettext', 'libwinpthread': 'winpthreads'}.get(base, base)
        name = f'mingw-w64-{base}-{metadata["version"]}.src.tar.zst'
        source = save('https://repo.msys2.org/mingw/sources/' + name)
        process = subprocess.Popen(['zstd', '-dc', str(source)], stdout=subprocess.PIPE)
        try:
            with tarfile.open(fileobj=process.stdout, mode='r|') as archive:
                names = []
                for member in archive:
                    if member.isfile():
                        names.append(member.name)
                upstream = any(n.endswith(('.tar.gz', '.tar.xz', '.tar.bz2', '.tar.lz', '.zip')) for n in names)
                # VCS packages carry a complete bare repository instead of a tarball.
                upstream = upstream or (any(n.endswith('/HEAD') for n in names) and any('/objects/pack/' in n and n.endswith('.pack') for n in names))
                if not any(n.endswith('/PKGBUILD') for n in names) or not upstream:
                    raise RuntimeError(f'MSYS2 source archive lacks recipe/upstream source: {key}')
        finally:
            process.stdout.close()
            if process.wait() != 0:
                raise RuntimeError(f'invalid MSYS2 source archive: {key}')
    elif owner == 'FreeRDP-3.25.0':
        # The build recipe verifies this same upstream archive.
        recipe = Path('scripts/build-freerdp.sh').read_text()
        digest = re.search(r'^sha256=([0-9a-f]{64})$', recipe, re.M)
        if not digest:
            digest = re.search(r'\b([0-9a-f]{64})\b', recipe)
        save('https://codeload.github.com/FreeRDP/FreeRDP/tar.gz/refs/tags/3.25.0', digest[1], 'FreeRDP-3.25.0.tar.gz')
    else:
        if not metadata.get('source_sha256'):
            raise RuntimeError(f'no verified upstream source for {key}')
        save(metadata['source_url'], metadata['source_sha256'])
        if owner in recipes:
            recipe = recipes[owner]
            installed = directory / 'installed-formula.rb'
            installed.write_text(recipe)
            files.append({'file': str(installed.relative_to(output)), 'origin': 'installed Homebrew formula', 'sha256': hashlib.sha256(installed.read_bytes()).hexdigest()})
            # Include version-pinned resources and URL patches without executing Ruby.
            pairs = re.findall(r'\burl "(https://[^"\n]+)"(?:(?!\n\s*(?:url|end)\b)[\s\S])*?\bsha256 "([0-9a-f]{64})"', recipe)
            for url, digest in pairs:
                if url != metadata['source_url']:
                    save(url, digest)
            local_patches = re.findall(r'\bfile "(Patches/[^"\n]+)"', recipe)
            if local_patches:
                prefix, path = brew_revision(owner, recipe, cache)
                save(prefix + path, filename='homebrew-formula.rb')
                for filename in local_patches:
                    save(prefix + filename)
        elif owner != 'gdk-pixbuf-minimal':
            raise RuntimeError(f'missing build recipe for upstream package: {owner}')
    print('SOURCE OK:', key, flush=True)
    return key, {'package': owner, 'metadata': metadata, 'files': files}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archives', nargs='+', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--cache', type=Path, default=Path('.cache/release-sources'))
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    args.output.mkdir(parents=True, exist_ok=True)
    packages, platforms, recipes = {}, {}, {}
    for archive in args.archives:
        inv = inventory(archive)
        recipes.update(brew_recipes(archive))
        keys = []
        for owner, metadata in inv['packages'].items():
            family = 'ubuntu' if metadata.get('source_version') else ('msys2' if owner.startswith('mingw-w64-') else 'upstream')
            name = metadata.get('source', owner) if family == 'ubuntu' else owner
            version = metadata.get('source_version', metadata['version'])
            key = re.sub(r'[^A-Za-z0-9._+-]', '_', f'{family}-{name}-{version}')
            packages[key] = (key, owner, metadata)
            keys.append(key)
        platforms[archive.name] = sorted(set(keys))
    result, errors = {}, []
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(collect_package, item, args.cache, args.output, recipes): key for key, item in packages.items()}
        for future in concurrent.futures.as_completed(futures):
            try:
                key, record = future.result()
                result[key] = record
            except Exception as error:
                errors.append(f'{futures[future]}: {error}')
                print('SOURCE FAILED:', errors[-1], flush=True)
    (args.output / 'index.json').write_text(json.dumps({'platforms': platforms, 'packages': result, 'errors': errors}, indent=2) + '\n')
    if errors:
        raise SystemExit('Source collection incomplete; do not publish the source bundle.')


if __name__ == '__main__':
    main()

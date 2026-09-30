#!/usr/bin/env python3
"""Offline release installation and optional glibc-only chroot smoke test."""
import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile


def run(args, env=None, ok=True):
    p = subprocess.run(list(map(str, args)), env=env, capture_output=True, text=True, timeout=60)
    assert (p.returncode == 0) == ok, (args, p.stdout, p.stderr)
    return p


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archive', type=Path)
    parser.add_argument('--chroot', action='store_true', help='requires passwordless sudo; creates a temporary root')
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='rcu-package-') as tmp:
        tmp = Path(tmp)
        with tarfile.open(args.archive) as archive:
            archive.extractall(tmp / 'unpacked', filter='data')
        package = next((tmp / 'unpacked').iterdir())
        home = tmp / 'home'
        home.mkdir()
        env = os.environ.copy()
        for key in list(env):
            if key.startswith(('REMOTE_COMPUTER_', 'XDG_', 'LD_')) or key in ('DISPLAY', 'WAYLAND_DISPLAY'):
                del env[key]
        env['HOME'] = str(home)
        env['XDG_CACHE_HOME'] = str(home / 'cache')
        env['XDG_CONFIG_HOME'] = str(home / 'config')
        ctl = package / 'bin/remote-computer-ctl'
        # All commands can be moved independently of the source tree or libraries.
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(lambda _: run([ctl, 'doctor', '--json'], env), range(4)))
        for result in results:
            report = json.loads(result.stdout)
            assert report['ok'] and report['runtime'] == 'embedded', report
        library_dir = next(p for p in (home / 'cache/remote-computer-use/native').iterdir() if p.is_dir())
        assert len(list(library_dir.iterdir())) >= 2
        # Explicit override wins, and failures do not leak a local path in JSON.
        failure = run([ctl, 'doctor', '--json'], env | {'REMOTE_COMPUTER_LIB_DIR': str(tmp / 'private-missing')}, ok=False)
        assert 'private-missing' not in failure.stdout + failure.stderr
        assert not json.loads(failure.stdout)['ok']
        cache = next(library_dir.iterdir())
        original = cache.read_bytes()
        cache.write_bytes(b'corrupt')
        assert not json.loads(run([ctl, 'doctor', '--json'], env, ok=False).stdout)['ok']
        cache.write_bytes(original)
        # Test default installation, paths containing spaces, config preservation,
        # idempotence, optional Skills, and conflicts before command replacement.
        prefix = home / 'install path'
        bindir = home / 'bin path'
        install = [package / 'install.sh', '--prefix', prefix, '--bin-dir', bindir]
        run([package / 'install.sh'], env)
        assert (home / '.local/bin/remote-computer-ctl').is_symlink()
        run(install + ['--agent', 'codex'], env)
        config = home / '.remote-computer-use/config.toml'
        assert stat.S_IMODE(config.stat().st_mode) == 0o600
        assert config.read_bytes() == (package / 'config.example.toml').read_bytes()
        config.write_text('version = 1\n# preserve this existing private configuration\n')
        saved = config.read_bytes()
        for agent in ('codex', 'opencode', 'claude-code'):
            run(install + ['--agent', agent], env)
            assert config.read_bytes() == saved
        assert (home / '.agents/skills/remote-computer-use/SKILL.md').is_file()
        assert (home / 'config/opencode/skills/remote-computer-use/SKILL.md').is_file()
        assert (home / '.claude/skills/remote-computer-use/SKILL.md').is_file()
        assert (prefix / 'current').is_symlink()
        assert json.loads(run([bindir / 'remote-computer-ctl', 'doctor', '--json'], env).stdout)['ok']
        old = (bindir / 'remote-computer-ctl').resolve().read_bytes()
        skill = home / '.agents/skills/remote-computer-use/SKILL.md'
        skill.write_text('customized skill')
        run(install + ['--agent', 'codex'], env, ok=False)
        assert skill.read_text() == 'customized skill'
        skill.unlink()
        skill.symlink_to(package / 'skills/remote-computer-use/SKILL.md')
        run(install + ['--agent', 'codex'], env, ok=False)
        conflict = home / 'conflict'
        conflict.mkdir()
        (conflict / 'remote-computer-ctl').write_text('unrelated command')
        run([package / 'install.sh', '--prefix', prefix, '--bin-dir', conflict], env, ok=False)
        assert (conflict / 'remote-computer-ctl').read_text() == 'unrelated command'
        assert (bindir / 'remote-computer-ctl').resolve().read_bytes() == old
        if args.chroot:
            clean = tmp / 'root'
            clean.mkdir(mode=0o755)
            (clean / 'bin').mkdir()
            (clean / 'home/test').mkdir(parents=True)
            for binary in (package / 'bin').iterdir():
                shutil.copy2(binary, clean / 'bin' / binary.name)
            # The clean root contains only host glibc, its loader, and the CLIs.
            # No /usr, FreeRDP, libgvnc, OpenSSL or build tree is mounted into it.
            paths = run(['dpkg-query', '-L', 'libc6:amd64']).stdout.splitlines()
            for path in map(Path, paths):
                if path.is_file() and ('.so' in path.name or path.name.startswith('ld-linux')):
                    target = clean / str(path).lstrip('/')
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(path, target)
                    target.chmod(0o755)
            report = run(['sudo', '-n', 'env', 'HOME=/home/test', 'XDG_CACHE_HOME=/home/test/cache',
                          'chroot', f'--userspec={os.getuid()}:{os.getgid()}', clean,
                          '/bin/remote-computer-ctl', 'doctor', '--json'])
            assert json.loads(report.stdout)['ok'], report.stdout
            print('PASS: both embedded adapters load inside glibc-only chroot')
        print('PASS: offline install, moved CLIs, concurrent extraction, corruption rejection, config preservation, Skills and conflicts')


if __name__ == '__main__':
    main()

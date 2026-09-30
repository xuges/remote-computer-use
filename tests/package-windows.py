#!/usr/bin/env python3
"""Windows package smoke test without MSYS2/Go directories on runtime PATH."""
import base64
import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile


def run(args, env, ok=True):
    p = subprocess.run(list(map(str, args)), env=env, capture_output=True, timeout=90)
    assert (p.returncode == 0) == ok, (args, p.stdout, p.stderr)
    return p


def ps(script, env):
    encoded = base64.b64encode(script.encode('utf-16le')).decode('ascii')
    return run(['powershell.exe', '-NoProfile', '-NonInteractive', '-EncodedCommand', encoded], env)


def quote(path):
    return "'" + str(path).replace("'", "''") + "'"


def main():
    with tempfile.TemporaryDirectory(prefix='rcu package 中文 ') as temporary:
        tmp = Path(temporary)
        with zipfile.ZipFile(sys.argv[1]) as archive:
            archive.extractall(tmp / 'unpacked')
        package = next((tmp / 'unpacked').iterdir())
        profile = tmp / 'profile'
        profile.mkdir()
        env = {k: v for k, v in os.environ.items() if not k.startswith(('REMOTE_COMPUTER_', 'MSYS', 'MINGW'))}
        env['USERPROFILE'] = str(profile)
        env['REMOTE_COMPUTER_CACHE_DIR'] = str(tmp / 'cache')
        env['PATH'] = str(Path(env['SYSTEMROOT']) / 'System32') + ';' + str(Path(env['SYSTEMROOT']) / 'System32/WindowsPowerShell/v1.0')
        ctl = package / 'bin/remote-computer-ctl.exe'
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            reports = list(pool.map(lambda _: run([ctl, 'doctor', '--json'], env), range(4)))
        for result in reports:
            report = json.loads(result.stdout)
            assert report['ok'] and report['runtime'] == 'embedded', report
        cache = next((tmp / 'cache').iterdir())
        library = next(cache.iterdir())
        original = library.read_bytes()
        library.write_bytes(b'corrupt')
        assert not json.loads(run([ctl, 'doctor', '--json'], env, False).stdout)['ok']
        library.write_bytes(original)
        prefix = tmp / 'install 中文 path'
        install = "$ErrorActionPreference='Stop'; & " + quote(package / 'install.ps1') + ' -Prefix ' + quote(prefix)
        ps(install, env)
        config = profile / '.remote-computer-use/config.toml'
        assert config.is_file(), 'installer ignored isolated user profile'
        saved = config.read_bytes()
        for agent in ('codex', 'opencode', 'claude-code'):
            ps(install + ' -Agent ' + agent, env)
            assert config.read_bytes() == saved
        report = json.loads(ps('& ' + quote(prefix / 'bin/remote-computer-ctl.cmd') + ' doctor --json; exit $LASTEXITCODE', env).stdout)
        assert report['ok'], report
        # The portable fixture includes worker startup, Unicode and cancellation.
        fixture = Path(sys.argv[2]).resolve()
        run([fixture, '--bin', package / 'bin'], env)
        print('PASS: Windows self-contained runtime, private concurrent cache, corruption rejection, installer/reinstall, Unicode paths, config preservation, Skills, native VNC lifecycle')


if __name__ == '__main__':
    main()

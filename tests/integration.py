#!/usr/bin/env python3
"""Real VNC loopback test; creates only temporary servers, config and desktop."""
import io
import json
import os
from pathlib import Path
import socket
import ssl
import hashlib
import sys
import re
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
BIN = Path(os.environ.get('RCU_BIN_DIR', ROOT / 'bin'))
CTL = BIN / 'remote-computer-ctl'
USE = BIN / 'remote-computer-use'


def main():
    from PIL import Image
    children = []
    with tempfile.TemporaryDirectory(prefix='rcu-test-') as tmp:
        tmp = Path(tmp)
        env = os.environ.copy()
        env.pop('DISPLAY', None)
        env.pop('WAYLAND_DISPLAY', None)
        env['REMOTE_COMPUTER_RUN_DIR'] = str(tmp / 'run')
        env['REMOTE_COMPUTER_CONFIG'] = str(tmp / 'config.toml')
        env['RCU_TEST_SECRET'] = 'testpass'

        def call(binary, *args, ok=True, input=None):
            p = subprocess.run([str(binary), *map(str, args)], env=env, input=input, capture_output=True, timeout=40)
            assert b'testpass' not in p.stdout + p.stderr
            assert b'PRIVATE-USER' not in p.stdout + p.stderr
            if ok is True:
                assert p.returncode == 0, (args, p.stdout.decode(errors='replace')[:1000], p.stderr.decode(errors='replace')[:1000])
            elif ok is False:
                assert p.returncode != 0, args
            return p

        try:
            port_socket = socket.socket()
            port_socket.bind(('127.0.0.1', 0))
            port = port_socket.getsockname()[1]
            port_socket.close()
            display = next(n for n in range(220, 1000) if not Path(f'/tmp/.X11-unix/X{n}').exists() and not Path(f'/tmp/.X{n}-lock').exists())
            passwd = subprocess.run(['tigervncpasswd', '-f'], input=b'testpass\n', capture_output=True, check=True).stdout
            (tmp / 'passwd').write_bytes(passwd)
            (tmp / 'passwd').chmod(0o600)
            server_log = (tmp / 'server.log').open('wb')
            server = subprocess.Popen(['Xtigervnc', f':{display}', '-rfbport', str(port), '-localhost', '-SecurityTypes', 'VncAuth', '-rfbauth', str(tmp / 'passwd'), '-geometry', '640x480', '-depth', '24', '-ac', '-nolisten', 'tcp'], stdout=server_log, stderr=server_log)
            children.append(server)
            desktop_env = env | {'DISPLAY': f':{display}'}
            for _ in range(100):
                if Path(f'/tmp/.X11-unix/X{display}').exists():
                    break
                assert server.poll() is None, 'temporary VNC server failed'
                time.sleep(.05)
            events = (tmp / 'events').open('wb')
            desktop = subprocess.Popen([str(ROOT / 'build/test-desktop')], env=desktop_env, stdout=events, stderr=subprocess.DEVNULL)
            children.append(desktop)
            time.sleep(.2)
            config = f'''version = 1
[connections.test]
protocol = "vnc"
host = "127.0.0.1"
port = {port}
username = "PRIVATE-USER"
password_env = "RCU_TEST_SECRET"
allow_unencrypted = true
[connections.bad]
protocol = "vnc"
host = "127.0.0.1"
port = {port}
password = "wrong-password"
allow_unencrypted = true
'''
            Path(env['REMOTE_COMPUTER_CONFIG']).write_text(config)
            Path(env['REMOTE_COMPUTER_CONFIG']).chmod(0o600)
            call(CTL, 'connect', 'test', '--json')
            call(CTL, 'connect', 'test', '--json')  # idempotence
            state = json.loads(call(CTL, 'status', 'test', '--json').stdout)
            assert state['state'] == 'ready' and state['width'] == 640 and state['height'] == 480
            assert not {'host', 'port', 'username', 'password'} & state.keys()
            data = call(USE, '--conn', 'test', 'screenshot', '--output', '-').stdout
            im = Image.open(io.BytesIO(data)).convert('RGB')
            assert im.size == (640, 480)
            assert im.getpixel((100, 100)) == (255, 0, 0), im.getpixel((100, 100))
            assert im.getpixel((400, 100)) == (0, 255, 0)
            assert im.getpixel((100, 300)) == (0, 0, 255)
            call(USE, '--conn', 'test', 'click', '--x', '123', '--y', '234', '--count', '2')
            call(USE, '--conn', 'test', 'key', 'CTRL+L')
            call(USE, '--conn', 'test', 'type', '--stdin', input=b'Hi\n')
            call(USE, '--conn', 'test', 'scroll', '--dy', '2')
            call(USE, '--conn', 'test', 'drag', '--from', '30,40', '--to', '90,100', '--duration', '100ms')
            call(USE, '--conn', 'test', 'move', '--x', '640', '--y', '0', ok=False)
            call(USE, '--conn', 'test', 'move', '--x', '10', '--y', '10', '--geometry-version', str(state['geometry_version'] + 1), ok=False)
            time.sleep(.15)
            text = (tmp / 'events').read_text()
            assert text.count('button down 1 123 234') == 2, text
            assert 'key down 65507' in text and 'key up 65507' in text, text
            assert 'button up 1 90 100' in text, text
            assert 'button down 5' in text, text
            data = call(USE, '--conn', 'test', 'screenshot', '--output', '-').stdout
            assert Image.open(io.BytesIO(data)).convert('RGB').getpixel((15, 15)) == (255, 255, 0)
            # Closing the requesting process must release a button already sent remotely.
            before = len((tmp / 'events').read_text())
            dragging = subprocess.Popen([str(USE), '--conn', 'test', 'drag', '--from', '50,60', '--to', '300,300', '--duration', '5s'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            children.append(dragging)
            for _ in range(100):
                if 'button down 1 50 60' in (tmp / 'events').read_text()[before:]:
                    break
                time.sleep(.02)
            assert 'button down 1 50 60' in (tmp / 'events').read_text()[before:]
            dragging.terminate()
            dragging.wait(timeout=5)
            for _ in range(100):
                if 'button up 1' in (tmp / 'events').read_text()[before:]:
                    break
                time.sleep(.02)
            assert 'button up 1' in (tmp / 'events').read_text()[before:]
            # Resize the actual protocol desktop, then reject coordinates from the old frame.
            subprocess.run(['xrandr', '--fb', '800x600'], env=desktop_env, capture_output=True, check=True)
            for _ in range(100):
                resized = json.loads(call(CTL, 'status', 'test', '--json', ok=None).stdout)
                if resized['state'] == 'ready' and resized['width'] == 800 and resized['height'] == 600:
                    break
                time.sleep(.02)
            assert resized['width'] == 800 and resized['height'] == 600, resized
            assert resized['geometry_version'] > state['geometry_version']
            result = json.loads(call(USE, '--conn', 'test', 'click', '--x', '10', '--y', '10', '--geometry-version', str(state['geometry_version']), '--json', ok=False).stdout)
            assert result['error'] == 'GEOMETRY_CHANGED', result
            assert Image.open(io.BytesIO(call(USE, '--conn', 'test', 'screenshot', '--output', '-').stdout)).size == (800, 600)
            subprocess.run(['xrandr', '--fb', '640x480'], env=desktop_env, capture_output=True, check=True)
            if '--rdp' in sys.argv:
                shadow = Path(os.environ.get('RCU_SHADOW_SERVER', ROOT / '.cache/freerdp/bin/freerdp-shadow-cli'))
                assert shadow.exists(), 'set RCU_SHADOW_SERVER to a FreeRDP 3 shadow test server'
                listener = socket.socket()
                listener.bind(('127.0.0.1', 0))
                rdp_port = listener.getsockname()[1]
                listener.close()
                shadow_env = desktop_env | {'XDG_CONFIG_HOME': str(tmp / 'xdg')}
                shadow_log = (tmp / 'shadow.log').open('wb')
                rdp_server = subprocess.Popen([str(shadow), f'/port:{rdp_port}', '/bind-address:127.0.0.1', '-auth', '/sec:tls', '-rfx', '-nsc', '-gfx', '/log-level:WARN'], env=shadow_env, stdout=shadow_log, stderr=shadow_log)
                children.append(rdp_server)
                for _ in range(100):
                    certs = list((tmp / 'xdg').rglob('shadow.crt'))
                    pem = re.search(r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', certs[0].read_text(), re.S) if certs else None
                    if pem:
                        break
                    assert rdp_server.poll() is None, (tmp / 'shadow.log').read_text()[-2000:]
                    time.sleep(.1)
                assert pem, 'shadow certificate not generated in test directory'
                fingerprint = hashlib.sha256(ssl.PEM_cert_to_DER_cert(pem.group())).hexdigest()
                extra = f'''\n[connections.rdp]
protocol = "rdp"
host = "127.0.0.1"
port = {rdp_port}
username = "PRIVATE-USER"
password_env = "RCU_TEST_SECRET"
certificate_sha256 = "{fingerprint}"
width = 640
height = 480
[connections.untrusted]
protocol = "rdp"
host = "127.0.0.1"
port = {rdp_port}
username = "PRIVATE-USER"
password_env = "RCU_TEST_SECRET"
certificate_sha256 = "{'0' * 64}"
[connections.unverified]
protocol = "rdp"
host = "127.0.0.1"
port = {rdp_port}
username = "PRIVATE-USER"
password_env = "RCU_TEST_SECRET"
tls_verify = false
[connections.systemtrust]
protocol = "rdp"
host = "127.0.0.1"
port = {rdp_port}
username = "PRIVATE-USER"
password_env = "RCU_TEST_SECRET"
'''
                with Path(env['REMOTE_COMPUTER_CONFIG']).open('a') as f:
                    f.write(extra)
                time.sleep(.3)
                rejected = json.loads(call(CTL, 'connect', 'untrusted', '--json', ok=False).stdout)
                assert rejected['error'] == 'TLS_VERIFY_FAILED', rejected
                rejected = json.loads(call(CTL, 'connect', 'systemtrust', '--json', ok=False).stdout)
                assert rejected['error'] == 'TLS_VERIFY_FAILED', rejected
                call(CTL, 'connect', 'unverified', '--json')
                Image.open(io.BytesIO(call(USE, '--conn', 'unverified', 'screenshot', '--output', '-').stdout)).load()
                call(CTL, 'disconnect', 'unverified', '--json')
                call(CTL, 'connect', 'rdp', '--json')
                for _ in range(30):
                    data = call(USE, '--conn', 'rdp', 'screenshot', '--output', '-').stdout
                    rim = Image.open(io.BytesIO(data)).convert('RGB')
                    if rim.getpixel((100, 100)) == (255, 0, 0):
                        break
                    time.sleep(.1)
                assert rim.size == (640, 480), rim.size
                assert rim.getpixel((100, 100)) == (255, 0, 0), rim.getpixel((100, 100))
                assert rim.getpixel((100, 300)) == (0, 0, 255), rim.getpixel((100, 300))
                call(USE, '--conn', 'rdp', 'click', '--x', '222', '--y', '111')
                call(USE, '--conn', 'rdp', 'key', 'CTRL+A')
                call(USE, '--conn', 'rdp', 'type', '--text', 'Hello')
                time.sleep(.2)
                assert 'button down 1 222 111' in (tmp / 'events').read_text()
                call(CTL, 'disconnect', 'rdp', '--json')
                rdp_server.terminate()
                rdp_server.wait(timeout=5)
                nt_hash = subprocess.run(['openssl', 'dgst', '-md4', '-provider', 'legacy', '-binary'], input='testpass'.encode('utf-16le'), capture_output=True, check=True).stdout.hex()
                sam = tmp / 'test.sam'
                sam.write_text(f'PRIVATE-USER:::{nt_hash}:\n')
                sam.chmod(0o600)
                rdp_server = subprocess.Popen([str(shadow), f'/port:{rdp_port}', '/bind-address:127.0.0.1', f'/sam-file:{sam}', '/sec:nla', '-rfx', '-nsc', '-gfx', '/log-level:WARN'], env=shadow_env, stdout=shadow_log, stderr=shadow_log)
                children.append(rdp_server)
                time.sleep(.3)
                with Path(env['REMOTE_COMPUTER_CONFIG']).open('a') as f:
                    f.write(f'''\n[connections.rdpauthbad]
protocol = "rdp"
host = "127.0.0.1"
port = {rdp_port}
username = "PRIVATE-USER"
password = "wrong-password"
certificate_sha256 = "{fingerprint}"
''')
                auth = json.loads(call(CTL, 'connect', 'rdpauthbad', '--json', ok=False).stdout)
                assert auth['error'] == 'AUTH_FAILED', auth
                call(CTL, 'connect', 'unverified', '--json')
                call(CTL, 'disconnect', 'unverified', '--json')
                call(CTL, 'connect', 'rdp', '--json')
                call(USE, '--conn', 'rdp', 'screenshot', '--output', '-')
                call(CTL, 'disconnect', 'rdp', '--json')
                print('PASS: real headless RDP, default certificate validation, pinning/rejection, explicit verification skip, NLA authentication/rejection, RGB screenshot, pointer and keyboard')
            call(CTL, 'connect', '--all', '--json', ok=False)
            call(CTL, 'status', 'test', '--json')  # batch failure did not roll back
            server.terminate()
            server.wait(timeout=5)
            time.sleep(.2)
            call(USE, '--conn', 'test', 'click', '--x', '10', '--y', '10', ok=False)
            call(CTL, 'disconnect', '--all', '--json')
            call(CTL, 'disconnect', 'test', '--json')
            assert not list((tmp / 'run').glob('*.sock'))
            print('PASS: real VNC authentication, headless RGB screenshots, input events, incremental frames, resize/geometry guard, cancellation, batch failure, disconnect and redaction')
        finally:
            subprocess.run([str(CTL), 'disconnect', '--all', '--json'], env=env, capture_output=True, timeout=15)
            for p in reversed(children):
                if p.poll() is None:
                    p.terminate()
                    try:
                        p.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        p.kill()
                        p.wait()


if __name__ == '__main__':
    main()

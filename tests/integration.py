#!/usr/bin/env python3
"""Real VNC loopback test; creates only temporary servers, config and desktop."""
import io
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
CTL = ROOT / 'bin/remote-computer-ctl'
USE = ROOT / 'bin/remote-computer-use'


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
            if ok:
                assert p.returncode == 0, (args, p.stdout.decode(errors='replace')[:1000], p.stderr.decode(errors='replace')[:1000])
            else:
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
            call(CTL, 'connect', '--all', '--json', ok=False)
            call(CTL, 'status', 'test', '--json')  # batch failure did not roll back
            server.terminate()
            server.wait(timeout=5)
            time.sleep(.2)
            call(USE, '--conn', 'test', 'click', '--x', '10', '--y', '10', ok=False)
            call(CTL, 'disconnect', '--all', '--json')
            call(CTL, 'disconnect', 'test', '--json')
            assert not list((tmp / 'run').glob('*.sock'))
            print('PASS: real VNC authentication, headless RGB screenshots, input events, incremental frames, geometry guard, batch failure, disconnect and redaction')
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

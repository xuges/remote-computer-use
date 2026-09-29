---
name: remote-computer-use
description: Observe and operate remote desktops over VNC or RDP using screenshots, mouse and keyboard. Use for remote GUI tasks; remote shell commands and file transfers belong to remote-shell.
---

# Remote computer use

Run the local `remote-computer-ctl` and `remote-computer-use` executables. If absent from PATH, check `~/.remote-computer-use/bin/`. They maintain background desktop sessions without a local display. The remote user may still see actions. No SSH connection or remote helper is needed.

## Select and connect

```sh
remote-computer-ctl list --json
remote-computer-ctl connect desktop --json
remote-computer-ctl status desktop --json
```

Use the user's selected alias. If several aliases match and context does not identify one, clarify the target before acting. `ready` means a frame is available, not that the desktop is unlocked. Do not print the private config or native logs to discover targets; `list` exposes safe aliases and states. Missing configuration or authentication failures need configuration repair, not credential guessing.

## Observe, act, verify

```sh
remote-computer-use --conn desktop screenshot --output /tmp/desktop.png --json
remote-computer-use --conn desktop click --x 300 --y 200 --geometry-version 1 --json
remote-computer-use --conn desktop key CTRL+L --json
remote-computer-use --conn desktop type --text 'https://example.com' --json
remote-computer-use --conn desktop key ENTER --json
```

Open the actual PNG with the agent's image viewing tool before choosing coordinates. Use the returned `width`, `height`, and `geometry_version`; the `1` above is only an example. Coordinates are pixels in the original PNG, origin at its top left. If the viewer scales the image, map coordinates back to the original dimensions. Capture again after actions to verify the application outcome. Command success only acknowledges input delivery. Screenshot files are local to the machine running the CLI.

Available actions:

| Action | Arguments |
| --- | --- |
| Screenshot | `screenshot --output PATH` (PNG; `--output -` for binary stdout, without `--json`) |
| Move | `move --x X --y Y` |
| Click/double click | `click --x X --y Y --button left\|middle\|right --count 1\|2` |
| Scroll | `scroll --dy 3` (positive down), `--dx 3` (positive right); units are notches |
| Drag | `drag --from X,Y --to X,Y --duration 500ms` |
| Shortcut | `key CTRL+SHIFT+S`; use `CMD` on macOS, `WIN` on Windows |
| Text | `type --text TEXT` or `type --stdin` |

Every action requires `--conn ALIAS`. Common options: `--json`, `--timeout 10s`, `--geometry-version N`. There is no arbitrary shell execution command. Text uses protocol key events, including Unicode where the server supports it; clipboard mode is currently unsupported. For sensitive text, use stdin from an existing private source without echoing its contents or placing it in a shell command line.

On `GEOMETRY_CHANGED`, take a new screenshot before recomputing coordinates. On timeout or connection loss, inspect status and obtain a fresh screenshot before deciding whether to repeat an action; part of the previous action may already have happened. Stop repeated attempts when the same error persists and report the safe error code. Do not automatically weaken TLS verification or enable unencrypted transport.

## Finish

Disconnect connections created solely for the task when finished, unless they are intended to stay open. Keep pre-existing user sessions available.

```sh
remote-computer-ctl disconnect desktop --json
```

`connect --all` and `disconnect --all` affect every configured/running connection; use them when the user requests that scope. Disconnect closes this client session and does not explicitly log out the remote user. `remote-shell`, if separately available, complements this skill for commands and file transfer; it does not share this desktop session automatically.

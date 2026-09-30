#!/usr/bin/env bash
# Offline installer distributed at the root of a release archive.
set -euo pipefail
umask 077
release_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
prefix=${HOME:?}/.remote-computer-use
bin_dir=$HOME/.local/bin
agent=
usage() { echo 'Usage: ./install.sh [--prefix /absolute/path] [--bin-dir /absolute/path] [--agent codex|opencode|claude-code]'; }
while (( $# )); do
    case $1 in
        --prefix|--bin-dir|--agent)
            (( $# >= 2 )) || { usage >&2; exit 2; }
            case $1 in --prefix) prefix=$2;; --bin-dir) bin_dir=$2;; --agent) agent=$2;; esac
            shift 2;;
        --help) usage; exit 0;;
        *) usage >&2; exit 2;;
    esac
done
[[ $prefix == /* && $bin_dir == /* ]] || { echo 'Installation paths must be absolute.' >&2; exit 2; }
skill_dir=
case $agent in
    '') ;;
    codex) skill_dir=$HOME/.agents/skills/remote-computer-use;;
    opencode) skill_dir=${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/remote-computer-use;;
    claude-code) skill_dir=$HOME/.claude/skills/remote-computer-use;;
    *) usage >&2; exit 2;;
esac
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'This package requires Linux x86_64 (glibc 2.35+).' >&2; exit 1; }
for name in remote-computer-ctl remote-computer-use; do
    [[ -x $release_dir/bin/$name ]] || { echo 'Run install.sh from an extracted release package.' >&2; exit 1; }
    target=$bin_dir/$name
    if [[ $bin_dir != "$prefix/bin" && ( -e $target || -L $target ) ]]; then
        [[ -L $target && $(readlink -- "$target") == "$prefix/bin/$name" ]] || { echo "Refusing to replace existing command: $target" >&2; exit 1; }
    fi
done
if [[ -n $skill_dir && ( -e $skill_dir || -L $skill_dir ) ]]; then
    [[ ! -L $skill_dir && ! -L $skill_dir/SKILL.md && -f $skill_dir/SKILL.md ]] && cmp -s "$release_dir/skills/remote-computer-use/SKILL.md" "$skill_dir/SKILL.md" || { echo "Existing Skill differs; preserve or remove it before reinstalling: $skill_dir" >&2; exit 1; }
fi
# Check the bundled runtime, not any development override from the caller.
REMOTE_COMPUTER_LIB_DIR= "$release_dir/bin/remote-computer-ctl" doctor
version=$("$release_dir/bin/remote-computer-ctl" --version)
[[ $("$release_dir/bin/remote-computer-use" --version) == "$version" ]] || { echo 'CLI versions differ.' >&2; exit 1; }
identity=$(sha256sum "$release_dir/bin/remote-computer-ctl" "$release_dir/bin/remote-computer-use" | cut -d ' ' -f 1 | sha256sum | cut -d ' ' -f 1)
mkdir -p -- "$prefix/releases" "$prefix/bin" "$bin_dir"
stage=$(mktemp -d "$prefix/.install.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
mkdir "$stage/bin"
install -m 755 "$release_dir/bin/remote-computer-ctl" "$release_dir/bin/remote-computer-use" "$stage/bin/"
cp -R "$release_dir/THIRD_PARTY_NOTICES" "$stage/"
cp "$release_dir/config.example.toml" "$stage/"
if [[ ! -e $prefix/releases/$identity ]]; then
    mv -- "$stage" "$prefix/releases/$identity"
    stage=$(mktemp -d "$prefix/.install.XXXXXX")
else
    # Never trust an interrupted or modified previous installation.
    cmp -s "$stage/bin/remote-computer-ctl" "$prefix/releases/$identity/bin/remote-computer-ctl"
    cmp -s "$stage/bin/remote-computer-use" "$prefix/releases/$identity/bin/remote-computer-use"
fi
ln -s "releases/$identity" "$stage/current"
mv -Tf -- "$stage/current" "$prefix/current"
for name in remote-computer-ctl remote-computer-use; do
    ln -s "../current/bin/$name" "$stage/$name"
    mv -Tf -- "$stage/$name" "$prefix/bin/$name"
    if [[ $bin_dir != "$prefix/bin" ]]; then
        ln -sfn -- "$prefix/bin/$name" "$bin_dir/$name"
    fi
done
config_dir=$HOME/.remote-computer-use
mkdir -p -- "$config_dir"
if [[ ! -e $config_dir/config.toml && ! -L $config_dir/config.toml ]]; then
    (set -C; cat "$release_dir/config.example.toml" > "$config_dir/config.toml")
    echo "Created configuration template: $config_dir/config.toml"
else
    echo 'Existing configuration preserved.'
fi
if [[ -n $skill_dir ]]; then
    mkdir -p -- "$skill_dir"
    if [[ ! -e $skill_dir/SKILL.md ]]; then
        install -m 644 "$release_dir/skills/remote-computer-use/SKILL.md" "$skill_dir/SKILL.md"
    fi
    echo "Skill installed: $skill_dir"
fi
echo "Installed $version in $prefix"
echo "Edit $config_dir/config.toml, then run remote-computer-ctl connect NAME."
case :$PATH: in
    *:"$bin_dir":*) ;;
    *) printf 'Add this directory to your shell/Agent PATH: export PATH=%q:"$PATH"\n' "$bin_dir";;
esac

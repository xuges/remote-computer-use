#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
version=${VERSION:-dev}
[[ $version =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { echo 'Invalid VERSION' >&2; exit 2; }
case $(uname -s) in Linux) target_os=linux;; Darwin) target_os=darwin;; MINGW*|MSYS*) target_os=windows;; *) exit 2;; esac
case $(uname -m) in x86_64|amd64) target_arch=amd64;; arm64|aarch64) target_arch=arm64;; *) exit 2;; esac
exe=
[[ $target_os != windows ]] || exe=.exe
mkdir -p build dist
stage=$(mktemp -d "$root/build/release.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
name=remote-computer-use-${version}-${target_os}-${target_arch}
package=$stage/$name
mkdir -p "$package/bin" "$package/skills"
python3 scripts/bundle-runtime.py --notices "$package/THIRD_PARTY_NOTICES"
for command in remote-computer-ctl remote-computer-use; do
    CGO_ENABLED=0 GOOS=$target_os GOARCH=$target_arch "${GO:-go}" build -tags embedded -trimpath \
        -ldflags "-s -w -X github.com/xuges/remote-computer-use/internal/app.Version=$version" \
        -o "$package/bin/$command$exe" "./cmd/$command"
done
# Native Go on Windows needs a Windows absolute cache path.
cache=$stage/cache
[[ $target_os != windows ]] || cache=$(cygpath -w "$cache")
REMOTE_COMPUTER_LIB_DIR= REMOTE_COMPUTER_CACHE_DIR="$cache" "$package/bin/remote-computer-ctl$exe" doctor
cp scripts/install.sh scripts/install.ps1 "$package/"
cp config.example.toml README.md "$package/"
cp internal/runtimebundle/payload/manifest.json "$package/runtime-manifest.json"
cp -R docs "$package/docs"
cp -R skills/remote-computer-use "$package/skills/"
python3 - "$stage" "$name" "$target_os" <<'PY'
import hashlib, pathlib, shutil, sys
stage, name, target = sys.argv[1:]
fmt = 'zip' if target == 'windows' else 'gztar'
archive = pathlib.Path(shutil.make_archive('dist/' + name, fmt, stage, name))
archive.with_name(archive.name + '.sha256').write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name + '\n')
print('Created ' + str(archive) + ' and checksum')
PY

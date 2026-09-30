#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
version=${VERSION:-dev}
[[ $version =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { echo 'Invalid VERSION' >&2; exit 2; }
mkdir -p build dist
stage=$(mktemp -d "$root/build/release.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
name=remote-computer-use-${version}-linux-amd64
package=$stage/$name
mkdir -p "$package/bin" "$package/skills"
python3 scripts/bundle-runtime.py --notices "$package/THIRD_PARTY_NOTICES"
for command in remote-computer-ctl remote-computer-use; do
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "${GO:-go}" build -tags embedded -trimpath \
        -ldflags "-s -w -X github.com/xuges/remote-computer-use/internal/app.Version=$version" \
        -o "$package/bin/$command" "./cmd/$command"
done
REMOTE_COMPUTER_LIB_DIR= REMOTE_COMPUTER_CACHE_DIR="$stage/cache" "$package/bin/remote-computer-ctl" doctor
cp scripts/install.sh "$package/install.sh"
cp config.example.toml README.md "$package/"
cp -R docs "$package/docs"
cp -R skills/remote-computer-use "$package/skills/"
# Preserve executable bits; the archive contains no connection credentials.
tar -C "$stage" -czf "dist/$name.tar.gz" "$name"
(cd dist && sha256sum "$name.tar.gz" > "$name.tar.gz.sha256")
echo "Created dist/$name.tar.gz and checksum"

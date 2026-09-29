#!/usr/bin/env bash
set -euo pipefail

# Build the pinned runtime into a private prefix; never install system packages.
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
version=3.25.0
sha256=6779159924ba7b768cdabb262dfff092bba7ba3204d7849b326f070a44d16466
prefix=${FREERDP_PREFIX:-"$root/.cache/freerdp"}
build_dir=${FREERDP_BUILD_DIR:-"$root/.cache/freerdp-build"}
with_test=OFF
if [[ ${1:-} == --with-test-server && $# == 1 ]]; then
    with_test=ON
elif [[ $# != 0 ]]; then
    echo 'Usage: scripts/build-freerdp.sh [--with-test-server]' >&2
    exit 2
fi
if [[ $prefix != /* ]]; then
    echo 'FREERDP_PREFIX must be an absolute path' >&2
    exit 2
fi
mkdir -p "$root/.cache"
archive="$root/.cache/freerdp.tar.gz"
if [[ ! -f $archive ]]; then
    curl -fL --retry 3 -o "$archive.tmp" "https://codeload.github.com/FreeRDP/FreeRDP/tar.gz/refs/tags/$version"
    mv "$archive.tmp" "$archive"
fi
printf '%s  %s\n' "$sha256" "$archive" | sha256sum --check --status
source_dir="$root/.cache/FreeRDP-$version"
if [[ ! -d $source_dir ]]; then
    tar -xzf "$archive" -C "$root/.cache"
fi
cmake -S "$source_dir" -B "$build_dir" -G Ninja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_INTERPROCEDURAL_OPTIMIZATION=OFF \
    -DWITH_CLIENT=ON -DWITH_CLIENT_COMMON=ON -DWITH_CLIENT_INTERFACE=OFF \
    -DWITH_CLIENT_SDL=OFF -DWITH_WAYLAND=OFF -DWITH_X11="$with_test" \
    -DWITH_SERVER="$with_test" -DWITH_SHADOW="$with_test" \
    -DWITH_PROXY=OFF -DWITH_SAMPLE=OFF -DWITH_MANPAGES=OFF \
    -DWITH_FFMPEG=OFF -DWITH_DSP_FFMPEG=OFF -DWITH_VIDEO_FFMPEG=OFF \
    -DWITH_OPENH264=OFF -DWITH_CUPS=OFF -DWITH_PULSE=OFF -DWITH_ALSA=OFF \
    -DWITH_PCSC=OFF -DWITH_URBDRC=OFF -DWITH_AAD=OFF -DBUILD_TESTING=OFF \
    -DWITH_JSON_DISABLED=ON -DWITH_FUSE=OFF -DWITH_SWSCALE=OFF
cmake --build "$build_dir" --parallel "${JOBS:-4}"
cmake --install "$build_dir"
printf 'FreeRDP installed in %s\nBuild adapters with PKG_CONFIG_PATH=%s/lib/pkgconfig make native\n' "$prefix" "$prefix"

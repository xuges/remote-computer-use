#!/usr/bin/env bash
# Build only the in-memory pixbuf API needed by libgvnc. No image loaders.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
version=2.42.12
archive="$root/.cache/gdk-pixbuf-$version.tar.xz"
digest=b9505b3445b9a7e48ced34760c3bcb73e966df3ac94c95a148cb669ab748e3c7
mkdir -p .cache
if [[ ! -f $archive ]]; then
    curl -fL --retry 3 -o "$archive.tmp" "https://download.gnome.org/sources/gdk-pixbuf/2.42/gdk-pixbuf-$version.tar.xz"
    mv "$archive.tmp" "$archive"
fi
printf '%s  %s\n' "$digest" "$archive" | sha256sum -c -
source_dir="$root/.cache/gdk-pixbuf-$version"
if [[ ! -f $source_dir/.rcu-extracted ]]; then
    # Windows does not need symlink-based image fixtures; tests are disabled.
    tar -xf "$archive" -C "$root/.cache" --exclude="gdk-pixbuf-$version/tests/test-images"
    touch "$source_dir/.rcu-extracted"
fi
prefix="$root/.cache/gdk-pixbuf"
build="$root/.cache/gdk-pixbuf-build"
reconfigure=()
[[ ! -d $build/meson-private ]] || reconfigure=(--reconfigure)
meson setup "${reconfigure[@]}" "$build" "$source_dir" --prefix="$prefix" --buildtype=release \
        -Dpng=disabled -Djpeg=disabled -Dtiff=disabled -Dgif=disabled \
        -Dothers=disabled -Dbuiltin_loaders=none -Dnative_windows_loaders=false \
        -Dintrospection=disabled -Dgtk_doc=false -Dman=false \
        -Dtests=false -Dinstalled_tests=false
meson compile -C "$build" gdk_pixbuf-2.0
# Only the DLL is distributed; thumbnailers and image conversion tools are unused.
mkdir -p "$prefix/bin"
install -m 755 "$build/gdk-pixbuf/libgdk_pixbuf-2.0-0.dll" "$prefix/bin/"

# Third-party software

The project's own code is licensed under MIT, as stated in LICENSE. This does
not relicense the third-party software included in the self-contained packages.
The package directory `THIRD_PARTY_NOTICES/` contains their notices, license
texts, versions, and native-library inventory.

## Go code

| Component | License |
| --- | --- |
| Go standard library; golang.org/x/sys | BSD-3-Clause |
| github.com/BurntSushi/toml | MIT |
| github.com/goexlib/cgo | MIT |
| github.com/ebitengine/purego | Apache-2.0 |

Exact module and toolchain versions and original notices are collected from
the build inputs into `THIRD_PARTY_NOTICES/go/`.

## Native libraries

This product uses LGPL libraries, including libgvnc (gtk-vnc), GLib,
gdk-pixbuf, GnuTLS, and several of their dependencies. Those libraries and
their use remain covered by their applicable LGPL terms. Some dependencies,
such as GMP, Nettle, libidn2, and libunistring, offer alternative licenses;
the LGPL alternative is used for the library builds shipped here. GNU
gettext's libintl and libiconv retain their library licenses, independently
of licenses on command-line tools in the same upstream packages.

FreeRDP and OpenSSL use Apache-2.0. Other dependencies include libraries
under BSD, MIT, zlib, libpng and other permissive terms. GCC runtime libraries
retain GPL terms with the GCC Runtime Library Exception, whose original text
is included in their notices. The inventory identifies the actual libraries;
an upstream source package can also contain tools or tests with other licenses.

The Windows build uses a minimal gdk-pixbuf DLL without image loaders. It does
not bundle libtiff or JBIG. The packager rejects libjbig so an unrelated image
codec cannot silently introduce an additional GPL dependency.

## Source and modification

The release must provide `remote-computer-use-<version>-sources.tar.gz` and its
SHA-256 file alongside the binary downloads. This companion archive contains
the corresponding native source packages, distribution patches and recipes,
the project build scripts and source, and an index of source file hashes.
Ubuntu sources include the exact `.dsc` and its referenced source archives;
MSYS2 source packages include PKGBUILD, patches and upstream source; Homebrew
sources include the installed formula, matching repository recipe, resources
and patches. URLs in the inventory are provenance, not a substitute for
providing the source archive.

Native libraries are dynamically loaded after extraction, not statically
linked into the Go program. To use a modified, ABI-compatible library, place
it with its dependencies and the adapters in a separate directory and set
`REMOTE_COMPUTER_LIB_DIR` to that absolute directory before starting ctl.
Disconnect existing workers first. This override does not require the
embedded manifest's hashes, allowing the user to substitute modified libraries.
Editing the integrity-checked embedded cache is not the replacement mechanism.

The project does not prohibit modification for personal use or reverse
engineering to debug modifications to these libraries. MIT applies only to
the project's own files; original third-party notices remain intact. Packaging
changes loader paths and signatures where needed; distribution patches and
the minimal Windows library build are documented in the accompanying recipes.
FreeRDP is built with the compatibility adjustments recorded in
`scripts/build-freerdp.sh`; both the unmodified pinned upstream archive and
that build script are included in the source package.

References: [LGPL 2.1, sections 4 and 6](https://opensource.org/license/lgpl-2-1),
[LGPL 3](https://www.gnu.org/licenses/lgpl-3.0.html), and
[GCC Runtime Library Exception](https://www.gnu.org/licenses/gcc-exception-3.1.html).

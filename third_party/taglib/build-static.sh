#!/bin/bash
# file: third_party/taglib/build-static.sh
# version: 1.0.0
#
# Cross-compiles TagLib 2.0.2 + zlib as static libraries for musl-linux-amd64.
# Requires: brew install filosottile/musl-cross/musl-cross cmake
#
# Usage: ./third_party/taglib/build-static.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKDIR="/tmp/taglib-build-$$"
INSTALL_DIR="$WORKDIR/install"

MUSL_CC=x86_64-linux-musl-gcc
MUSL_CXX=x86_64-linux-musl-g++
MUSL_AR=x86_64-linux-musl-ar
MUSL_RANLIB=x86_64-linux-musl-ranlib

ZLIB_VERSION="1.3.1"
TAGLIB_VERSION="2.0.2"
UTFCPP_VERSION="4.0.6"

echo "=== Building static TagLib for musl-linux-amd64 ==="

# Check prerequisites
command -v $MUSL_CC >/dev/null || { echo "ERROR: $MUSL_CC not found. Run: brew install filosottile/musl-cross/musl-cross"; exit 1; }
command -v cmake >/dev/null || { echo "ERROR: cmake not found. Run: brew install cmake"; exit 1; }

mkdir -p "$WORKDIR" "$INSTALL_DIR"

# Build zlib
echo "--- Building zlib $ZLIB_VERSION ---"
cd "$WORKDIR"
curl -sL -o zlib.tar.gz "https://github.com/madler/zlib/releases/download/v${ZLIB_VERSION}/zlib-${ZLIB_VERSION}.tar.gz"
tar xf zlib.tar.gz
cd "zlib-${ZLIB_VERSION}"
CC=$MUSL_CC CFLAGS="-fPIC -O2" ./configure --static >/dev/null
$MUSL_CC -fPIC -O2 -c adler32.c crc32.c deflate.c infback.c inffast.c inflate.c inftrees.c trees.c zutil.c compress.c uncompr.c gzclose.c gzlib.c gzread.c gzwrite.c
$MUSL_AR rcs libz.a *.o
$MUSL_RANLIB libz.a
mkdir -p "$INSTALL_DIR/lib" "$INSTALL_DIR/include"
cp libz.a "$INSTALL_DIR/lib/"
cp zlib.h zconf.h "$INSTALL_DIR/include/"
echo "    zlib: $(wc -c < "$INSTALL_DIR/lib/libz.a") bytes"

# Get utfcpp (header-only)
echo "--- Downloading utfcpp $UTFCPP_VERSION ---"
cd "$WORKDIR"
curl -sL "https://github.com/nemtrif/utfcpp/archive/refs/tags/v${UTFCPP_VERSION}.tar.gz" | tar xz

# Build TagLib
echo "--- Building TagLib $TAGLIB_VERSION ---"
cd "$WORKDIR"
curl -sL "https://github.com/taglib/taglib/releases/download/v${TAGLIB_VERSION}/taglib-${TAGLIB_VERSION}.tar.gz" | tar xz
mkdir -p build && cd build
cmake "../taglib-${TAGLIB_VERSION}" \
  -DCMAKE_SYSTEM_NAME=Linux \
  -DCMAKE_C_COMPILER=$MUSL_CC \
  -DCMAKE_CXX_COMPILER=$MUSL_CXX \
  -DCMAKE_INSTALL_PREFIX="$INSTALL_DIR" \
  -DCMAKE_FIND_ROOT_PATH="$INSTALL_DIR" \
  -DBUILD_SHARED_LIBS=OFF \
  -DBUILD_EXAMPLES=OFF \
  -DBUILD_TESTING=OFF \
  -DWITH_ZLIB=ON \
  -DZLIB_LIBRARY="$INSTALL_DIR/lib/libz.a" \
  -DZLIB_INCLUDE_DIR="$INSTALL_DIR/include" \
  -Dutf8cpp_INCLUDE_DIR="$WORKDIR/utfcpp-${UTFCPP_VERSION}/source" \
  -DCMAKE_C_FLAGS="-fPIC" \
  -DCMAKE_CXX_FLAGS="-fPIC" \
  >/dev/null 2>&1
make -j"$(sysctl -n hw.ncpu 2>/dev/null || nproc)" >/dev/null 2>&1
make install >/dev/null 2>&1
echo "    libtag.a: $(wc -c < "$INSTALL_DIR/lib/libtag.a") bytes"
echo "    libtag_c.a: $(wc -c < "$INSTALL_DIR/lib/libtag_c.a") bytes"

# Copy to project
echo "--- Installing to $SCRIPT_DIR ---"
mkdir -p "$SCRIPT_DIR/lib" "$SCRIPT_DIR/include"
cp "$INSTALL_DIR/lib/libtag.a" "$INSTALL_DIR/lib/libtag_c.a" "$INSTALL_DIR/lib/libz.a" "$SCRIPT_DIR/lib/"
cp "$INSTALL_DIR/include/taglib/tag_c.h" "$SCRIPT_DIR/include/"

# Cleanup
rm -rf "$WORKDIR"

echo "=== Done. Build with: go build -tags native_taglib ==="

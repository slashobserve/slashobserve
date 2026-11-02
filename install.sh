#!/bin/sh
set -eu

RELEASES_PAGE="https://github.com/slashobserve/slashobserve/releases"

say() {
  printf '%s\n' "$*"
}

warn() {
  printf 'install.sh: warning: %s\n' "$*" >&2
}

fail() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 1
}

unsupported() {
  fail "$1 is not supported by this installer; download a release from $RELEASES_PAGE"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;;
    Linux) echo linux ;;
    *) unsupported "operating system $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo amd64 ;;
    arm64 | aarch64) echo arm64 ;;
    *) unsupported "architecture $(uname -m)" ;;
  esac
}

normalize_version() {
  version="${1#v}"
  printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
    fail "invalid version '$1'; use a release like v0.1.0 or 0.1.0"
  printf '%s\n' "$version"
}

pick_downloader() {
  if command -v curl >/dev/null 2>&1; then
    echo curl
  elif command -v wget >/dev/null 2>&1; then
    echo wget
  else
    fail "curl or wget is required"
  fi
}

download() {
  case "$downloader" in
    curl) curl -fsSL --retry 3 -o "$2" "$1" </dev/null ;;
    wget) wget -q -O "$2" "$1" </dev/null ;;
  esac || fail "download failed: $1"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    fail "sha256sum or shasum is required to verify the download"
  fi
}

version_from_checksums() {
  sed -n "s/^[0-9A-Fa-f]*[ *]*observe_\\(.*\\)_$2_$3\\.tar\\.gz\$/\\1/p" "$1" | head -n 1
}

expected_checksum() {
  awk -v name="$2" '{ file = $2; sub(/^\*/, "", file); if (file == name) { print tolower($1); exit } }' "$1"
}

choose_install_dir() {
  if [ -n "${OBSERVE_INSTALL_DIR:-}" ]; then
    echo "$OBSERVE_INSTALL_DIR"
  elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    echo /usr/local/bin
  elif [ -n "${HOME:-}" ]; then
    echo "$HOME/.local/bin"
  else
    fail "HOME is not set; set OBSERVE_INSTALL_DIR to choose where to install"
  fi
}

install_binary() {
  mkdir -p "$2" || fail "cannot create $2; set OBSERVE_INSTALL_DIR to a writable directory"
  staged="$2/.observe.$$"
  if ! { cp "$1" "$staged" && chmod 755 "$staged" && mv -f "$staged" "$2/observe"; }; then
    rm -f "$staged"
    fail "cannot write to $2; set OBSERVE_INSTALL_DIR to a writable directory"
  fi
}

main() {
  os="$(detect_os)"
  arch="$(detect_arch)"
  downloader="$(pick_downloader)"
  base="${OBSERVE_RELEASES_URL:-$RELEASES_PAGE}"

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  trap 'exit 1' HUP INT TERM

  if [ -n "${OBSERVE_VERSION:-}" ]; then
    version="$(normalize_version "$OBSERVE_VERSION")"
    download "$base/download/v$version/checksums.txt" "$tmp/checksums.txt"
  else
    download "$base/latest/download/checksums.txt" "$tmp/checksums.txt"
    version="$(version_from_checksums "$tmp/checksums.txt" "$os" "$arch")"
    [ -n "$version" ] || fail "cannot find the latest release for ${os}_$arch in $RELEASES_PAGE"
  fi

  archive="observe_${version}_${os}_$arch.tar.gz"
  want="$(expected_checksum "$tmp/checksums.txt" "$archive")"
  [ -n "$want" ] || fail "release v$version has no $archive; see $RELEASES_PAGE"

  say "Downloading observe v$version for ${os}/$arch"
  download "$base/download/v$version/$archive" "$tmp/$archive"

  got="$(sha256_of "$tmp/$archive")"
  [ "$got" = "$want" ] || fail "checksum mismatch for $archive (expected $want, got $got); not installing"

  mkdir "$tmp/unpacked"
  tar -xzf "$tmp/$archive" -C "$tmp/unpacked" observe || fail "$archive does not contain observe"

  dir="$(choose_install_dir)"
  install_binary "$tmp/unpacked/observe" "$dir"

  say "Installed $("$dir/observe" --version </dev/null) to $dir/observe"
  case ":${PATH:-}:" in
    *":$dir:"*) ;;
    *) warn "$dir is not on your PATH; add it, for example: export PATH=\"$dir:\$PATH\"" ;;
  esac
}

main "$@"

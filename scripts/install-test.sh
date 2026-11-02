#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
installer="$here/../install.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
export COPYFILE_DISABLE=1

fail() {
  printf 'FAIL  %s\n' "$1" >&2
  failures=$((failures + 1))
}

expect() {
  local name="$1" want="$2" got="$3"
  if [[ "$got" != "$want" ]]; then
    printf 'FAIL  %s\n  want: %q\n  got:  %q\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
}

expect_contains() {
  local name="$1" needle="$2" haystack="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'FAIL  %s\n  want to contain: %q\n  got: %q\n' "$name" "$needle" "$haystack" >&2
    failures=$((failures + 1))
  fi
}

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

make_release() {
  local root="$1" version="$2" dir sums stage os arch name
  dir="$root/download/v$version"
  mkdir -p "$dir"
  sums="$dir/checksums.txt"
  : >"$sums"
  for os in darwin linux; do
    for arch in amd64 arm64; do
      stage="$work/stage-$version-$os-$arch"
      mkdir -p "$stage"
      printf '#!/bin/sh\necho fake-%s-%s_%s\n' "$version" "$os" "$arch" >"$stage/observe"
      chmod 755 "$stage/observe"
      echo license >"$stage/LICENSE"
      echo notice >"$stage/NOTICE"
      name="observe_${version}_${os}_$arch.tar.gz"
      tar -czf "$dir/$name" -C "$stage" observe LICENSE NOTICE
      printf '%s  %s\n' "$(hash_file "$dir/$name")" "$name" >>"$sums"
    done
  done
  for arch in amd64 arm64; do
    printf '%064d  observe_%s_windows_%s.zip\n' 0 "$version" "$arch" >>"$sums"
  done
}

releases="$work/releases"
make_release "$releases" 0.1.0
make_release "$releases" 0.0.9
mkdir -p "$releases/latest/download"
cp "$releases/download/v0.1.0/checksums.txt" "$releases/latest/download/checksums.txt"

fakebin="$work/fakebin"
mkdir -p "$fakebin"
cat >"$fakebin/uname" <<'SH'
#!/bin/sh
case "$1" in
  -s) echo "$FAKE_UNAME_S" ;;
  -m) echo "$FAKE_UNAME_M" ;;
esac
SH
chmod 755 "$fakebin/uname"

out=""
rc=0
run_install() {
  local kernel="$1" machine="$2"
  shift 2
  rc=0
  out="$(env FAKE_UNAME_S="$kernel" FAKE_UNAME_M="$machine" PATH="$fakebin:$PATH" \
    OBSERVE_RELEASES_URL="file://$releases" "$@" sh "$installer" 2>&1)" || rc=$?
}

installed_version() { "$1/observe" --version 2>/dev/null || echo missing; }

while read -r kernel machine want; do
  dest="$work/map-$kernel-$machine"
  run_install "$kernel" "$machine" OBSERVE_INSTALL_DIR="$dest"
  expect "platform $kernel $machine exits 0" 0 "$rc"
  expect "platform $kernel $machine picks its archive" "fake-0.1.0-$want" "$(installed_version "$dest")"
done <<'TABLE'
Darwin x86_64 darwin_amd64
Darwin arm64 darwin_arm64
Linux x86_64 linux_amd64
Linux aarch64 linux_arm64
Linux arm64 linux_arm64
Linux amd64 linux_amd64
TABLE

while read -r kernel machine; do
  dest="$work/unsupported-$kernel-$machine"
  run_install "$kernel" "$machine" OBSERVE_INSTALL_DIR="$dest"
  expect "unsupported $kernel $machine fails" 1 "$rc"
  expect_contains "unsupported $kernel $machine points to releases" "https://github.com/slashobserve/slashobserve/releases" "$out"
  [[ ! -e "$dest/observe" ]] || fail "unsupported $kernel $machine installed a binary"
done <<'TABLE'
MINGW64_NT-10.0 x86_64
MSYS_NT-10.0 x86_64
CYGWIN_NT-10.0 x86_64
FreeBSD amd64
Linux i686
Linux armv7l
Darwin ppc
TABLE

dest="$work/latest"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest"
expect "latest resolves from the latest checksums" "fake-0.1.0-linux_amd64" "$(installed_version "$dest")"
expect_contains "latest prints the installed version" "Installed fake-0.1.0-linux_amd64 to $dest/observe" "$out"

while read -r pin want; do
  dest="$work/pin-$pin"
  run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest" OBSERVE_VERSION="$pin"
  expect "pin $pin exits 0" 0 "$rc"
  expect "pin $pin installs $want" "fake-$want-linux_amd64" "$(installed_version "$dest")"
done <<'TABLE'
v0.1.0 0.1.0
0.1.0 0.1.0
v0.0.9 0.0.9
0.0.9 0.0.9
TABLE

for bad in latest v1.2 1.2.3.4 "../x" "0.1.0/../../x" "v"; do
  dest="$work/badpin"
  run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest" OBSERVE_VERSION="$bad"
  expect "version '$bad' is rejected" 1 "$rc"
  expect_contains "version '$bad' says invalid" "invalid version" "$out"
done

dest="$work/unknown-release"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest" OBSERVE_VERSION=9.9.9
expect "a release that does not exist fails" 1 "$rc"
[[ ! -e "$dest/observe" ]] || fail "missing release installed a binary"

tampered="$work/tampered"
cp -R "$releases" "$tampered"
echo tampered >>"$tampered/download/v0.1.0/observe_0.1.0_linux_amd64.tar.gz"
dest="$work/mismatch"
rc=0
out="$(env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$fakebin:$PATH" OBSERVE_RELEASES_URL="file://$tampered" \
  OBSERVE_INSTALL_DIR="$dest" sh "$installer" 2>&1)" || rc=$?
expect "a checksum mismatch fails" 1 "$rc"
expect_contains "a checksum mismatch says so" "checksum mismatch" "$out"
[[ ! -e "$dest/observe" ]] || fail "a checksum mismatch installed a binary"

noasset="$work/noasset"
cp -R "$releases" "$noasset"
grep -v linux_amd64 "$releases/download/v0.1.0/checksums.txt" >"$noasset/download/v0.1.0/checksums.txt"
dest="$work/noasset-dest"
rc=0
out="$(env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$fakebin:$PATH" OBSERVE_RELEASES_URL="file://$noasset" \
  OBSERVE_INSTALL_DIR="$dest" OBSERVE_VERSION=0.1.0 sh "$installer" 2>&1)" || rc=$?
expect "an archive missing from checksums.txt fails" 1 "$rc"
[[ ! -e "$dest/observe" ]] || fail "an archive missing from checksums.txt installed a binary"

linked_bin() {
  local dir="$1" tool path
  shift
  mkdir -p "$dir"
  for tool in sh curl tar gzip awk sed grep head mktemp rm cp chmod mv mkdir cut "$@"; do
    path="$(command -v "$tool" || true)"
    [[ -n "$path" ]] && ln -sf "$path" "$dir/$tool"
  done
  cp "$fakebin/uname" "$dir/uname"
}

nohash="$work/nohash-bin"
linked_bin "$nohash"
dest="$work/nohash-dest"
rc=0
out="$(env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$nohash" OBSERVE_RELEASES_URL="file://$releases" \
  OBSERVE_INSTALL_DIR="$dest" sh "$installer" 2>&1)" || rc=$?
expect "no sha256 tool fails" 1 "$rc"
expect_contains "no sha256 tool says what is needed" "sha256sum or shasum" "$out"
[[ ! -e "$dest/observe" ]] || fail "no sha256 tool installed a binary"

if command -v shasum >/dev/null 2>&1 && command -v perl >/dev/null 2>&1; then
  shaonly="$work/shasum-bin"
  linked_bin "$shaonly" shasum perl
  dest="$work/shasum-dest"
  rc=0
  out="$(env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$shaonly" OBSERVE_RELEASES_URL="file://$releases" \
    OBSERVE_INSTALL_DIR="$dest" sh "$installer" 2>&1)" || rc=$?
  expect "shasum alone is enough" 0 "$rc"
  expect "shasum alone installs" "fake-0.1.0-linux_amd64" "$(installed_version "$dest")"
fi

dest="$work/nested/new/dir"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest" PATH="$fakebin:$dest:$PATH"
expect "the install dir is created" 0 "$rc"
[[ -x "$dest/observe" ]] || fail "installed file is not executable"
[[ "$out" != *"not on your PATH"* ]] || fail "warned about PATH although the dir is on it"

dest="$work/offpath"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest"
expect_contains "warns when the dir is not on PATH" "$dest is not on your PATH" "$out"

dest="$work/replace"
mkdir -p "$dest"
echo old >"$dest/observe"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$dest"
expect "an existing binary is replaced" "fake-0.1.0-linux_amd64" "$(installed_version "$dest")"

blocker="$work/blocker"
echo file >"$blocker"
run_install Linux x86_64 OBSERVE_INSTALL_DIR="$blocker/sub"
expect "an unwritable install dir fails" 1 "$rc"
expect_contains "an unwritable install dir says what to do" "OBSERVE_INSTALL_DIR" "$out"

if [[ ! -w /usr/local/bin ]]; then
  home="$work/home"
  mkdir -p "$home"
  run_install Linux x86_64 HOME="$home"
  expect "the default dir falls back to ~/.local/bin" "fake-0.1.0-linux_amd64" "$(installed_version "$home/.local/bin")"
fi

dest="$work/piped"
rc=0
out="$(cat "$installer" | env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$fakebin:$PATH" OBSERVE_RELEASES_URL="file://$releases" \
  OBSERVE_INSTALL_DIR="$dest" sh 2>&1)" || rc=$?
expect "piping the script into sh works" 0 "$rc"
expect "piping the script into sh installs" "fake-0.1.0-linux_amd64" "$(installed_version "$dest")"

dest="$work/truncated"
rc=0
sed '$d' "$installer" | env FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 PATH="$fakebin:$PATH" OBSERVE_RELEASES_URL="file://$releases" \
  OBSERVE_INSTALL_DIR="$dest" sh >/dev/null 2>&1 || rc=$?
expect "a script cut before its last line does nothing" 0 "$rc"
[[ ! -e "$dest/observe" ]] || fail "a truncated script installed a binary"

if ((failures > 0)); then
  exit 1
fi
echo "install script: ok"

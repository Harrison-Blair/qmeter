#!/usr/bin/env bash
# Install qmeter, a CLI tool to see your AI subscription usage limits.
#
#   curl -fsSL https://raw.githubusercontent.com/Harrison-Blair/qmeter/main/install.sh | bash
#
# Downloads the release archive for this machine from GitHub, verifies its
# SHA-256 against the release's checksums.txt, and unpacks the binary into
# ~/.local/bin. It never uses sudo and never writes outside the install
# directory.
#
# Environment:
#   QMETER_INSTALL_DIR   where to put the binary (default: ~/.local/bin)
#   QMETER_VERSION       release tag to install (default: the latest release)
#   QMETER_BASE_URL      repository URL (default: the qmeter repository)
#
# Options: --version <tag>, --install-dir <dir>, --help.

set -euo pipefail

DEFAULT_BASE_URL="https://github.com/Harrison-Blair/qmeter"
base_url="${QMETER_BASE_URL:-${DEFAULT_BASE_URL}}"
install_dir="${QMETER_INSTALL_DIR:-${HOME}/.local/bin}"
version="${QMETER_VERSION:-}"

die() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

say() {
	printf '%s\n' "$*"
}

usage() {
	cat <<'EOF'
Install qmeter.

Usage:
  install.sh [--version <tag>] [--install-dir <dir>]
  curl -fsSL <raw-url>/install.sh | bash
  curl -fsSL <raw-url>/install.sh | bash -s -- --version v0.1.0

Options:
  --version <tag>      install this release tag (default: the latest release)
  --install-dir <dir>  install into this directory (default: ~/.local/bin)
  -h, --help           print this help

Environment:
  QMETER_VERSION, QMETER_INSTALL_DIR, QMETER_BASE_URL
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || die "--version needs a release tag, like --version v0.1.0"
		version="$2"
		shift 2
		;;
	--version=*)
		version="${1#--version=}"
		shift
		;;
	--install-dir)
		[ "$#" -ge 2 ] || die "--install-dir needs a directory"
		install_dir="$2"
		shift 2
		;;
	--install-dir=*)
		install_dir="${1#--install-dir=}"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown option: $1 (try --help)"
		;;
	esac
done

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found on PATH"
}

need curl
need tar
need uname

if command -v sha256sum >/dev/null 2>&1; then
	sha256_of() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256_of() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	die "need sha256sum or shasum to verify the download, and found neither"
fi

os="$(uname -s)"
case "${os}" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) die "unsupported operating system: ${os} (this script installs on Linux and macOS; on Windows run install.ps1)" ;;
esac

arch="$(uname -m)"
case "${arch}" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "unsupported architecture: ${arch} (qmeter ships amd64 and arm64 builds)" ;;
esac

if [ -z "${version}" ]; then
	# /releases/latest answers with a redirect to /releases/tag/<tag>; reading
	# the Location header keeps this off the rate-limited REST API.
	redirect="$(curl -fsS -o /dev/null -w '%{redirect_url}' "${base_url}/releases/latest" 2>/dev/null || true)"
	case "${redirect}" in
	*/releases/tag/*) version="${redirect##*/releases/tag/}" ;;
	*) version="" ;;
	esac
	version="${version%%[/?#]*}"
	[ -n "${version}" ] || die "no qmeter release found at ${base_url}/releases/latest -- if a release exists, pass one explicitly with --version v0.1.0"
fi

asset="qmeter_${version}_${os}_${arch}.tar.gz"
asset_url="${base_url}/releases/download/${version}/${asset}"
checksums_url="${base_url}/releases/download/${version}/checksums.txt"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/qmeter-install.XXXXXX")"
# staged is set once there is a half-installed file to clean up as well.
staged=""
cleanup() {
	rm -rf "${tmp}"
	if [ -n "${staged}" ]; then
		rm -f "${staged}"
	fi
}
trap cleanup EXIT

fetch() { # fetch <url> <dest>
	curl -fsSL --retry 2 -o "$2" "$1" ||
		die "could not download $1 -- check the tag ${version} exists and that you are online"
}

say "Installing qmeter ${version} (${os}/${arch})"
fetch "${asset_url}" "${tmp}/${asset}"
fetch "${checksums_url}" "${tmp}/checksums.txt"

want="$(awk -v name="${asset}" '$2 == name {print $1}' "${tmp}/checksums.txt" | head -n1)"
[ -n "${want}" ] || die "checksums.txt for ${version} has no entry for ${asset}"
got="$(sha256_of "${tmp}/${asset}")"
if [ "${want}" != "${got}" ]; then
	die "checksum mismatch for ${asset}: expected ${want}, got ${got} -- refusing to install"
fi

tar -xzf "${tmp}/${asset}" -C "${tmp}" ||
	die "could not unpack ${asset}"
[ -f "${tmp}/qmeter" ] || die "${asset} did not contain a qmeter binary"

mkdir -p "${install_dir}" 2>/dev/null ||
	die "could not create ${install_dir} -- set QMETER_INSTALL_DIR to a directory you own"
[ -w "${install_dir}" ] ||
	die "${install_dir} is not writable -- set QMETER_INSTALL_DIR to a directory you own (this script never uses sudo)"

# Write beside the target and rename, so a running qmeter is replaced whole and
# is never briefly present with the wrong mode.
staged="${install_dir}/.qmeter.install.$$"
if command -v install >/dev/null 2>&1; then
	install -m 0755 "${tmp}/qmeter" "${staged}" ||
		die "could not write ${staged}"
else
	chmod 0755 "${tmp}/qmeter"
	cp -p "${tmp}/qmeter" "${staged}" ||
		die "could not write ${staged}"
fi
mv -f "${staged}" "${install_dir}/qmeter" ||
	die "could not move ${staged} into place"
staged=""

say "Installed ${install_dir}/qmeter"

case ":${PATH}:" in
*":${install_dir}:"*) ;;
*)
	say ""
	say "${install_dir} is not on your PATH. Add it, for example:"
	say "  echo 'export PATH=\"${install_dir}:\$PATH\"' >> ~/.profile"
	say "  export PATH=\"${install_dir}:\$PATH\""
	say ""
	;;
esac

"${install_dir}/qmeter" version

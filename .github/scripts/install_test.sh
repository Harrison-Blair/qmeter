#!/usr/bin/env bash
# Tests for install.sh. Runs it against a local stand-in for GitHub releases
# (fake_release_server.py) so nothing here touches the network, the real repo,
# or the user's home directory.
#
#   bash .github/scripts/install_test.sh
#
# Needs bash, curl, tar, python3 and sha256sum/shasum -- the same tools the
# installer itself needs. shellcheck is not run: it is not available here and is
# not a Go tool, so it cannot be fetched through the module cache.

set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
installer="${repo_root}/install.sh"
server_py="${script_dir}/fake_release_server.py"

work="$(mktemp -d "${TMPDIR:-/tmp}/qmeter-install-test.XXXXXX")"
server_pid=""

cleanup() {
	if [ -n "${server_pid}" ]; then
		kill "${server_pid}" 2>/dev/null
		wait "${server_pid}" 2>/dev/null
	fi
	rm -rf "${work}"
}
trap cleanup EXIT

passed=0
failed=0
current=""

start() {
	current="$1"
	printf '\n--- %s\n' "${current}"
}

ok() {
	passed=$((passed + 1))
	printf 'PASS %s: %s\n' "${current}" "$1"
}

bad() {
	failed=$((failed + 1))
	printf 'FAIL %s: %s\n' "${current}" "$1"
}

assert_status() { # assert_status <want> <got> <label>
	if [ "$1" = "$2" ]; then
		ok "$3 exit status $2"
	else
		bad "$3 exit status $2, want $1"
	fi
}

assert_contains() { # assert_contains <haystack> <needle> <label>
	case "$1" in
	*"$2"*) ok "$3" ;;
	*) bad "$3 -- output was: $1" ;;
	esac
}

assert_missing() { # assert_missing <haystack> <needle> <label>
	case "$1" in
	*"$2"*) bad "$3 -- output was: $1" ;;
	*) ok "$3" ;;
	esac
}

assert_file() {
	if [ -x "$1" ]; then
		ok "$2"
	else
		bad "$2 -- $1 is not an executable file"
	fi
}

assert_absent() {
	if [ -e "$1" ]; then
		bad "$2 -- $1 exists"
	else
		ok "$2"
	fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

# The host triple, worked out the same way install.sh does, so the fixtures
# carry an asset the installer will actually ask for.
host_os="$(uname -s)"
case "${host_os}" in
Linux) host_os=linux ;;
Darwin) host_os=darwin ;;
*)
	printf 'install_test.sh: unsupported test host %s\n' "${host_os}" >&2
	exit 1
	;;
esac
host_arch="$(uname -m)"
case "${host_arch}" in
x86_64 | amd64) host_arch=amd64 ;;
aarch64 | arm64) host_arch=arm64 ;;
*)
	printf 'install_test.sh: unsupported test host arch %s\n' "${host_arch}" >&2
	exit 1
	;;
esac

# add_asset <tag> <os> <arch> builds one release archive the way
# .github/workflows/release.yml does -- a qmeter binary (here a script that
# echoes its version and target), LICENSE and README.md, tarred as
# qmeter_<tag>_<os>_<arch>.tar.gz -- and appends its line to checksums.txt.
serve_root="${work}/serve"
add_asset() {
	local tag="$1" os="$2" arch="$3"
	local stage="${work}/stage-${tag}-${os}-${arch}"
	local dest="${serve_root}/releases/download/${tag}"
	local asset="qmeter_${tag}_${os}_${arch}.tar.gz"

	mkdir -p "${stage}" "${dest}"
	# The fake binary names its own target, so a test can tell which asset was
	# installed and not merely that some asset was.
	cat >"${stage}/qmeter" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = "version" ]; then
	echo "qmeter ${tag} ${os}_${arch}"
	exit 0
fi
echo "fake qmeter ${tag} ${os}_${arch}: \$*"
EOF
	chmod +x "${stage}/qmeter"
	printf 'fake license\n' >"${stage}/LICENSE"
	printf 'fake readme\n' >"${stage}/README.md"
	tar -czf "${dest}/${asset}" -C "${stage}" qmeter LICENSE README.md
	# checksums.txt is one `sha256sum qmeter_*` run over every asset.
	(cd "${dest}" && printf '%s  %s\n' "$(sha256_of "${asset}")" "${asset}" >>checksums.txt)
}

# make_release <tag> publishes the assets a real tag would have: the host's, so
# an unshimmed run finds one, and linux/arm64, so the uname-shimmed arm64 cases
# have something to install.
make_release() {
	local tag="$1"
	mkdir -p "${serve_root}/releases/download/${tag}"
	: >"${serve_root}/releases/download/${tag}/checksums.txt"
	add_asset "${tag}" "${host_os}" "${host_arch}"
	if [ "${host_os}/${host_arch}" != "linux/arm64" ]; then
		add_asset "${tag}" linux arm64
	fi
}

make_release v0.1.0
make_release v0.2.0
make_release v0.3.0
# v0.3.0 is the tampered one: flip the archive after checksums.txt was written,
# so what the installer downloads no longer matches what it was promised.
printf 'tampered' >>"${serve_root}/releases/download/v0.3.0/qmeter_v0.3.0_${host_os}_${host_arch}.tar.gz"

start_server() { # start_server <latest-tag-or-empty> ; echoes the port
	local latest="$1"
	local out="${work}/server-port-${2}"
	python3 "${server_py}" --root "${serve_root}" --latest "${latest}" \
		>"${out}" 2>"${work}/server-log-${2}" &
	server_pid=$!
	local i
	for i in $(seq 1 100); do
		[ -s "${out}" ] && break
		sleep 0.1
	done
	head -n1 "${out}"
}

stop_server() {
	if [ -n "${server_pid}" ]; then
		kill "${server_pid}" 2>/dev/null
		wait "${server_pid}" 2>/dev/null
		server_pid=""
	fi
}

port="$(start_server v0.1.0 main)"
if [ -z "${port}" ]; then
	printf 'install_test.sh: fake release server did not start\n' >&2
	cat "${work}/server-log-main" >&2
	exit 1
fi
base_url="http://127.0.0.1:${port}"

new_home() {
	local home
	home="$(mktemp -d "${work}/home.XXXXXX")"
	printf '%s' "${home}"
}

# shim_uname <sysname> <machine> puts a uname on PATH that reports the given
# platform, so the detection arms can be exercised off a single host.
shim="${work}/shim"
mkdir -p "${shim}"
shim_uname() {
	cat >"${shim}/uname" <<EOF
#!/usr/bin/env bash
case "\${1:-}" in
-m) echo "$2" ;;
*) echo "$1" ;;
esac
EOF
	chmod +x "${shim}/uname"
}

###############################################################################

start "syntax"
out="$(bash -n "${installer}" 2>&1)"
assert_status 0 "$?" "bash -n"
assert_missing "${out}" "error" "bash -n is quiet"

###############################################################################

start "default install dir"
home="$(new_home)"
out="$(env HOME="${home}" QMETER_BASE_URL="${base_url}" bash "${installer}" 2>&1)"
status=$?
assert_status 0 "${status}" "installer"
assert_file "${home}/.local/bin/qmeter" "installs into ~/.local/bin"
assert_contains "${out}" "qmeter v0.1.0 ${host_os}_${host_arch}" "prints the installed version, from the host's own asset"
assert_contains "${out}" "v0.1.0" "resolved the latest tag"

###############################################################################

start "PATH hint"
# The fresh HOME's ~/.local/bin is not on PATH, so the previous run should have
# said so; a run whose install dir is on PATH should not.
assert_contains "${out}" "${home}/.local/bin" "hint names the directory"
assert_contains "${out}" "PATH" "hint mentions PATH"

home2="$(new_home)"
dir2="${home2}/bin"
mkdir -p "${dir2}"
out2="$(env HOME="${home2}" QMETER_INSTALL_DIR="${dir2}" QMETER_BASE_URL="${base_url}" \
	PATH="${dir2}:${PATH}" bash "${installer}" 2>&1)"
assert_status 0 "$?" "installer with dir already on PATH"
assert_missing "${out2}" "not on your PATH" "no PATH hint when the dir is on PATH"

###############################################################################

start "QMETER_INSTALL_DIR override"
home3="$(new_home)"
dir3="${home3}/opt/qmeter-bin"
out3="$(env HOME="${home3}" QMETER_INSTALL_DIR="${dir3}" QMETER_BASE_URL="${base_url}" \
	bash "${installer}" 2>&1)"
assert_status 0 "$?" "installer"
assert_file "${dir3}/qmeter" "installs into the override dir"
assert_absent "${home3}/.local/bin/qmeter" "leaves the default dir alone"

###############################################################################

start "QMETER_VERSION pin"
home4="$(new_home)"
out4="$(env HOME="${home4}" QMETER_VERSION=v0.2.0 QMETER_BASE_URL="${base_url}" \
	bash "${installer}" 2>&1)"
assert_status 0 "$?" "installer"
assert_contains "${out4}" "qmeter v0.2.0" "installs the pinned version, not the latest"
assert_missing "${out4}" "qmeter v0.1.0" "does not install the latest version"

start "--version argument"
home5="$(new_home)"
out5="$(env HOME="${home5}" QMETER_BASE_URL="${base_url}" \
	bash "${installer}" --version v0.2.0 2>&1)"
assert_status 0 "$?" "installer"
assert_contains "${out5}" "qmeter v0.2.0" "--version pins the release"

###############################################################################

start "checksum mismatch"
home6="$(new_home)"
out6="$(env HOME="${home6}" QMETER_VERSION=v0.3.0 QMETER_BASE_URL="${base_url}" \
	bash "${installer}" 2>&1)"
status=$?
if [ "${status}" -ne 0 ]; then
	ok "installer refuses a tampered archive (exit ${status})"
else
	bad "installer accepted a tampered archive"
fi
assert_contains "${out6}" "checksum" "says the checksum did not match"
assert_absent "${home6}/.local/bin/qmeter" "installs nothing"

###############################################################################

# `uname -m` says aarch64 on 64-bit ARM Linux and arm64 on Apple silicon; both
# have to reach the arm64 asset. The test host is x86_64, so without these the
# whole arm64 arm of the case statement is unexercised.
start "aarch64 maps to the arm64 asset"
shim_uname Linux aarch64
home_a64="$(new_home)"
out_a64="$(env HOME="${home_a64}" QMETER_BASE_URL="${base_url}" PATH="${shim}:${PATH}" \
	bash "${installer}" 2>&1)"
assert_status 0 "$?" "installer"
assert_file "${home_a64}/.local/bin/qmeter" "installs on aarch64"
assert_contains "${out_a64}" "qmeter v0.1.0 linux_arm64" "installed the arm64 asset"
assert_missing "${out_a64}" "_amd64" "did not fall back to the amd64 asset"

start "arm64 maps to the arm64 asset"
shim_uname Linux arm64
home_m1="$(new_home)"
out_m1="$(env HOME="${home_m1}" QMETER_BASE_URL="${base_url}" PATH="${shim}:${PATH}" \
	bash "${installer}" 2>&1)"
assert_status 0 "$?" "installer"
assert_contains "${out_m1}" "qmeter v0.1.0 linux_arm64" "installed the arm64 asset"
assert_missing "${out_m1}" "_amd64" "did not fall back to the amd64 asset"

###############################################################################

start "unsupported architecture"
shim_uname Linux ppc64le
home7="$(new_home)"
out7="$(env HOME="${home7}" QMETER_BASE_URL="${base_url}" PATH="${shim}:${PATH}" \
	bash "${installer}" 2>&1)"
status=$?
if [ "${status}" -ne 0 ]; then
	ok "installer refuses an unsupported arch (exit ${status})"
else
	bad "installer accepted an unsupported arch"
fi
assert_contains "${out7}" "ppc64le" "names the unsupported architecture"
assert_absent "${home7}/.local/bin/qmeter" "installs nothing"

start "unsupported operating system"
shim_uname Plan9 x86_64
home8="$(new_home)"
out8="$(env HOME="${home8}" QMETER_BASE_URL="${base_url}" PATH="${shim}:${PATH}" \
	bash "${installer}" 2>&1)"
status=$?
if [ "${status}" -ne 0 ]; then
	ok "installer refuses an unsupported OS (exit ${status})"
else
	bad "installer accepted an unsupported OS"
fi
assert_contains "${out8}" "Plan9" "names the unsupported OS"

###############################################################################

start "piped into bash"
home9="$(new_home)"
out9="$(cat "${installer}" | env HOME="${home9}" QMETER_BASE_URL="${base_url}" bash 2>&1)"
assert_status 0 "$?" "cat install.sh | bash"
assert_file "${home9}/.local/bin/qmeter" "installs when piped"
assert_contains "${out9}" "qmeter v0.1.0" "prints the installed version when piped"

###############################################################################

start "unwritable install dir"
home10="$(new_home)"
dir10="${home10}/readonly"
mkdir -p "${dir10}"
chmod 500 "${dir10}"
out10="$(env HOME="${home10}" QMETER_INSTALL_DIR="${dir10}" QMETER_BASE_URL="${base_url}" \
	bash "${installer}" 2>&1)"
status=$?
chmod 700 "${dir10}"
if [ "${status}" -ne 0 ]; then
	ok "installer refuses an unwritable dir (exit ${status})"
else
	bad "installer wrote to an unwritable dir"
fi
assert_contains "${out10}" "not writable" "explains the directory is not writable"
assert_missing "${out10}" "sudo:" "does not try to escalate"

###############################################################################

start "no release published"
stop_server
port2="$(start_server "" norelease)"
home11="$(new_home)"
out11="$(env HOME="${home11}" QMETER_BASE_URL="http://127.0.0.1:${port2}" \
	bash "${installer}" 2>&1)"
status=$?
if [ "${status}" -ne 0 ]; then
	ok "installer fails when there is no release (exit ${status})"
else
	bad "installer claimed success with no release"
fi
assert_contains "${out11}" "no qmeter release found" "says there is no release"

###############################################################################

printf '\n%d passed, %d failed\n' "${passed}" "${failed}"
[ "${failed}" -eq 0 ]

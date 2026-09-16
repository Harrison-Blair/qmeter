#!/usr/bin/env bash
# Tests for next-version.sh.
#
# Each case builds a throwaway git repository under ${TMPDIR:-/tmp} and runs
# next-version.sh inside it, so the checkout this script lives in is never
# touched.
#
# Run: bash .github/scripts/next-version_test.sh

set -uo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script="$script_dir/next-version.sh"

failures=0
repos=()

cleanup() {
	local dir
	for dir in ${repos+"${repos[@]}"}; do
		rm -rf "$dir"
	done
}
trap cleanup EXIT

pass() { printf 'ok   - %s\n' "$1"; }
fail() {
	printf 'FAIL - %s\n' "$1" >&2
	failures=$((failures + 1))
}

# commit <repo> <message> adds one commit.
commit() {
	local dir=$1 msg=$2
	printf '%s\n' "$msg" >>"$dir/log.txt"
	git -C "$dir" add -A
	git -C "$dir" commit -q -m "$msg"
}

# make_repo prints the path to a fresh repo holding a single commit.
make_repo() {
	local dir
	dir=$(mktemp -d "${TMPDIR:-/tmp}/next-version-test.XXXXXX")
	repos+=("$dir")
	git -C "$dir" init -q
	git -C "$dir" config user.email test@example.com
	git -C "$dir" config user.name "Test User"
	commit "$dir" first
	printf '%s\n' "$dir"
}

# expect_output <name> <repo> <want> [ref]
expect_output() {
	local name=$1 dir=$2 want=$3
	shift 3
	local got status
	got=$(cd "$dir" && bash "$script" "$@" 2>&1)
	status=$?
	if [ "$status" -ne 0 ]; then
		fail "$name: exited $status, output: ${got:-<empty>}"
	elif [ "$got" != "$want" ]; then
		fail "$name: got '${got}', want '${want}'"
	else
		pass "$name"
	fi
}

# expect_failure <name> <repo> [ref]
expect_failure() {
	local name=$1 dir=$2
	shift 2
	local got status
	got=$(cd "$dir" && bash "$script" "$@" 2>/dev/null)
	status=$?
	if [ "$status" -eq 0 ]; then
		fail "$name: expected non-zero exit, got 0 with output '${got}'"
	elif [ -n "$got" ]; then
		fail "$name: expected no stdout, got '${got}'"
	else
		pass "$name"
	fi
}

# expect_stderr <name> <repo> checks a failing run explains itself on stderr.
expect_stderr() {
	local name=$1 dir=$2
	local err
	err=$(cd "$dir" && bash "$script" 2>&1 >/dev/null)
	if [ -z "$err" ]; then
		fail "$name: expected a message on stderr"
	else
		pass "$name"
	fi
}

# 1. No tags at all: the first release is v0.1.0.
repo=$(make_repo)
expect_output "no tags yields v0.1.0" "$repo" "v0.1.0"

# 2. Latest tag on an older commit: bump the minor.
repo=$(make_repo)
git -C "$repo" tag v0.1.0
commit "$repo" second
expect_output "v0.1.0 on an older commit yields v0.2.0" "$repo" "v0.2.0"

# 3. Patch resets to zero on a minor bump.
repo=$(make_repo)
git -C "$repo" tag v1.4.7
commit "$repo" second
expect_output "v1.4.7 yields v1.5.0" "$repo" "v1.5.0"

# 4. Ref already tagged: nothing to release.
repo=$(make_repo)
git -C "$repo" tag v0.3.0
expect_output "tag on HEAD yields empty output" "$repo" ""

# 5. The explicit ref argument is honoured.
repo=$(make_repo)
git -C "$repo" tag v0.3.0
commit "$repo" second
expect_output "explicit tagged ref yields empty output" "$repo" "" "v0.3.0"

# 6. Tags sort by version, not lexically (v0.10.0 > v0.9.0).
repo=$(make_repo)
git -C "$repo" tag v0.9.0
commit "$repo" second
git -C "$repo" tag v0.10.0
commit "$repo" third
expect_output "tags sort by version not lexically" "$repo" "v0.11.0"

# 7. A malformed latest tag is an error, not a guess.
repo=$(make_repo)
git -C "$repo" tag v1.2
commit "$repo" second
expect_failure "malformed tag v1.2 exits non-zero" "$repo"
expect_stderr "malformed tag v1.2 explains itself on stderr" "$repo"

# 8. Tags that are not vX.Y.Z releases are ignored.
repo=$(make_repo)
git -C "$repo" tag foo
commit "$repo" second
expect_output "non-v tag is ignored" "$repo" "v0.1.0"

# 9. Many tags: reading only the newest must not break on a long tag list.
#    Piping `git tag` into `head -1` dies with SIGPIPE (exit 141) under
#    `set -o pipefail` once the tag list outgrows the pipe buffer, which is a
#    few hundred releases in. Tags are written in one `update-ref` batch so the
#    case stays fast.
repo=$(make_repo)
first_commit=$(git -C "$repo" rev-parse HEAD)
for i in $(seq 1 500); do
	printf 'create refs/tags/v0.%d.0 %s\n' "$i" "$first_commit"
done | git -C "$repo" update-ref --stdin
commit "$repo" second
expect_output "500 tags yields v0.501.0" "$repo" "v0.501.0"

if [ "$failures" -ne 0 ]; then
	printf '\n%d test(s) failed\n' "$failures" >&2
	exit 1
fi
printf '\nall tests passed\n'

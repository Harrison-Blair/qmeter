#!/usr/bin/env bash
# next-version.sh [ref]
#
# Prints the version to release for <ref> (default HEAD), based on the git tags
# in the current repository:
#
#   * no vX.Y.Z tag exists          -> v0.1.0 (the first release)
#   * the latest tag points at <ref> -> nothing; <ref> is already released
#   * otherwise                      -> the latest tag with its minor bumped
#                                       and its patch reset (v1.4.7 -> v1.5.0)
#
# Exits non-zero with a message on stderr when the latest tag is not a vX.Y.Z
# version, rather than guessing what was meant.

set -euo pipefail

ref=${1:-HEAD}

if ! head_commit=$(git rev-parse --verify --quiet "${ref}^{commit}"); then
	printf 'next-version: cannot resolve ref %s\n' "$ref" >&2
	exit 1
fi

# Version-aware sort, so v0.10.0 outranks v0.9.0. This uses for-each-ref rather
# than piping `git tag` into `head`, because that pipeline dies with SIGPIPE once
# the tag list outgrows the pipe buffer -- which `set -o pipefail` turns into a
# silent exit 141 a few hundred releases in.
latest=$(git for-each-ref --count=1 --sort=-v:refname \
	--format='%(refname:strip=2)' 'refs/tags/v*')

if [ -z "$latest" ]; then
	printf 'v0.1.0\n'
	exit 0
fi

if ! [[ $latest =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
	printf 'next-version: latest tag %s is not a vX.Y.Z version\n' "$latest" >&2
	exit 1
fi

major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}

latest_commit=$(git rev-parse --verify "${latest}^{commit}")
if [ "$latest_commit" = "$head_commit" ]; then
	# Already released; print nothing so callers can skip the release.
	exit 0
fi

printf 'v%s.%s.0\n' "$major" "$((minor + 1))"

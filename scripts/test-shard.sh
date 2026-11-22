#!/usr/bin/env bash
#
# Run one shard of a Go package's tests, so a slow serial package can be split
# across CI jobs. Top-level tests are listed, sorted, and dealt round-robin, so
# neighbouring tests (which tend to be alike in cost) land on different shards.
#
# Usage:
#   bash scripts/test-shard.sh <package> <shard> <shards> [extra go test flags]
#   bash scripts/test-shard.sh ./cmd 1 3
#
# DRY_RUN=1 prints the shard's tests instead of running them.
# PRINT_REGEX=1 prints only the shard's -run regex, for wrappers that run go
#   test themselves (scripts/test-snowflake.sh keeps its retries that way).
# SHARD_LIST_RUN narrows which tests are listed (default: all), e.g. '^Test'.
# SHARD_LIST_ARGS are passed to the test binary while listing, e.g. -db=none
#   so tests/'s TestMain lists without starting a database.
# SHARD_SOLO names tests (space-separated) slow enough to need the last shard
#   to themselves; the rest are dealt across the other shards. A named test
#   that no longer exists leaves the last shard empty, which fails loudly.
#
# Every listed test lands in exactly one shard. A test added later is picked up
# by the next run's listing; nothing here needs updating.

set -euo pipefail

if [ "$#" -lt 3 ]; then
    echo "usage: $0 <package> <shard> <shards> [go test flags]" >&2
    exit 2
fi

pkg="$1"; shard="$2"; shards="$3"; shift 3

if ! [ "$shard" -ge 1 ] 2>/dev/null || ! [ "$shard" -le "$shards" ] 2>/dev/null; then
    echo "shard must be between 1 and $shards, got $shard" >&2
    exit 2
fi

timeout="${GO_TEST_TIMEOUT:-30m}"

# -list compiles the same -race test binary the run below uses, so the second
# build is a cache hit. Test, Example and Fuzz (seed corpus) entries all run
# under plain `go test`; benchmarks do not, so leave them out.
# SHARD_LIST_ARGS is deliberately unquoted: it holds zero or more flags.
# shellcheck disable=SC2086
list=$(go test -race -list "${SHARD_LIST_RUN:-.*}" "$pkg" ${SHARD_LIST_ARGS:-} | grep -E '^(Test|Example|Fuzz)' | LC_ALL=C sort)

total=$(printf '%s\n' "$list" | grep -c . || true)
if [ "$total" -eq 0 ]; then
    echo "no tests listed in $pkg" >&2
    exit 1
fi

solo="${SHARD_SOLO:-}"
if [ -n "$solo" ] && [ "$shards" -gt 1 ]; then
    solo_re="^($(printf '%s\n' $solo | paste -sd '|' -))\$"
    if [ "$shard" -eq "$shards" ]; then
        mine=$(printf '%s\n' "$list" | grep -E "$solo_re" || true)
    else
        mine=$(printf '%s\n' "$list" | grep -vE "$solo_re" | awk -v s="$shard" -v n="$((shards - 1))" '(NR - 1) % n == s - 1')
    fi
else
    mine=$(printf '%s\n' "$list" | awk -v s="$shard" -v n="$shards" '(NR - 1) % n == s - 1')
fi
count=$(printf '%s\n' "$mine" | grep -c . || true)

echo "==> $pkg shard $shard/$shards: $count of $total tests" >&2

if [ "$count" -eq 0 ]; then
    echo "shard $shard/$shards of $pkg is empty: use fewer shards, or check SHARD_SOLO (${solo:-unset}) still names real tests" >&2
    exit 1
fi

if [ "${DRY_RUN:-}" = "1" ]; then
    printf '%s\n' "$mine"
    exit 0
fi

# Anchored, slash-free: matches these top-level names exactly; their subtests run.
regex="^($(printf '%s\n' "$mine" | paste -sd '|' -))\$"

if [ "${PRINT_REGEX:-}" = "1" ]; then
    printf '%s\n' "$regex"
    exit 0
fi

exec go test -v -race -timeout "$timeout" -run "$regex" "$@" "$pkg"

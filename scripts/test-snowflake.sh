#!/bin/bash
set -e
cd "$(dirname "$0")/../tests"
echo "Running Snowflake tests..."

export GRAPHJIN_SNOWFLAKE_MOCK="${GRAPHJIN_SNOWFLAKE_MOCK:-1}"
if [ "$GRAPHJIN_SNOWFLAKE_MOCK" = "1" ]; then
	export GRAPHJIN_SNOWFLAKE_BACKEND="${GRAPHJIN_SNOWFLAKE_BACKEND:-duckdb}"
	export GRAPHJIN_SNOWFLAKE_FALLBACK="${GRAPHJIN_SNOWFLAKE_FALLBACK:-strict}"
	echo "Using Snowflake simulator (${GRAPHJIN_SNOWFLAKE_BACKEND} backend)"
fi

# DB_SHARD/DB_SHARDS run one slice of the suite (CI splits it across jobs; the
# simulator makes it the slowest database suite). The slice is fixed before the
# retries, so a retry re-runs the same tests.
RUN='^Test'
if [ -n "${DB_SHARD:-}" ]; then
	# TestSubscription is ~40% of the suite under the simulator (99 concurrent
	# subscriptions), so it gets the last shard to itself.
	RUN=$(SHARD_LIST_RUN='^Test' SHARD_LIST_ARGS='-db=none' SHARD_SOLO='TestSubscription' PRINT_REGEX=1 \
		bash ../scripts/test-shard.sh . "$DB_SHARD" "${DB_SHARDS:?DB_SHARDS must be set with DB_SHARD}")
fi

MAX_RETRIES=${SNOWFLAKE_TEST_RETRIES:-3}

for attempt in $(seq 1 "$MAX_RETRIES"); do
	if go test -v -timeout 30m -race -db=snowflake -run "$RUN" "$@" .; then
		exit 0
	fi

	if [ "$attempt" -lt "$MAX_RETRIES" ]; then
		echo ""
		echo "==> Attempt $attempt/$MAX_RETRIES failed, retrying in 5s..."
		sleep 5
	fi
done

echo "==> All $MAX_RETRIES attempts failed."
exit 1

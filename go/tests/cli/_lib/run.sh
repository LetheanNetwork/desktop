#!/usr/bin/env bash

# CLI test helpers for lthn.
# Borrowed verbatim shape from core/agent/tests/cli/_lib/run.sh — same
# discipline: every leaf Taskfile sources this and uses the run_capture_*
# / assert_* primitives so test output stays uniform across subsystems.

run_capture_stdout() {
	local expected_status="$1"
	local output_file="$2"
	shift 2

	set +e
	"$@" >"$output_file"
	local status=$?
	set -e

	if [[ "$status" -ne "$expected_status" ]]; then
		printf 'expected exit %s, got %s\n' "$expected_status" "$status" >&2
		if [[ -s "$output_file" ]]; then
			printf 'stdout:\n' >&2
			cat "$output_file" >&2
		fi
		return 1
	fi

	return 0
}

run_capture_all() {
	local expected_status="$1"
	local output_file="$2"
	shift 2

	set +e
	"$@" >"$output_file" 2>&1
	local status=$?
	set -e

	if [[ "$status" -ne "$expected_status" ]]; then
		printf 'expected exit %s, got %s\n' "$expected_status" "$status" >&2
		if [[ -s "$output_file" ]]; then
			printf 'output:\n' >&2
			cat "$output_file" >&2
		fi
		return 1
	fi

	return 0
}

run_capture_stderr() {
	local expected_status="$1"
	local output_file="$2"
	shift 2

	set +e
	"$@" 2>"$output_file" >/dev/null
	local status=$?
	set -e

	if [[ "$status" -ne "$expected_status" ]]; then
		printf 'expected exit %s, got %s\n' "$expected_status" "$status" >&2
		if [[ -s "$output_file" ]]; then
			printf 'stderr:\n' >&2
			cat "$output_file" >&2
		fi
		return 1
	fi

	return 0
}

assert_jq() {
	local expression="$1"
	local input_file="$2"
	jq -e "$expression" "$input_file" >/dev/null
}

assert_contains() {
	local needle="$1"
	local input_file="$2"
	grep -Fq "$needle" "$input_file"
}

assert_equals() {
	local expected="$1"
	local input_file="$2"
	local actual
	actual="$(cat "$input_file")"
	if [[ "$actual" != "$expected" ]]; then
		printf 'expected %q, got %q\n' "$expected" "$actual" >&2
		return 1
	fi
}

assert_not_contains() {
	local needle="$1"
	local input_file="$2"
	if grep -Fq "$needle" "$input_file"; then
		printf 'unexpected presence of %q in:\n' "$needle" >&2
		cat "$input_file" >&2
		return 1
	fi
}

# Isolate the test from the user's real ~/Lethean/ tree by pointing
# pkg/paths at a tmpdir-rooted HOME. Use at the top of any test that
# constructs newAppCore (state/events/process/config/serve/ai verbs).
#
#   isolate_home
#   trap "rm -rf $LTHN_TEST_HOME" EXIT
isolate_home() {
	LTHN_TEST_HOME="$(mktemp -d -t lthn-cli)"
	export HOME="$LTHN_TEST_HOME"
}

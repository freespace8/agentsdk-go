#!/usr/bin/env bash
set -euo pipefail

PORT=8080
VERBOSE=0
HOST="127.0.0.1"
HEALTH_TIMEOUT=30
CONNECT_TIMEOUT=2
REQUEST_TIMEOUT=20
SSE_TIMEOUT=8
BASE_URL=""

usage() {
  cat <<USAGE
Usage: ${0##*/} [-p port] [-v]

Options:
  -p <port>  Target HTTP port (default 8080)
  -v         Verbose mode (print full HTTP/SSE responses)
  -h         Show this help
USAGE
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command '$1' not found" >&2
    exit 1
  fi
}

now_ms() {
  python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
}

format_duration() {
  local ms=$1
  if (( ms >= 1000 )); then
    awk -v ms="$ms" 'BEGIN { printf "%.2fs", ms/1000 }'
  else
    printf "%dms" "$ms"
  fi
}

is_json() {
  local payload=$1
  [[ -n $payload ]] || return 1
  jq empty >/dev/null 2>&1 <<<"$payload"
}

perform_get() {
  local path=$1
  local timeout=${2:-5}
  local url="${BASE_URL}${path}"
  local tmp
  tmp=$(mktemp)
  local http_code status
  set +e
  http_code=$(curl -sS -m "$timeout" --connect-timeout "$CONNECT_TIMEOUT" \
    -H "Accept: application/json" \
    -o "$tmp" -w '%{http_code}' "$url")
  status=$?
  set -e
  RESPONSE_BODY=$(cat "$tmp")
  rm -f "$tmp"
  RESPONSE_CODE=$http_code
  [[ $status -eq 0 ]]
}

perform_post() {
  local path=$1
  local body=$2
  local timeout=${3:-$REQUEST_TIMEOUT}
  local url="${BASE_URL}${path}"
  local tmp
  tmp=$(mktemp)
  local http_code status
  set +e
  http_code=$(curl -sS -m "$timeout" --connect-timeout "$CONNECT_TIMEOUT" \
    -H "Accept: application/json" -H "Content-Type: application/json" \
    -X POST --data "$body" -o "$tmp" -w '%{http_code}' "$url")
  status=$?
  set -e
  RESPONSE_BODY=$(cat "$tmp")
  rm -f "$tmp"
  RESPONSE_CODE=$http_code
  [[ $status -eq 0 ]]
}

post_run_tool() {
  local instruction=$1
  local expected_tool=$2
  local expect_substr=${3:-}
  local payload
  payload=$(jq -n --arg input "$instruction" '{input:$input}')
  if ! perform_post "/run" "$payload" "$REQUEST_TIMEOUT"; then
    TEST_BODY=$RESPONSE_BODY
    TEST_SUMMARY="request failed (curl exit)"
    return 1
  fi
  TEST_BODY=$RESPONSE_BODY
  if [[ ${RESPONSE_CODE:-0} -ne 200 ]]; then
    TEST_SUMMARY="HTTP ${RESPONSE_CODE:-0}"
    return 1
  fi
  if ! is_json "$RESPONSE_BODY"; then
    TEST_SUMMARY="invalid JSON response"
    return 1
  fi
  local actual_tool
  actual_tool=$(jq -r '.tool_calls[0].name // empty' <<<"$RESPONSE_BODY")
  if [[ $actual_tool != "$expected_tool" ]]; then
    TEST_SUMMARY="expected tool $expected_tool (got ${actual_tool:-none})"
    return 1
  fi
  if [[ -n $expect_substr ]]; then
    local tool_output
    tool_output=$(jq -r '.tool_calls[0].output.Output // empty' <<<"$RESPONSE_BODY")
    if [[ $tool_output != *"$expect_substr"* ]]; then
      TEST_SUMMARY="output missing '$expect_substr'"
      return 1
    fi
  fi
  TEST_SUMMARY="HTTP ${RESPONSE_CODE:-0}"
  return 0
}

trim_inline() {
  local value=$1
  value=${value//$'\r'/ }
  value=${value//$'\n'/ }
  printf '%s' "$value"
}

health_test() {
  local attempt=0
  while (( attempt < HEALTH_TIMEOUT )); do
    if perform_get "/health" 5 && [[ ${RESPONSE_CODE:-0} -eq 200 ]]; then
      TEST_BODY=$RESPONSE_BODY
      if is_json "$RESPONSE_BODY"; then
        local status
        status=$(jq -r '.status // empty' <<<"$RESPONSE_BODY")
        local ts
        ts=$(jq -r '.time // empty' <<<"$RESPONSE_BODY")
        if [[ -n $status ]]; then
          TEST_SUMMARY="status=$status${ts:+ time=$ts}"
        else
          TEST_SUMMARY="HTTP 200"
        fi
      else
        TEST_SUMMARY=$(trim_inline "${RESPONSE_BODY:0:40}")
      fi
      return 0
    fi
    sleep 1
    ((attempt++))
  done
  TEST_SUMMARY="server not ready after ${HEALTH_TIMEOUT}s (last HTTP ${RESPONSE_CODE:-000})"
  TEST_BODY=$RESPONSE_BODY
  return 1
}

bash_tool_test() {
  local instruction='tool:bash_execute {"command":"echo test"}'
  if ! post_run_tool "$instruction" "bash_execute" "test"; then
    return 1
  fi
  local stdout
  stdout=$(jq -r '.tool_calls[0].output.Output // empty' <<<"$RESPONSE_BODY")
  stdout=$(trim_inline "$stdout")
  TEST_SUMMARY="stdout=${stdout:-<empty>}"
  TEST_BODY=$RESPONSE_BODY
  return 0
}

file_tool_test() {
  local instruction='tool:file_operation {"operation":"read","path":"go.mod"}'
  if ! post_run_tool "$instruction" "file_operation" "module"; then
    return 1
  fi
  local size
  size=$(jq -r '.tool_calls[0].output.Data.size // empty' <<<"$RESPONSE_BODY")
  TEST_SUMMARY="read go.mod (${size:-unknown} bytes)"
  TEST_BODY=$RESPONSE_BODY
  return 0
}

glob_tool_test() {
  local instruction='tool:glob {"pattern":"*.go"}'
  if ! post_run_tool "$instruction" "glob" ".go"; then
    return 1
  fi
  local matches
  matches=$(jq -r '.tool_calls[0].output.Output // empty' <<<"$RESPONSE_BODY")
  matches=$(trim_inline "${matches:0:60}")
  TEST_SUMMARY="matches: ${matches:-<none>}"
  TEST_BODY=$RESPONSE_BODY
  return 0
}

grep_tool_test() {
  local instruction='tool:grep {"pattern":"package main","path":"."}'
  if ! post_run_tool "$instruction" "grep" "package main"; then
    return 1
  fi
  local snippet
  snippet=$(jq -r '.tool_calls[0].output.Output // empty' <<<"$RESPONSE_BODY")
  snippet=$(trim_inline "${snippet:0:60}")
  TEST_SUMMARY="snippet: ${snippet:-<none>}"
  TEST_BODY=$RESPONSE_BODY
  return 0
}

stream_test() {
  local url="${BASE_URL}/run/stream?input=stream-test"
  local tmp
  tmp=$(mktemp)
  local http_code status
  set +e
  http_code=$(curl -sS -N --max-time "$SSE_TIMEOUT" --connect-timeout "$CONNECT_TIMEOUT" \
    -H "Accept: text/event-stream" -o "$tmp" -w '%{http_code}' "$url")
  status=$?
  set -e
  RESPONSE_BODY=$(cat "$tmp")
  rm -f "$tmp"
  RESPONSE_CODE=$http_code
  TEST_BODY=$RESPONSE_BODY
  if [[ $status -ne 0 && $status -ne 28 ]]; then
    TEST_SUMMARY="curl error $status"
    return 1
  fi
  if [[ -z $RESPONSE_BODY ]]; then
    TEST_SUMMARY="empty SSE payload"
    return 1
  fi
  local event_count
  event_count=$(grep -c '^event:' <<<"$RESPONSE_BODY")
  if (( event_count == 0 )); then
    TEST_SUMMARY="no SSE events"
    return 1
  fi
  TEST_SUMMARY="events=$event_count http=${RESPONSE_CODE:-000}"
  return 0
}

print_result() {
  local name=$1
  local status=$2
  local duration=$3
  local summary=$4
  printf "%-24s %-6s %8s  %s\n" "$name" "$status" "$(format_duration "$duration")" "$summary"
}

run_test() {
  local name=$1
  shift
  local fn=$1
  shift || true
  local start end duration status ret=0
  TEST_BODY=""
  TEST_SUMMARY=""
  start=$(now_ms)
  if "$fn" "$@"; then
    status="PASS"
  else
    status="FAIL"
    ret=1
  fi
  end=$(now_ms)
  duration=$((end - start))
  print_result "$name" "$status" "$duration" "${TEST_SUMMARY:-}"
  if (( VERBOSE )) && [[ -n ${TEST_BODY:-} ]]; then
    echo "---- ${name} response (HTTP ${RESPONSE_CODE:-???}) ----"
    echo "$TEST_BODY"
    echo "-----------------------------------------------"
  fi
  return $ret
}

main() {
  while getopts "p:vh" opt; do
    case $opt in
      p)
        PORT=$OPTARG
        ;;
      v)
        VERBOSE=1
        ;;
      h)
        usage
        exit 0
        ;;
      *)
        usage >&2
        exit 1
        ;;
    esac
  done
  shift $((OPTIND - 1))

  if ! [[ $PORT =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then
    echo "error: invalid port $PORT" >&2
    exit 1
  fi

  require_command curl
  require_command jq
  require_command python3

  BASE_URL="http://${HOST}:${PORT}"

  echo "Target server: ${BASE_URL}"
  printf "%-24s %-6s %8s  %s\n" "Test" "Status" "Time" "Details"

  local failed=0
  if ! run_test "Health" health_test; then
    exit 1
  fi
  if ! run_test "POST /run bash" bash_tool_test; then
    failed=1
  fi
  if ! run_test "POST /run file" file_tool_test; then
    failed=1
  fi
  if ! run_test "POST /run glob" glob_tool_test; then
    failed=1
  fi
  if ! run_test "POST /run grep" grep_tool_test; then
    failed=1
  fi
  if ! run_test "GET /run/stream" stream_test; then
    failed=1
  fi

  exit $failed
}

main "$@"

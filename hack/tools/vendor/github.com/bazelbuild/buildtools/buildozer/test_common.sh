#!/bin/bash

root="$TEST_TMPDIR/root"
TEST_log="$TEST_TMPDIR/log"
PKG="pkg"

function set_up() {
  ERROR=0
  mkdir -p "$root"
  cd "$root"
  touch WORKSPACE
}

# Runs buildozer, including saving the log and error messages, by sending the
# BUILD file contents ($1) to STDIN.
function run() {
  input="$1"
  shift
  mkdir -p "$PKG"

  pwd
  echo "$input" > "$PKG/BUILD"
  run_with_current_workspace "$buildozer --buildifier=" "$@"
}

# Runs buildozer, including saving the log and error messages.
function run_with_current_workspace() {
  log="$TEST_TMPDIR/log"
  log_err="$TEST_TMPDIR/log_err"
  cmd="$1"
  shift
  echo "$cmd $*"
  ret=0
  $cmd "$@" > "$log" 2> "$log_err" || ret=$?
  if [ "$ret" -ne "$ERROR" ]; then
    cat "$log"
    cat "$log_err"
    fail "Expected error code $ERROR, got $ret"
  fi
  # There must be an error message if error code is 1 or 2.
  if [ "$ret" -eq "1" -o "$ret" -eq "2" ]; then
    [ -s "$log_err" ] || fail "No error message, despite error code $ret"
  fi
}

function assert_equals() {
  expected="$1"
  pkg="${2-}"
  if [ -z "$pkg" ]; then
    pkg="$PKG"
  fi
  echo "$expected" > "$pkg/expected"
  diff -u "$root/$pkg/expected" "$root/$pkg/BUILD" || fail "Output didn't match"
}

function assert_err() {
  if ! grep "$1" "$log_err"; then
    cat "$log_err"
    fail "Error log doesn't contain '$1'"
  fi
}

function assert_no_err() {
  if grep "$1" "$log_err"; then
    cat "$log_err"
    fail "Error log contains '$1'"
  fi
}

function fail() {
    __show_log >&2
    echo "$TEST_name FAILED:" "$@" "." >&2
    echo "$@" >$TEST_TMPDIR/__fail
    TEST_passed="false"
    # Cleanup as we are leaving the subshell now
    exit 1
}

function __show_log() {
    echo "-- Test log: -----------------------------------------------------------"
    [[ -e $TEST_log ]] && cat $TEST_log || echo "(Log file did not exist.)"
    echo "------------------------------------------------------------------------"
}

function __pad() {
    local title=$1
    local pad=$2
    {
        echo -n "$pad$pad $title "
        printf "%80s" " " | tr ' ' "$pad"
    } | head -c 80
    echo
}

function __trap_with_arg() {
    func="$1" ; shift
    for sig ; do
        trap "$func $sig" "$sig"
    done
}

function run_suite() {
  local message="$1"

  echo >&2
  echo "$message" >&2
  echo >&2

  local total=0
  local passed=0

  TESTS=$(declare -F | awk '{print $3}' | grep ^test_ || true)

  for TEST_name in ${TESTS[@]}; do
    >$TEST_log # Reset the log.
    TEST_passed="true"

    total=$(($total + 1))
    __pad $TEST_name '*' >&2

    if [ "$(type -t $TEST_name)" = function ]; then
      # Run test in a subshell.
      rm -f $TEST_TMPDIR/__err_handled
      __trap_with_arg __test_terminated INT KILL PIPE TERM ABRT FPE ILL QUIT SEGV
      (
        set_up
        eval $TEST_name
        test $TEST_passed == "true"
      ) 2>&1 | tee $TEST_TMPDIR/__log
      # Note that tee will prevent the control flow continuing if the test
      # spawned any processes which are still running and have not closed
      # their stdout.

      test_subshell_status=${PIPESTATUS[0]}
      if [ "$test_subshell_status" != 0 ]; then
        TEST_passed="false"
      fi

    else # Bad test explicitly specified in $TESTS.
      fail "Not a function: '$TEST_name'"
    fi

    local red='\033[0;31m'
    local green='\033[0;32m'
    local no_color='\033[0m'

    if [[ "$TEST_passed" == "true" ]]; then
      echo -e "${green}PASSED${no_color}: $TEST_name" >&2
      passed=$(($passed + 1))
    else
      echo -e "${red}FAILED${no_color}: $TEST_name" >&2
      # end marker in CDATA cannot be escaped, we need to split the CDATA sections
      log=$(cat $TEST_TMPDIR/__log | sed 's/]]>/]]>]]&gt;<![CDATA[/g')
    fi

    echo >&2
  done
}
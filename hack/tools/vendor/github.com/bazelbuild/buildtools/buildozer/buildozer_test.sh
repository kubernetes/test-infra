#!/bin/bash

# --- begin runfiles.bash initialization ---
# Copy-pasted from Bazel's Bash runfiles library (tools/bash/runfiles/runfiles.bash).
set -euo pipefail
if [[ ! -d "${RUNFILES_DIR:-/dev/null}" && ! -f "${RUNFILES_MANIFEST_FILE:-/dev/null}" ]]; then
  if [[ -f "$0.runfiles_manifest" ]]; then
    export RUNFILES_MANIFEST_FILE="$0.runfiles_manifest"
  elif [[ -f "$0.runfiles/MANIFEST" ]]; then
    export RUNFILES_MANIFEST_FILE="$0.runfiles/MANIFEST"
  elif [[ -f "$0.runfiles/bazel_tools/tools/bash/runfiles/runfiles.bash" ]]; then
    export RUNFILES_DIR="$0.runfiles"
  fi
fi
if [[ -f "${RUNFILES_DIR:-/dev/null}/bazel_tools/tools/bash/runfiles/runfiles.bash" ]]; then
  source "${RUNFILES_DIR}/bazel_tools/tools/bash/runfiles/runfiles.bash"
elif [[ -f "${RUNFILES_MANIFEST_FILE:-/dev/null}" ]]; then
  source "$(grep -m1 "^bazel_tools/tools/bash/runfiles/runfiles.bash " \
            "$RUNFILES_MANIFEST_FILE" | cut -d ' ' -f 2-)"
else
  echo >&2 "ERROR: cannot find @bazel_tools//tools/bash/runfiles:runfiles.bash"
  exit 1
fi
# --- end runfiles.bash initialization ---

die () {
  echo "$1" 1>&2
  exit 1
}

[[ "$1" =~ external/* ]] && buildozer="${{1#external/}}" || buildozer="$TEST_WORKSPACE/$1"
buildozer="$(rlocation "$buildozer")"

source $TEST_SRCDIR/com_github_bazelbuild_buildtools/buildozer/test_common.sh

## TEST INPUTS

no_deps='go_library(
    name = "edit",
)'

empty_deps='go_library(
    name = "edit",
    deps = [],
)'

one_dep='go_library(
    name = "edit",
    deps = ["//buildifier:build"],
)'

two_deps='go_library(
    name = "edit",
    deps = [
        ":local",
        "//buildifier:build",
    ],
)'

two_deps_with_select='go_library(
    name = "edit",
    deps = [
        ":local",
        "//buildifier:build",
    ] + select({
        "//tools/some:condition": [
            "//some:value",
            "//some/other:value",
        ],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
    }),
)'

quoted_deps='go_library(
    name = "edit",
    deps = [
        "//buildifier:build",
        "//foo",
        "//bar",
        "\"//baz\"",
    ],
)'

## TESTS

function test_add_one_dep() {
  run "$one_dep" 'add deps //foo' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        "//buildifier:build",
        "//foo",
    ],
)'
}

function test_add_dep_no_deps() {
  run "$no_deps" 'add deps //foo' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//foo"],
)'
}

function test_add_dep_quotes() {
  run "$no_deps" 'add deps "//foo"' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//foo"],
)'
}

function test_add_dep_empty_deps() {
  run "$empty_deps" 'add deps //foo' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//foo"],
)'
}

function test_add_dep_two_deps() {
  run "$two_deps" 'add deps :local2' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":local",
        ":local2",
        "//buildifier:build",
    ],
)'
}

function test_add_existing_dep() {
  in='go_library(
    name = "edit",
    deps = [":local"],
)'
  ERROR=3 run "$in" 'add deps //pkg:local' '//pkg:edit'
  assert_equals "$in"
}

function test_add_existing_dep2() {
  in='go_library(
    name = "edit",
    deps = ["//pkg:local"],
)'
  ERROR=3 run "$in" 'add deps //pkg:local' '//pkg:edit'
  assert_equals "$in"
}

function test_add_shortened_dep() {
  in='go_library(
    name = "edit",
)'
  run "$in" 'add deps //pkg:local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":local"],
)'
}

function test_sorted_deps() {
  in='go_library(
    name = "edit",
    deps = [
      ":c",
      # comment that prevents buildifier reordering
      ":x",
      "//foo",
    ],
)'
  run "$in" 'add deps :z :a :e //last' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":a",
        ":c",
        ":e",
        # comment that prevents buildifier reordering
        ":x",
        ":z",
        "//foo",
        "//last",
    ],
)'
}

function test_noshorten_labels_flag() {
  in='go_library(
    name = "edit",
)'
  run "$in" --shorten_labels=false 'add deps //pkg:local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//pkg:local"],
)'
}

function test_add_duplicate_label() {
  # "build" and ":build" labels are equivalent
  in='go_library(
    name = "edit",
    deps = ["build"],
)'
  ERROR=3 run "$in" 'add deps :build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["build"],
)'
}

function test_add_duplicate_label2() {
  # "build" and ":build" labels are equivalent
  in='go_library(
    name = "edit",
    deps = [":build"],
)'
  ERROR=3 run "$in" 'add deps build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":build"],
)'
}

function test_remove_last_dep() {
  run "$one_dep" 'remove deps //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(name = "edit")'
}

function test_remove_dep() {
  run "$two_deps" 'remove deps //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":local"],
)'
}

function test_remove_dep_outside_of_select() {
  run "$two_deps_with_select" 'remove deps //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":local"] + select({
        "//tools/some:condition": [
            "//some:value",
            "//some/other:value",
        ],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
    }),
)'
}

function test_remove_all_deps_outside_of_select() {
  run "$two_deps_with_select" 'remove deps //buildifier:build :local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = select({
        "//tools/some:condition": [
            "//some:value",
            "//some/other:value",
        ],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
    }),
)'
}

function test_remove_dep_in_select() {
  run "$two_deps_with_select" 'remove deps //some:value' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":local",
        "//buildifier:build",
    ] + select({
        "//tools/some:condition": ["//some/other:value"],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
    }),
)'
}

function test_remove_deps_in_select() {
  run "$two_deps_with_select" 'remove deps //some:value //some/other:value' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":local",
        "//buildifier:build",
    ] + select({
        "//tools/some:condition": [],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
    }),
)'
}

function test_remove_all_deps_in_select() {
  run "$two_deps_with_select" 'remove deps //some:value //some/other:value //yet/another:value' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":local",
        "//buildifier:build",
    ],
)'
}

function test_remove_all_deps() {
  run "$two_deps_with_select" 'remove deps //some:value //some/other:value //yet/another:value :local //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(name = "edit")'
}

function test_remove_dep_quotes() {
  run "$two_deps" 'remove deps "//buildifier:build"' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":local"],
)'
}

function test_remove_local_dep() {
  run "$two_deps" 'remove deps local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//buildifier:build"],
)'
}

function test_remove_two_deps() {
  run "$two_deps" 'remove deps //buildifier:build :local' '//pkg:edit'
  assert_equals 'go_library(name = "edit")'
}

function test_remove_dep_using_long_label() {
  run "$two_deps" 'remove deps //pkg:local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//buildifier:build"],
)'
}

function test_remove_nonexistent_item() {
  run "$two_deps" 'remove deps //foo:bar :local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//buildifier:build"],
)'
}

function test_remove_item_with_comment() {
  commented_deps='go_library(
    name = "edit",
    deps = [
        # Do not remove!!!!11111!!!
        ":local",
        "//buildifier:build",  #fixdeps: keep
        "//remove:me",
    ],
)'

  run "$commented_deps" 'remove deps //buildifier:build //remove:me :local' '//pkg:edit'
  assert_equals 'go_library(name = "edit")'
}

function test_remove_src() {
  in='go_library(
    name = "edit",
    srcs = ["file" + ".go", "other.go"],
)'
  run "$in" 'remove srcs other.go' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    srcs = ["file" + ".go"],
)'
}

function test_remove_dep_without_colon() {
  in='go_library(
    name = "edit",
    deps = ["local", "//base"],
)'
  run "$in" 'remove deps //pkg:local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//base"],
)'
}

function test_remove_attribute() {
  run "$two_deps" 'remove deps' '//pkg:edit'
  assert_equals 'go_library(name = "edit")'
}

function test_remove_concatenated_attribute() {
  in='cc_library(name = "a", deps = ["//my/"] + ["foo"])'
  run "$in" 'remove deps :foo' '//pkg:a'
  assert_equals 'cc_library(
    name = "a",
    deps = ["//my/"],
)'
}

function test_remove_from_all_attributes() {
  in='java_library(name = "a", data = ["b","c"], exports = ["c"], deps = ["c","d"])'
  run "$in" 'remove * c' '//pkg:a'
  assert_equals 'java_library(
    name = "a",
    data = ["b"],
    deps = ["d"],
)'
}

function test_remove_from_all_rules() {
  in='cc_library(name = "a", visibility = ["//visibility:private"])
cc_library(name = "b")
exports_files(["a.cc"], visibility = ["//visibility:private"])'
  run "$in" 'remove visibility' '//pkg:all'
  assert_equals 'cc_library(name = "a")

cc_library(name = "b")

exports_files(["a.cc"])'
}

function test_remove_package_attribute() {
  in='package(default_visibility = ["//visibility:public"])'
  run "$in" 'remove default_visibility' '//pkg:__pkg__'
  [ $(wc -c < "$root/pkg/BUILD") -eq 0 ] || fail "Expected empty file"
}

function test_move_last_dep() {
  run "$one_dep" 'move deps runtime_deps //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = ["//buildifier:build"],
)'
}

function test_move_dep() {
  run "$two_deps" 'move deps runtime_deps //buildifier:build' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = ["//buildifier:build"],
    deps = [":local"],
)'
}

function test_move_two_deps() {
  run "$two_deps" 'move deps runtime_deps //buildifier:build :local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [
        ":local",
        "//buildifier:build",
    ],
)'
}

function test_move_dep_using_long_label() {
  run "$two_deps" 'move deps runtime_deps //pkg:local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [":local"],
    deps = ["//buildifier:build"],
)'
}

function test_move_dep_with_comment() {
  local input='go_library(
    name = "edit",
    deps = [
        ":local",  # needed at runtime for some obscure reason
        "//buildifier:build",
    ],
)'
  run "$input" 'move deps runtime_deps :local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [
        ":local",  # needed at runtime for some obscure reason
    ],
    deps = ["//buildifier:build"],
)'
}

function test_move_nonexistent_item() {
  run "$two_deps" 'move deps runtime_deps //foo:bar :local' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [":local"],
    deps = ["//buildifier:build"],
)'
}

function test_move_all_deps_to_new_attribute() {
  run "$two_deps" 'move deps runtime_deps *' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [
        ":local",
        "//buildifier:build",
    ],
)'
}

function test_move_all_deps_to_existing_attribute() {
  local input='go_library(
    name = "edit",
    runtime_deps = [":remote"],
    deps = [
        ":local",
        "//buildifier:build",
    ],
)'
  run "$input" 'move deps runtime_deps *' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    runtime_deps = [
        ":local",
        ":remote",
        "//buildifier:build",
    ],
)'
}

function test_rename_attribute() {
  run "$two_deps" 'rename deps data' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    data = [
        ":local",
        "//buildifier:build",
    ],
)'
}

function test_rename_attribute_already_exist() {
  in='cc_library(
    name = "edit",
    srcs = ["b.h"],
    hdrs = ["a.h"],
)'
  ERROR=2 run "$in" 'rename srcs hdrs' '//pkg:edit'
  assert_equals "$in"
  assert_err "attribute hdrs already exists in rule edit"
}

function test_rename_attribute_does_not_exist() {
  ERROR=2 run "$two_deps" 'rename data resources' '//pkg:edit'
  assert_equals "$two_deps"
  assert_err "no attribute data found in rule edit"
}

function test_with_python_code() {
  in='# test that Python code is preserved and does not crash
top_files = [
    "lgpl.txt",
    "README",
]

boom_files = top_files

boom_files.extend(glob(["**/*.hpp"]))

boom_files.remove("src/math/special_functions.hpp")

# comment for foo
def foo(x):
    x += [1, 2]

foo(boom_files)'

  run "$in" 'add default_visibility //visibility:public' '//pkg:__pkg__'
  assert_equals "package(default_visibility = [\"//visibility:public\"])

$in"
}

function test_replace_string_attr() {
  in='go_library(
    name = "edit",
    shared_library = ":local",  # Suffix comment.
)'
  run "$in" 'replace shared_library :local :new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    shared_library = ":new",  # Suffix comment.
)'
}

function test_replace_string_attr_quotes() {
  in='go_library(
    name = "edit",
    shared_library = ":local",  # Suffix comment.
)'
  run "$in" 'replace shared_library ":local" ":new"' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    shared_library = ":new",  # Suffix comment.
)'
}

function test_replace_string_attr_no_match() {
  in='go_library(
    name = "edit",
    library = ":no_match",  # Suffix comment.
)'
  ERROR=3 run "$in" 'replace library :local :new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    library = ":no_match",  # Suffix comment.
)'
}

function test_replace_concatenated_lists() {
  in='go_library(
    name = "edit",
    deps = [":local"] + CONSTANT,
)'
  run "$in" 'replace deps :local :new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":new"] + CONSTANT,
)'
}

function test_replace_dep() {
  in='go_library(
    name = "edit",
    deps = [
        # Before-comment.
        ":local",  # Suffix comment.
        "//buildifier:build",
    ] + select({
        "//tools/some:condition": [
            "//some:value",
        ],
    }),
)'
  run "$in" 'replace deps :local :new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        # Before-comment.
        ":new",  # Suffix comment.
        "//buildifier:build",
    ] + select({
        "//tools/some:condition": [
            "//some:value",
        ],
    }),
)'
}

function test_replace_dep_select() {
  # Replace a dep inside a select statement
  in='go_library(
    name = "edit",
    deps = [":dep"] + select({
        "//tools/some:condition": [
            "//some/other:value",
        ],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
        "//conditions:default": SOME_CONSTANT,
    }),
)'
  run "$in" 'replace deps //some/other:value :new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [":dep"] + select({
        "//tools/some:condition": [
            ":new",
        ],
        "//tools/other:condition": [
            "//yet/another:value",
        ],
        "//conditions:default": SOME_CONSTANT,
    }),
)'
}

function test_replace_dep_using_long_label() {
  run "$two_deps" 'replace deps //pkg:local //pkg:new' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        ":new",
        "//buildifier:build",
    ],
)'
}

function test_replace_in_all_attributes() {
  in='java_library(name = "a", data = ["b","c"], exports = ["c"], deps = ["c","d"])'
  run "$in" 'replace * c e' '//pkg:a'
  assert_equals 'java_library(
    name = "a",
    data = [
        "b",
        "e",
    ],
    exports = ["e"],
    deps = [
        "d",
        "e",
    ],
)'
}

function test_delete_rule_all() {
  in='cc_library(name = "all")
cc_library(name = "b")'
  run "$in" 'delete' '//pkg:all'
  assert_equals 'cc_library(name = "b")'
}

function test_delete_rule_star() {
  in='cc_library(name = "all")
cc_library(name = "b")'
  run "$in" 'delete' '//pkg:*'
  [ $(wc -c < "$root/pkg/BUILD") -eq 0 ] || fail "Expected empty file"
}

function test_delete_rule() {
  in='cc_library(name = "a")

cc_library(name = "b")

# Comment to make sure it is preserved

a = 42

cc_library(name = "c")'

  run "$in" 'delete' '//pkg:b'
  assert_equals 'cc_library(name = "a")

# Comment to make sure it is preserved

a = 42

cc_library(name = "c")'
}

function test_delete_package() {
  in='package(default_visibility = ["//visibility:public"])

cc_library(name = "a")'

  run "$in" 'delete' '//pkg:__pkg__'
  assert_equals 'cc_library(name = "a")'
}

function test_delete_missing_package() {
  in='cc_library(name = "a")'
  ERROR=3 run "$in" 'delete' '//pkg:__pkg__'
  assert_equals "$in"
}

function test_delete_target_without_name() {
  in='load("/path/f", "a")

a(arg1 = "foo")'

  run "$in" 'delete' '//pkg:%a'
  assert_equals 'load("/path/f", "a")'
}

function test_delete_using_line_number() {
  in='load("/path/f", "a")

a(arg1 = "foo")'

    run "$in" 'delete' '//pkg:%3'
  assert_equals 'load("/path/f", "a")'
}

function test_copy() {
  in='proto_library(name = "from", visibility = ["://foo"] + CONST)

cc_binary(name = "to")'

  run "$in" 'copy visibility from' '//pkg:to'
  assert_equals 'proto_library(
    name = "from",
    visibility = ["://foo"] + CONST,
)

cc_binary(
    name = "to",
    visibility = ["://foo"] + CONST,
)'
}

function test_copy_overwrite() {
  in='proto_library(name = "from", testonly = 1)

cc_binary(name = "to", testonly = 2)'

  run "$in" 'copy testonly from' '//pkg:to'
  assert_equals 'proto_library(
    name = "from",
    testonly = 1,
)

cc_binary(
    name = "to",
    testonly = 1,
)'
}

function test_copy_no_overwrite() {
  in='proto_library(name = "from", visibility = ["://foo"] + CONST)

cc_binary(name = "to")'

  run "$in" 'copy_no_overwrite visibility from' '//pkg:to'
  assert_equals 'proto_library(
    name = "from",
    visibility = ["://foo"] + CONST,
)

cc_binary(
    name = "to",
    visibility = ["://foo"] + CONST,
)'
}

function test_copy_no_overwrite_no_overwrite() {
  in='proto_library(name = "from", testonly = 1)

cc_binary(name = "to", testonly = 2)'

  run "$in" 'copy_no_overwrite testonly from' '//pkg:to'
  assert_equals 'proto_library(
    name = "from",
    testonly = 1,
)

cc_binary(
    name = "to",
    testonly = 2,
)'
}

function test_copy_no_from_rule() {
  in='go_binary(name = "to")'
  ERROR=2 run "$in" 'copy visibility from' '//pkg:to'
  assert_err "could not find rule 'from'"
}

function test_copy_no_attribute() {
  in='go_binary(name = "from")

go_binary(name = "to")'
  ERROR=2 run "$in" 'copy visibility from' '//pkg:to'
  assert_err "rule 'from' does not have attribute 'visibility'"
}

function test_set_kind() {
  in='cc_library(name = "a")'

  run "$in" 'set kind java_library' '//pkg:a'
  assert_equals 'java_library(name = "a")'
}

function test_set_list() {
  in='cc_library(name = "a")'

  run "$in" 'set copts foo' '//pkg:a'
  assert_equals 'cc_library(
    name = "a",
    copts = ["foo"],
)'
}

function test_set_list() {
  in='cc_library(name = "a")'

  run "$in" 'set copts foo bar baz' '//pkg:a'
  assert_equals 'cc_library(
    name = "a",
    copts = [
        "foo",
        "bar",
        "baz",
    ],
)'
}

function test_set_string() {
  in='cc_library(name = "a")'

  run "$in" 'set name b' '//pkg:a'
  assert_equals 'cc_library(name = "b")'
}

function test_set_int() {
  in='cc_test(name = "a")'

  run "$in" 'set shard_count 8' '//pkg:a'
  assert_equals 'cc_test(
    name = "a",
    shard_count = 8,
)'
}

function test_set_licenses() {
  in='cc_test(name = "a")'

  run "$in" 'set licenses foo' '//pkg:a'
  assert_equals 'cc_test(
    name = "a",
    licenses = ["foo"],
)'
}

function test_set_distribs() {
  in='cc_test(name = "a")'

  run "$in" 'set distribs foo' '//pkg:a'
  assert_equals 'cc_test(
    name = "a",
    distribs = ["foo"],
)'
}

function test_set_if_absent_absent() {
  in='soy_js(name = "a")'

  run "$in" 'set_if_absent allowv1syntax 1' '//pkg:a'
  assert_equals 'soy_js(
    name = "a",
    allowv1syntax = 1,
)'
}

function test_set_if_absent_present() {
  in='soy_js(
  name = "a",
  allowv1syntax = 0
  )'

  run "$in" 'set_if_absent allowv1syntax 1' '//pkg:a'
  assert_equals 'soy_js(
    name = "a",
    allowv1syntax = 0,
)'
}

function test_set_custom_code() {
  in='cc_test(name = "a")'

  run "$in" 'set attr foo(a=1,b=2)' '//pkg:a'
  assert_equals 'cc_test(
    name = "a",
    attr = foo(
        a = 1,
        b = 2,
    ),
)'
}

function assert_output() {
  echo "$1" > "expected"
  diff -u "expected" "$log" || fail "Output didn't match"
}

function assert_output_any_order() {
  echo "$1" | sort > "expected"
  sort < "$log" > "log_sorted"
  diff -u "expected" "log_sorted" || fail "Output didn't match"
}

function test_print_all_functions() {
  in='package(default_visibility = ["//visibility:public"])
cc_test(name = "a")
java_binary(name = "b")
exports_files(["a.cc"])'
  run "$in" 'print kind' '//pkg:all'
  assert_output 'package
cc_test
java_binary
exports_files'
}

function test_print_java_libraries() {
  in='cc_test(name = "a")
java_library(name = "b")
java_library(name = "c")'
  run "$in" 'print' '//pkg:%java_library'
  assert_output 'b java_library
c java_library'
}

function test_refer_to_rule_by_location() {
  in='cc_test(name = "a")
java_library(name = "b")
java_library(name = "c")'
  run "$in" 'print label' '//pkg:%2'
  assert_output '//pkg:b'
}

function test_refer_to_rule_by_location_no_such_rule() {
  in='cc_test(name = "a")
java_library(name = "b")
java_library(name = "c")'
  ERROR=2 run "$in" 'print label' '//pkg:%999'
  assert_err "rule '%999' not found"
}

function test_visibility_exported_file() {
  in='exports_files(["a.txt", "b.txt"])'
  run "$in" 'set visibility //foo:__pkg__' '//pkg:b.txt'
  assert_equals 'exports_files(
    [
        "a.txt",
        "b.txt",
    ],
    visibility = ["//foo:__pkg__"],
)'
}

function test_print_srcs() {
  in='cc_test(name = "a", srcs = ["foo.cc"])
java_library(name = "b")
java_library(name = "c", srcs = ["foo.java", "bar.java"])'
  run "$in" 'print name kind srcs' '//pkg:*'
  assert_output 'a cc_test [foo.cc]
b java_library (missing)
c java_library [foo.java bar.java]'
  assert_err 'rule "//pkg:b" has no attribute "srcs"'
}

function test_print_empty_list() {
  in='package()
java_library(name = "b", deps = [])'
  run "$in" 'print deps' '//pkg:b'
  assert_output '[]'
}

function test_print_label() {
  in='package()
java_library(name = "b")'
  run "$in" 'print label kind' '//pkg:*'
  assert_output '//pkg:b java_library'
}

function test_print_label_ellipsis() {
  mkdir -p "ellipsis_test/foo/bar"
  echo 'java_library(name = "test")' > "ellipsis_test/BUILD"
  echo 'java_library(name = "foo")' > "ellipsis_test/foo/BUILD"
  echo 'java_library(name = "foobar"); java_library(name = "bar");' > "ellipsis_test/foo/bar/BUILD"

  in='package()
java_library(name = "b")'
  run "$in" 'print label' '//ellipsis_test/...:*'
  assert_output_any_order '//ellipsis_test:test
//ellipsis_test/foo
//ellipsis_test/foo/bar:foobar
//ellipsis_test/foo/bar'
}

function test_print_startline() {
  in='package()
java_library(name = "b")'
  run "$in" 'print startline label' '//pkg:*'
  assert_output '2 //pkg:b'
}

function test_print_endline() {
  in='package()
java_library(
    name = "b"
)'
  run "$in" 'print endline label' '//pkg:*'
  assert_output '4 //pkg:b'
}

function test_print_rule() {
  in='cc_library(name = "a")

# Comment before
cc_test(
    name = "b",
    copts = [
        # comment before
        "foo",  # comment after
    ],
)

cc_library(name = "c")'
  run "$in" 'print rule' '//pkg:b'
  assert_output '# Comment before
cc_test(
    name = "b",
    copts = [
        # comment before
        "foo",  # comment after
    ],
)'
}

function test_print_version() {
  in='gendeb(name = "foobar", version = "12345")'
  run "$in" 'print version' '//pkg:*'
  assert_output '12345'
}

function test_new_cc_library() {
  in='cc_test(name = "a")

# end of file comment'
  run "$in" 'new cc_library foo' '//pkg:__pkg__'
  assert_equals 'cc_test(name = "a")

cc_library(name = "foo")

# end of file comment'
}

function test_new_cc_library_after_other_libraries() {
  in='cc_library(name = "l")
cc_test(name = "a")'
  run "$in" 'new cc_library foo' '//pkg:__pkg__'
  assert_equals 'cc_library(name = "l")

cc_library(name = "foo")

cc_test(name = "a")'
}

function test_new_cc_library_empty_file() {
  in=''
  run "$in" 'new cc_library foo' '//pkg:__pkg__'
  assert_equals 'cc_library(name = "foo")'
}

function test_new_java_library() {
  in='cc_test(name = "a")'
  run "$in" 'new java_library foo' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

java_library(name = "foo")'
}

function test_new_already_exists() {
  in='cc_test(name = "a")'
  ERROR=2 run "$in" 'new cc_library a' '//pkg:__pkg__'
  assert_err "rule 'a' already exists"
}

function test_new_before_first() {
  in='cc_test(name = "a")'
  run "$in" 'new java_library foo before a' 'pkg/BUILD'
  assert_equals 'java_library(name = "foo")

cc_test(name = "a")'
}

function test_new_before_last() {
  in='cc_test(name = "a")

cc_test(name = "b")'
  run "$in" 'new java_library foo before b' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

java_library(name = "foo")

cc_test(name = "b")'
}

function test_new_before_nonexistent_rule() {
  in='cc_test(name = "a")

cc_test(name = "b")'
  run "$in" 'new java_library foo before bar' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

cc_test(name = "b")

java_library(name = "foo")'
}

function test_new_before_already_exists() {
  in='cc_test(name = "foo")
cc_test(name = "new_rule")'
  ERROR=2 run "$in" 'new java_library new_rule before foo' 'pkg/BUILD'
  assert_err "rule 'new_rule' already exists"
}

function test_new_after_first() {
  in='cc_test(name = "a")'
  run "$in" 'new java_library foo after a' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

java_library(name = "foo")'
}

function test_new_after_last() {
  in='cc_test(name = "a")

cc_test(name = "b")'
  run "$in" 'new java_library foo after b' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

cc_test(name = "b")

java_library(name = "foo")'
}

function test_new_after_by_location() {
  in='
cc_test(name = "a")

cc_test(name = "b")'
  run "$in" 'new java_library foo after %2' 'pkg/BUILD'
  assert_equals 'cc_test(name = "a")

java_library(name = "foo")

cc_test(name = "b")'
}

function test_new_after_package() {
  in='
load("/foo/bar", "x", "y", "z")
package(default_visibility = "//visibility:public")

cc_test(name = "a")

cc_test(name = "b")'
  run "$in" 'new java_library foo after __pkg__' 'pkg/BUILD'
  assert_equals 'load("/foo/bar", "x", "y", "z")

package(default_visibility = "//visibility:public")

java_library(name = "foo")

cc_test(name = "a")

cc_test(name = "b")'
}

function test_not_enough_arguments() {
  ERROR=1 run "$one_dep" 'add foo' '//pkg:edit'
  assert_err "Too few arguments for command 'add', expected at least 2."
}

function test_too_many_arguments() {
  ERROR=1 run "$one_dep" 'delete foo' '//pkg:edit'
  assert_err "Too many arguments for command 'delete', expected at most 0."
}

function test_package_name_missing() {
  ERROR=2 run "$one_dep" 'add deps //dep' ':edit'
  assert_err "file not found"
}

function test_nonexistent_package_name() {
  ERROR=2 run "$one_dep" 'add deps //dep' '//doesnt_exist:edit'
  assert_err "file not found"
}

function test_nonexistent_rule_name() {
  ERROR=2 run "$one_dep" 'add deps //dep' '//pkg:doesnt_exist'
  assert_err "rule 'doesnt_exist' not found"
}

function test_keep_going() {
  ERROR=2 run "$one_dep" -k 'add deps //dep' 'new cc_library edit' '//pkg:doesnt_exist' '//pkg:edit'
  assert_err "rule 'doesnt_exist' not found"
  assert_err "rule 'edit' already exists"
  # Make sure the commands are in the error message.
  assert_err "add deps //dep"
  assert_err "new cc_library edit"
  # Make sure the full targets are in the error message.
  assert_err "//pkg:doesnt_exist"
  assert_equals 'go_library(
    name = "edit",
    deps = [
        "//buildifier:build",
        "//dep",
    ],
)'
}

function test_buildifier_missing() {
  ERROR=2 run "$one_dep" '--buildifier=doesnt_exist' 'add deps //dep' '//pkg:edit'
  assert_err "executable file not found in \$PATH"
}

function test_buildifier_return_error() {
  ERROR=2 run "$one_dep" '--buildifier=false' 'add deps //dep' '//pkg:edit'
  assert_err "running buildifier: exit status 1"
}

function test_add_deps_to_targets_in_same_package() {
  in='cc_library(name = "smurf")
cc_library(name = "smorf")'
  run "$in" 'add deps //foo:bar' //pkg:smurf //pkg:smorf
  assert_equals 'cc_library(
    name = "smurf",
    deps = ["//foo:bar"],
)

cc_library(
    name = "smorf",
    deps = ["//foo:bar"],
)'
}

function test_rule_comment() {
  in='cc_library(name = "a")'
  run "$in" 'comment Hello' //pkg:a
  assert_equals '# Hello
cc_library(name = "a")'
}

function test_attribute_comment() {
  in='cc_library(
    name = "a",
    srcs = ["a.cc"],
)'
  run "$in" 'comment srcs Hello\ World' //pkg:a
  assert_equals 'cc_library(
    name = "a",
    srcs = ["a.cc"],  # Hello World
)'
}

function test_attribute_comment_no_eol() {
  in='cc_library(
    name = "a",
    srcs = ["a.cc"],
)'
  run "$in" --eol-comments=false 'comment srcs Hello\ World' //pkg:a
  assert_equals 'cc_library(
    name = "a",
    # Hello World
    srcs = ["a.cc"],
)'
}

function test_value_comment() {
  in='cc_library(
    name = "a",
    srcs = [
        "a.cc",
        "b.cc",  # Old
    ],
)'
  run "$in" 'comment srcs a.cc Hello' 'comment srcs b.cc New' //pkg:a
  assert_equals 'cc_library(
    name = "a",
    srcs = [
        "a.cc",  # Hello
        "b.cc",  # New
    ],
)'
}

function test_value_multiline_comment() {
  in='cc_library(
    name = "a",
    srcs = [
        "a.cc",
        "b.cc",
    ],
)'
  run "$in" 'comment srcs b.cc Just\ a\
multiline\ comment' //pkg:a
  assert_equals 'cc_library(
    name = "a",
    srcs = [
        "a.cc",
        # Just a
        # multiline comment
        "b.cc",
    ],
)'
  run "$in" 'comment srcs b.cc Another\nmultiline\ comment' //pkg:a
  assert_equals 'cc_library(
    name = "a",
    srcs = [
        "a.cc",
        # Another
        # multiline comment
        "b.cc",
    ],
)'
}

function test_rule_print_comment() {
  in='# Hello
cc_library(name = "a")'
  run "$in" 'print_comment' //pkg:a
  assert_output 'Hello'
}

function test_rule_print_comment_with_suffix_and_after() {
  in='# Hello Before
cc_library(name = "a") # Hello Suffix
# Hello After'
  run "$in" 'print_comment' //pkg:a
  assert_output 'Hello Before Hello Suffix Hello After'
}

function test_attribute_print_comment() {
  in='cc_library(
    name = "a",
    srcs = ["a.cc"],  # Hello World
)'
  run "$in" 'print_comment srcs' //pkg:a
  assert_output 'Hello World'
}

function test_attribute_print_comment_no_eol() {
  in='cc_library(
    name = "a",
    # Hello World
    srcs = ["a.cc"],
)'
  run "$in" --eol-comments=false 'print_comment srcs' //pkg:a
  assert_output 'Hello World'
}

function test_value_print_comment() {
  in='cc_library(
    name = "a",
    srcs = [
        "a.cc",  # World
        "b.cc",  # Hello
    ],
)'
  run "$in" 'print_comment srcs b.cc' 'print_comment srcs a.cc' //pkg:a
  assert_output 'Hello
World'
}

function test_value_multiline_print_comment() {
  in='cc_library(
    name = "a",
    srcs = [
        "a.cc",
        # Just a
        # multiline comment
        "b.cc",
    ],
)'
  run "$in" 'print_comment srcs b.cc' //pkg:a
  assert_output 'Just a multiline comment'
}

function test_value_inside_select_print_comment() {
  in='cc_library(
    name = "a",
    srcs = [
        "a.cc",  # World
        "b.cc",  # Hello
    ] + select({
        "foo": [
            "c.cc",  # hello
            "d.cc",  # world
        ],
    }),
)'
  run "$in" 'print_comment srcs c.cc' 'print_comment srcs d.cc' //pkg:a
  assert_output 'hello
world'
}

# Test both absolute and relative package names
function test_path() {
  mkdir -p "java/com/foo/myproject"
  echo 'java_library(name = "foo")' > "java/com/foo/myproject/BUILD"

  $buildozer --buildifier= 'add deps a' "java/com/foo/myproject:foo"
  cd java
  $buildozer --buildifier= 'add deps b' "com/foo/myproject:foo"
  cd com
  $buildozer --buildifier= 'add deps c' "//java/com/foo/myproject:foo"
  cd foo
  $buildozer --buildifier= 'add deps d' "myproject:foo"
  cd myproject
  $buildozer --buildifier= 'add deps e' ":foo"

  # Check that all dependencies have been added
  echo -n 'java_library(name="foo",deps=["a","b","c","d","e",],)' > expected
  tr -d ' \n' < "BUILD" > result
  diff -u "expected" "result" || fail "Output didn't match"
}

function setup_file_test() {
  mkdir -p "a/pkg1"
  mkdir -p "a/pkg2"
  cat > a/pkg1/BUILD <<EOF
cc_library(name = "foo")
cc_library(name = "bar")
EOF
  cat > a/pkg2/BUILD << EOF
cc_library(name = "foo")
cc_library(name = "bar")
EOF

  echo -n "
new cc_library baz|//a/pkg1:__pkg__
add deps a|//a/pkg1:baz
add deps a|a/pkg1:foo
add deps x#|a/pkg2:foo
# add deps y|a/pkg1:foo
  # add deps y|a/pkg2:foo

add deps y|a/pkg2:bar|add deps c|a/pkg1:foo
add deps z|a/pkg2:bar
add deps a|//a/pkg1:bar
add deps b|a/pkg1:foo" > commands
}

function check_file_test() {
  # Check that all dependencies have been added
  cat > expected_pkg_1 <<EOF
cc_library(
    name = "foo",
    deps = [
        "a",
        "b",
        "c",
        "y",
    ],
)

cc_library(
    name = "bar",
    deps = ["a"],
)

cc_library(
    name = "baz",
    deps = ["a"],
)
EOF
  diff -u expected_pkg_1 a/pkg1/BUILD || fail "Output didn't match"

  cat > expected_pkg_2 <<EOF
cc_library(
    name = "foo",
    deps = ["x#"],
)

cc_library(
    name = "bar",
    deps = [
        "c",
        "y",
        "z",
    ],
)
EOF
  diff -u expected_pkg_2 a/pkg2/BUILD || fail "Output didn't match"
}

# Test reading commands from stdin
function test_file() {
  setup_file_test
  $buildozer --buildifier= -f - < commands
  check_file_test

  setup_file_test
  $buildozer --buildifier= -f commands
  check_file_test
}

# Test for selectively adding to specific rule types
function test_add_dep_filtered() {
  in='go_library(
    name = "edit",
    deps = ["//pkg:a"],
)'
  ERROR=3 run "$in" -types='cc_library' 'add deps //pkg:b' '//pkg:edit'
  assert_equals "$in"
}

function test_add_dep_unfiltered() {
  in='cc_library(
    name = "edit",
    deps = ["//pkg:a"],
)'
  run "$in" -types='cc_library' 'add deps //pkg:b' '//pkg:edit'
  assert_equals 'cc_library(
    name = "edit",
    deps = [
        ":b",
        "//pkg:a",
    ],
)'
}

function test_add_library_always_unfiltered() {
  in='cc_library(
    name = "edit",
    deps = ["//pkg:a"],
)'
  run "$in" -types='go_library' 'new cc_library a' '//pkg:__pkg__'
  assert_equals 'cc_library(
    name = "edit",
    deps = ["//pkg:a"],
)

cc_library(name = "a")'
}

function test_new_load_after_package() {
in='# Comment

package(default_visibility = ["//visibility:public"])

x(
    name="x",
    srcs=["x.cc"],
)'

  run "$in" 'new_load /foo/bar x y z' '//pkg:x'
  assert_equals '# Comment

load("/foo/bar", "x", "y", "z")

package(default_visibility = ["//visibility:public"])

x(
    name = "x",
    srcs = ["x.cc"],
)'
}


function test_new_load_no_package() {
in='x(
    name="x",
    srcs=["x.cc"],
)'

  run "$in" 'new_load /foo/bar x y z' '//pkg:x'
  assert_equals 'load("/foo/bar", "x", "y", "z")

x(
    name = "x",
    srcs = ["x.cc"],
)'
}


function test_new_load_empty_file() {
  run '' 'new_load /foo/bar x y z' pkg/BUILD
  assert_equals 'load("/foo/bar", "x", "y", "z")'
}

function test_new_load_only_comments() {
in='# Just comments
# And more comments'

  run "$in" 'new_load /foo/bar x y z' pkg/BUILD
  assert_equals '# Just comments
# And more comments

load("/foo/bar", "x", "y", "z")'
}

function test_new_load_existing() {
in='load("/foo/bar", "y")
'

  run "$in" 'new_load /foo/bar z x y' pkg/BUILD
  assert_equals 'load("/foo/bar", "x", "y", "z")'
}

function test_new_load_existing_multiple() {
in='load("/foo/bar", "x")
load("/foo/bar", "y")
'

  run "$in" 'new_load /foo/bar x y z' pkg/BUILD
  assert_equals 'load("/foo/bar", "x")
load("/foo/bar", "y", "z")'
}

function test_new_load_wrong_location() {
in='load("/foo/bar", "x")
'

  run "$in" 'new_load /baz/bam y z' pkg/BUILD
  assert_equals 'load("/baz/bam", "y", "z")
load("/foo/bar", "x")'
}

function test_print_attribute_value_with_spaces() {
  in='cc_test(name = "a", deprecation = "one two three")'
  run "$in" 'print deprecation' '//pkg:a'
  assert_output '"one two three"'
}

function test_insert_into_variable() {
in='some_var = ["a"]
some_rule(name="r", deps=some_var)'

  run "$in" --edit-variables 'add deps b' '//pkg:r'
  assert_equals 'some_var = [
    "a",
    "b",
]

some_rule(
    name = "r",
    deps = some_var,
)'
}

function test_insert_into_variable_by_adding() {
in='some_var = glob(["x"])
some_rule(name="r", deps=some_var)'

  run "$in" --edit-variables "add deps b" '//pkg:r'
  assert_equals 'some_var = glob(["x"]) + ["b"]

some_rule(
    name = "r",
    deps = some_var,
)'
}

function test_insert_into_same_variable_twice() {
in='some_var = ["a"]
some_rule(name="r1", deps=some_var)
some_rule(name="r2", deps=some_var)'

  run "$in" --edit-variables "add deps b" '//pkg:r1' '//pkg:r2'
  assert_equals 'some_var = [
    "a",
    "b",
]

some_rule(
    name = "r1",
    deps = some_var,
)

some_rule(
    name = "r2",
    deps = some_var,
)'
}

function test_pkg_recursive_wildcard() {
mkdir -p foo/bar/baz
mkdir -p foo/abc

cat > foo/BUILD <<EOF
matching_rule(name = "r1", deps = [":bar"])
EOF
cat > foo/bar/BUILD <<EOF
non_matching_rule(name = "r2", deps = [":bar"])
EOF
cat > foo/bar/baz/BUILD <<EOF
matching_rule(name = "r3", deps = [":bar"])
EOF
cat > foo/abc/BUILD <<EOF
matching_rule(name = "r4", deps = [":bar"])
EOF

run_with_current_workspace "$buildozer --buildifier=" "add deps new_dep" "//foo/...:%matching_rule"
assert_no_err "rule '%matching_rule' not found"

assert_equals 'matching_rule(
    name = "r1",
    deps = [
        "new_dep",
        ":bar",
    ],
)' foo

assert_equals 'non_matching_rule(name = "r2", deps = [":bar"])' foo/bar

assert_equals 'matching_rule(
    name = "r3",
    deps = [
        "new_dep",
        ":bar",
    ],
)' foo/bar/baz

assert_equals 'matching_rule(
    name = "r4",
    deps = [
        "new_dep",
        ":bar",
    ],
)' foo/abc
}

function test_add_dep_string() {
  # Add a string without quotes, a quoted string (should be unquoted automatically)
  # and a quoted string with quotes inside (should be unquoted just once).
  run "$one_dep" 'add deps //foo "//bar" "\"//baz\""' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = [
        "\"//baz\"",
        "//bar",
        "//buildifier:build",
        "//foo",
    ],
)'
}

function test_remove_dep_string() {
  # Remove quoted and unquoted strings to make sure that `add` and `remove`
  # unquote them equally
  run "$quoted_deps" 'remove deps //foo "//bar" "\"//baz\""' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    deps = ["//buildifier:build"],
)'
}

function test_set_config_string() {
  run "$one_dep" 'set config "foo"' '//pkg:edit'
  assert_equals 'go_library(
    name = "edit",
    config = "foo",
    deps = ["//buildifier:build"],
)'
}

function test_fix_unused_load() {
  run 'load(":a.bzl", "a")
# TODO: refactor

# begin loads

load(":foo.bzl", "foo")  # foo
load(":foobar.bzl", "foobar")  # this one is actually used
load(":baz.bzl", "baz")  # this is @unused
load(":bar.bzl", "bar")  # bar

# end loads

# before
load(":qux.bzl", "qux")
# after

foobar()' 'fix unusedLoads' 'pkg/BUILD'
  assert_equals '# TODO: refactor

# begin loads

load(":foobar.bzl", "foobar")  # this one is actually used
load(":baz.bzl", "baz")  # this is @unused

# end loads

# before
# after

foobar()'
}

function test_commands_with_targets() {
  mkdir -p pkg1
  mkdir -p pkg2

  cat > pkg1/BUILD <<EOF
rule(name = "r1", deps = [":bar"], compatible_with=["//env:a"])
EOF
  cat > pkg2/BUILD <<EOF
rule(name = "r2", compatible_with=["//env:a"])
EOF

  cat > commands <<EOF
remove compatible_with //env:a|*
add deps :baz|*
EOF
  $buildozer --buildifier= -f commands pkg1:* pkg2:*
  assert_equals 'rule(
    name = "r1",
    deps = [
        ":bar",
        ":baz",
    ],
)' pkg1
  assert_equals 'rule(
    name = "r2",
    deps = [":baz"],
)' pkg2
}

run_suite "buildozer tests"

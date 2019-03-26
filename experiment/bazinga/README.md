# Bazinga

Bazinga (shell done) is a Golang replacement for shell2junit.

## Quick start

Build `bazinga` with `make` and execute a few quick tests:

**A successful test**

```shell
$ ./bazinga -- /bin/sh -c "echo success"
success

$ echo "${?}"
0

$ cat junit_1.xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite failures="0" tests="1" time="0.005793318">
    <testcase name="/bin/sh" time="0.005793318"></testcase>
  </testsuite>
</testsuites>
```

**A non-zero exit code**

```shell
$ ./bazinga -- /bin/sh -c "echo failed; exit 3"
failed

$ echo "${?}"
3

$ cat junit_1.xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite failures="1" tests="1" time="0.007376302">
    <testcase name="/bin/sh" time="0.007376302">
      <failure message="3" type="exitcode"><![CDATA[failed
]]></failure>
    </testcase>
  </testsuite>
</testsuites>
```

## API Config

The `bazinga` program can be configured with a Kubernetes API configuration document, for example:

```shell
$ ./bazinga -config examples/readme-config.yaml
total 24288
drwxr-xr-x  12 akutz  staff       384 Mar 26 03:50 .
drwxr-xr-x  59 akutz  staff      1888 Mar 24 11:58 ..
-rw-r--r--   1 akutz  staff        12 Mar 25 18:28 .gitignore
-rw-r--r--   1 akutz  staff       269 Mar 25 21:15 Makefile
-rw-r--r--   1 akutz  staff       112 Mar 25 13:17 OWNERS
-rw-r--r--   1 akutz  staff      1628 Mar 26 03:46 README.md
-rwxr-xr-x   1 akutz  staff  12410264 Mar 26 03:50 bazinga
drwxr-xr-x   3 akutz  staff        96 Mar 26 03:45 examples
drwxr-xr-x   3 akutz  staff        96 Mar 24 12:34 hack
-rw-r--r--   1 akutz  staff       460 Mar 26 03:50 junit_1.xml
-rw-r--r--   1 akutz  staff      1553 Mar 25 22:21 main.go
drwxr-xr-x   5 akutz  staff       160 Mar 25 18:25 pkg
why
has this test [FAILED]?
with not just one [error], but
two!

$ echo "${?}"
1

$ cat junit_1.xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite failures="2" tests="2" time="0.024370006">
    <testcase name="list-files" time="0.014263514"></testcase>
    <testcase name="test-failure-conditions" time="0.010106492">
      <failure message="" type="FAILURE"><![CDATA[has this test [FAILED]?]]></failure>
      <failure message="" type="ERROR"><![CDATA[with not just one [error], but]]></failure>
    </testcase>
  </testsuite>
</testsuites>
```

## Regexp Flags

The flags used with a failure condition are a bitmask defined in Golang's [`regexp/syntax.Flags`](https://golang.org/pkg/regexp/syntax/#Flags) type. For convenience, here is a list of the values for the package's defined flags and flag combinations:

```
FoldCase            1
Literal             2
ClassNL             4
DotNL               8
OneLine            16
NonGreedy          32
PerlX              64
UnicodeGroups     128
WasDollar         256
Simple            512
MatchNL            12
Perl              212
POSIX               0
```

## TODO

* Bazel-fy this directory and its sub-directories
* Remove the Makefile
* Add more documentation
* ??

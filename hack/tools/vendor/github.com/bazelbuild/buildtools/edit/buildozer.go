/*
Copyright 2016 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Buildozer is a tool for programmatically editing BUILD files.

package edit

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	apipb "github.com/bazelbuild/buildtools/api_proto"
	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/file"
	"github.com/bazelbuild/buildtools/labels"
	"github.com/bazelbuild/buildtools/wspace"
	"github.com/golang/protobuf/proto"
)

// Options represents choices about how buildozer should behave.
type Options struct {
	Stdout            bool      // write changed BUILD file to stdout
	Buildifier        string    // path to buildifier binary
	Parallelism       int       // number of cores to use for concurrent actions
	NumIO             int       // number of concurrent actions
	CommandsFiles     []string  // file names to read commands from, use '-' for stdin (format:|-separated command line arguments to buildozer, excluding flags
	KeepGoing         bool      // apply all commands, even if there are failures
	FilterRuleTypes   []string  // list of rule types to change, empty means all
	PreferEOLComments bool      // when adding a new comment, put it on the same line if possible
	RootDir           string    // If present, use this folder rather than $PWD to find the root dir
	Quiet             bool      // suppress informational messages.
	EditVariables     bool      // for attributes that simply assign a variable (e.g. hdrs = LIB_HDRS), edit the build variable instead of appending to the attribute.
	IsPrintingProto   bool      // output serialized devtools.buildozer.Output protos instead of human-readable strings
	OutWriter         io.Writer // where to write normal output (`os.Stdout` will be used if not specified)
	ErrWriter         io.Writer // where to write error output (`os.Stderr` will be used if not specified)
}

// NewOpts returns a new Options struct with some defaults set.
func NewOpts() *Options {
	return &Options{NumIO: 200, PreferEOLComments: true}
}

// Usage is a user-overridden func to print the program usage.
var Usage = func() {}

const stdinPackageName = "-" // the special package name to represent stdin

// CmdEnvironment stores the information the commands below have access to.
type CmdEnvironment struct {
	File   *build.File                  // the AST
	Rule   *build.Rule                  // the rule to modify
	Vars   map[string]*build.AssignExpr // global variables set in the build file
	Pkg    string                       // the full package name
	Args   []string                     // the command-line arguments
	output *apipb.Output_Record         // output proto, stores whatever a command wants to print
}

// The cmdXXX functions implement the various commands.

func cmdAdd(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	for _, val := range env.Args[1:] {
		if IsIntList(attr) {
			AddValueToListAttribute(env.Rule, attr, env.Pkg, &build.LiteralExpr{Token: val}, &env.Vars)
			continue
		}
		strVal := getStringExpr(val, env.Pkg)
		AddValueToListAttribute(env.Rule, attr, env.Pkg, strVal, &env.Vars)
	}
	ResolveAttr(env.Rule, attr, env.Pkg)
	return env.File, nil
}

func cmdComment(opts *Options, env CmdEnvironment) (*build.File, error) {
	// The comment string is always the last argument in the list.
	str := env.Args[len(env.Args)-1]
	str = strings.Replace(str, "\\n", "\n", -1)
	// Multiline comments should go on a separate line.
	fullLine := !opts.PreferEOLComments || strings.Contains(str, "\n")
	comment := []build.Comment{}
	for _, line := range strings.Split(str, "\n") {
		comment = append(comment, build.Comment{Token: "# " + line})
	}

	// The comment might be attached to a rule, an attribute, or a value in a list,
	// depending on how many arguments are passed.
	switch len(env.Args) {
	case 1: // Attach to a rule
		env.Rule.Call.Comments.Before = comment
	case 2: // Attach to an attribute
		if attr := env.Rule.AttrDefn(env.Args[0]); attr != nil {
			if fullLine {
				attr.LHS.Comment().Before = comment
			} else {
				attr.RHS.Comment().Suffix = comment
			}
		}
	case 3: // Attach to a specific value in a list
		if attr := env.Rule.Attr(env.Args[0]); attr != nil {
			if expr := listOrSelectFind(attr, env.Args[1], env.Pkg); expr != nil {
				if fullLine {
					expr.Comments.Before = comment
				} else {
					expr.Comments.Suffix = comment
				}
			}
		}
	default:
		panic("cmdComment")
	}
	return env.File, nil
}

// commentsText concatenates comments into a single line.
func commentsText(comments []build.Comment) string {
	var segments []string
	for _, comment := range comments {
		token := comment.Token
		if strings.HasPrefix(token, "#") {
			token = token[1:]
		}
		segments = append(segments, strings.TrimSpace(token))
	}
	return strings.Replace(strings.Join(segments, " "), "\n", " ", -1)
}

func cmdPrintComment(opts *Options, env CmdEnvironment) (*build.File, error) {
	attrError := func() error {
		return fmt.Errorf("rule \"//%s:%s\" has no attribute \"%s\"", env.Pkg, env.Rule.Name(), env.Args[0])
	}

	switch len(env.Args) {
	case 0: // Print rule comment.
		env.output.Fields = []*apipb.Output_Record_Field{
			{Value: &apipb.Output_Record_Field_Text{commentsText(env.Rule.Call.Comments.Before)}},
		}
		if text := commentsText(env.Rule.Call.Comments.Suffix); text != "" {
			env.output.Fields = append(env.output.Fields, &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{text}})
		}
		if text := commentsText(env.Rule.Call.Comments.After); text != "" {
			env.output.Fields = append(env.output.Fields, &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{text}})
		}
	case 1: // Print attribute comment.
		attr := env.Rule.AttrDefn(env.Args[0])
		if attr == nil {
			return nil, attrError()
		}
		comments := append(attr.Before, attr.Suffix...)
		env.output.Fields = []*apipb.Output_Record_Field{
			{Value: &apipb.Output_Record_Field_Text{commentsText(comments)}},
		}
	case 2: // Print comment of a specific value in a list.
		attr := env.Rule.Attr(env.Args[0])
		if attr == nil {
			return nil, attrError()
		}
		value := env.Args[1]
		expr := listOrSelectFind(attr, value, env.Pkg)
		if expr == nil {
			return nil, fmt.Errorf("attribute \"%s\" has no value \"%s\"", env.Args[0], value)
		}
		comments := append(expr.Comments.Before, expr.Comments.Suffix...)
		env.output.Fields = []*apipb.Output_Record_Field{
			{Value: &apipb.Output_Record_Field_Text{commentsText(comments)}},
		}
	default:
		panic("cmdPrintComment")
	}
	return nil, nil
}

func cmdDelete(opts *Options, env CmdEnvironment) (*build.File, error) {
	return DeleteRule(env.File, env.Rule), nil
}

func cmdMove(opts *Options, env CmdEnvironment) (*build.File, error) {
	oldAttr := env.Args[0]
	newAttr := env.Args[1]
	if len(env.Args) == 3 && env.Args[2] == "*" {
		if err := MoveAllListAttributeValues(env.Rule, oldAttr, newAttr, env.Pkg, &env.Vars); err != nil {
			return nil, err
		}
		return env.File, nil
	}
	fixed := false
	for _, val := range env.Args[2:] {
		if deleted := ListAttributeDelete(env.Rule, oldAttr, val, env.Pkg); deleted != nil {
			AddValueToListAttribute(env.Rule, newAttr, env.Pkg, deleted, &env.Vars)
			fixed = true
		}
	}
	if fixed {
		return env.File, nil
	}
	return nil, nil
}

func cmdNew(opts *Options, env CmdEnvironment) (*build.File, error) {
	kind := env.Args[0]
	name := env.Args[1]
	addAtEOF, insertionIndex, err := findInsertionIndex(env)
	if err != nil {
		return nil, err
	}

	if FindRuleByName(env.File, name) != nil {
		return nil, fmt.Errorf("rule '%s' already exists", name)
	}

	call := &build.CallExpr{X: &build.Ident{Name: kind}}
	rule := &build.Rule{call, ""}
	rule.SetAttr("name", &build.StringExpr{Value: name})

	if addAtEOF {
		env.File.Stmt = InsertAfterLastOfSameKind(env.File.Stmt, rule.Call)
	} else {
		env.File.Stmt = InsertAfter(insertionIndex, env.File.Stmt, call)
	}
	return env.File, nil
}

// findInsertionIndex is used by cmdNew to find the place at which to insert the new rule.
func findInsertionIndex(env CmdEnvironment) (bool, int, error) {
	if len(env.Args) < 4 {
		return true, 0, nil
	}

	relativeToRuleName := env.Args[3]
	ruleIdx, _ := IndexOfRuleByName(env.File, relativeToRuleName)
	if ruleIdx == -1 {
		return true, 0, nil
	}

	switch env.Args[2] {
	case "before":
		return false, ruleIdx - 1, nil
	case "after":
		return false, ruleIdx, nil
	default:
		return true, 0, fmt.Errorf("Unknown relative operator '%s'; allowed: 'before', 'after'", env.Args[1])
	}
}

// splitLoadArgs splits arguments of form <[to=]from>
// into a slice of froms and a slice of tos.
func splitLoadArgs(args []string) ([]string, []string) {
	from := args
	to := append([]string{}, args...)
	for i := range from {
		if s := strings.SplitN(from[i], "=", 2); len(s) == 2 {
			to[i] = s[0]
			from[i] = s[1]
		}
	}

	return from, to
}

func cmdNewLoad(opts *Options, env CmdEnvironment) (*build.File, error) {
	from, to := splitLoadArgs(env.Args[1:])
	env.File.Stmt = InsertLoad(env.File.Stmt, env.Args[0], from, to)
	return env.File, nil
}

func cmdReplaceLoad(opts *Options, env CmdEnvironment) (*build.File, error) {
	from, to := splitLoadArgs(env.Args[1:])
	env.File.Stmt = ReplaceLoad(env.File.Stmt, env.Args[0], from, to)
	return env.File, nil
}

func cmdSubstituteLoad(opts *Options, env CmdEnvironment) (*build.File, error) {
	oldRegexp, err := regexp.Compile(env.Args[0])
	if err != nil {
		return nil, err
	}
	newTemplate := env.Args[1]

	for _, stmt := range env.File.Stmt {
		load, ok := stmt.(*build.LoadStmt)
		if !ok {
			continue
		}

		if newValue, ok := stringSubstitute(load.Module.Value, oldRegexp, newTemplate); ok {
			load.Module.Value = newValue
		}
	}

	return env.File, nil
}

func cmdPrint(opts *Options, env CmdEnvironment) (*build.File, error) {
	format := env.Args
	if len(format) == 0 {
		format = []string{"name", "kind"}
	}
	fields := make([]*apipb.Output_Record_Field, len(format))

	for i, str := range format {
		value := env.Rule.Attr(str)
		if str == "kind" {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{env.Rule.Kind()}}
		} else if str == "name" {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{env.Rule.Name()}}
		} else if str == "label" {
			if env.Rule.Name() != "" {
				label := labels.Label{Package: env.Pkg, Target: env.Rule.Name()}
				fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{label.Format()}}
			} else {
				return nil, nil
			}
		} else if str == "rule" {
			fields[i] = &apipb.Output_Record_Field{
				Value: &apipb.Output_Record_Field_Text{build.FormatString(env.Rule.Call)},
			}
		} else if str == "startline" {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Number{int32(env.Rule.Call.ListStart.Line)}}
		} else if str == "endline" {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Number{int32(env.Rule.Call.End.Pos.Line)}}
		} else if str == "path" {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{env.File.Path}}
		} else if value == nil {
			fmt.Fprintf(opts.ErrWriter, "rule \"//%s:%s\" has no attribute \"%s\"\n",
				env.Pkg, env.Rule.Name(), str)
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Error{Error: apipb.Output_Record_Field_MISSING}}
		} else if lit, ok := value.(*build.LiteralExpr); ok {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{lit.Token}}
		} else if lit, ok := value.(*build.Ident); ok {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{lit.Name}}
		} else if string, ok := value.(*build.StringExpr); ok {
			fields[i] = &apipb.Output_Record_Field{
				Value:             &apipb.Output_Record_Field_Text{string.Value},
				QuoteWhenPrinting: true,
			}
		} else if strList := env.Rule.AttrStrings(str); strList != nil {
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_List{List: &apipb.RepeatedString{Strings: strList}}}
		} else {
			// Some other Expr we haven't listed above. Just print it.
			fields[i] = &apipb.Output_Record_Field{Value: &apipb.Output_Record_Field_Text{build.FormatString(value)}}
		}
	}

	env.output.Fields = fields
	return nil, nil
}

func attrKeysForPattern(rule *build.Rule, pattern string) []string {
	if pattern == "*" {
		return rule.AttrKeys()
	}
	return []string{pattern}
}

func cmdRemove(opts *Options, env CmdEnvironment) (*build.File, error) {
	if len(env.Args) == 1 { // Remove the attribute
		if env.Rule.DelAttr(env.Args[0]) != nil {
			return env.File, nil
		}
	} else { // Remove values in the attribute.
		fixed := false
		for _, key := range attrKeysForPattern(env.Rule, env.Args[0]) {
			for _, val := range env.Args[1:] {
				ListAttributeDelete(env.Rule, key, val, env.Pkg)
				fixed = true
			}
			ResolveAttr(env.Rule, key, env.Pkg)
			// Remove the attribute if's an empty list
			if listExpr, ok := env.Rule.Attr(key).(*build.ListExpr); ok && len(listExpr.List) == 0 {
				env.Rule.DelAttr(key)
			}
		}
		if fixed {
			return env.File, nil
		}
	}
	return nil, nil
}

func cmdRemoveComment(opts *Options, env CmdEnvironment) (*build.File, error) {
	switch len(env.Args) {
	case 0: // Remove comment attached to rule
		env.Rule.Call.Comments.Before = nil
		env.Rule.Call.Comments.Suffix = nil
		env.Rule.Call.Comments.After = nil
	case 1: // Remove comment attached to attr
		if attr := env.Rule.AttrDefn(env.Args[0]); attr != nil {
			attr.Comments.Before = nil
			attr.Comments.Suffix = nil
			attr.Comments.After = nil
			attr.LHS.Comment().Before = nil
			attr.LHS.Comment().Suffix = nil
			attr.LHS.Comment().After = nil
			attr.RHS.Comment().Before = nil
			attr.RHS.Comment().Suffix = nil
			attr.RHS.Comment().After = nil
		}
	case 2: // Remove comment attached to value
		if attr := env.Rule.Attr(env.Args[0]); attr != nil {
			if expr := listOrSelectFind(attr, env.Args[1], env.Pkg); expr != nil {
				expr.Comments.Before = nil
				expr.Comments.Suffix = nil
				expr.Comments.After = nil
			}
		}
	default:
		panic("cmdRemoveComment")
	}
	return env.File, nil
}

func cmdRename(opts *Options, env CmdEnvironment) (*build.File, error) {
	oldAttr := env.Args[0]
	newAttr := env.Args[1]
	if err := RenameAttribute(env.Rule, oldAttr, newAttr); err != nil {
		return nil, err
	}
	return env.File, nil
}

func cmdReplace(opts *Options, env CmdEnvironment) (*build.File, error) {
	oldV := getStringValue(env.Args[1])
	newV := getStringValue(env.Args[2])
	for _, key := range attrKeysForPattern(env.Rule, env.Args[0]) {
		attr := env.Rule.Attr(key)
		if e, ok := attr.(*build.StringExpr); ok {
			if labels.Equal(e.Value, oldV, env.Pkg) {
				env.Rule.SetAttr(key, getAttrValueExpr(key, []string{newV}, env))
			}
		} else {
			ListReplace(attr, oldV, newV, env.Pkg)
		}
	}
	return env.File, nil
}

func cmdSubstitute(opts *Options, env CmdEnvironment) (*build.File, error) {
	oldRegexp, err := regexp.Compile(env.Args[1])
	if err != nil {
		return nil, err
	}
	newTemplate := env.Args[2]
	for _, key := range attrKeysForPattern(env.Rule, env.Args[0]) {
		attr := env.Rule.Attr(key)
		e, ok := attr.(*build.StringExpr)
		if !ok {
			ListSubstitute(attr, oldRegexp, newTemplate)
			continue
		}
		if newValue, ok := stringSubstitute(e.Value, oldRegexp, newTemplate); ok {
			env.Rule.SetAttr(key, getAttrValueExpr(key, []string{newValue}, env))
		}
	}
	return env.File, nil
}

func cmdSet(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	args := env.Args[1:]
	if attr == "kind" {
		env.Rule.SetKind(args[0])
	} else {
		env.Rule.SetAttr(attr, getAttrValueExpr(attr, args, env))
	}
	return env.File, nil
}

func cmdSetIfAbsent(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	args := env.Args[1:]
	if attr == "kind" {
		return nil, fmt.Errorf("setting 'kind' is not allowed for set_if_absent. Got %s", env.Args)
	}
	if env.Rule.Attr(attr) == nil {
		env.Rule.SetAttr(attr, getAttrValueExpr(attr, args, env))
	}
	return env.File, nil
}

func getAttrValueExpr(attr string, args []string, env CmdEnvironment) build.Expr {
	switch {
	case attr == "kind":
		return nil
	case IsIntList(attr):
		var list []build.Expr
		for _, i := range args {
			list = append(list, &build.LiteralExpr{Token: i})
		}
		return &build.ListExpr{List: list}
	case IsList(attr) && !(len(args) == 1 && strings.HasPrefix(args[0], "glob(")):
		var list []build.Expr
		for _, arg := range args {
			list = append(list, getStringExpr(arg, env.Pkg))
		}
		return &build.ListExpr{List: list}
	case len(args) == 0:
		// Expected a non-list argument, nothing provided
		return &build.Ident{Name: "None"}
	case IsString(attr):
		return getStringExpr(args[0], env.Pkg)
	default:
		return &build.Ident{Name: args[0]}
	}
}

// getStringValue extracts a string value, which can be either quoted or not, from an input argument
func getStringValue(value string) string {
	if unquoted, _, err := build.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

// getStringExpr creates a StringExpr from an input argument, which can be either quoted or not,
// and shortens the label value if possible.
func getStringExpr(value, pkg string) build.Expr {
	if unquoted, triple, err := build.Unquote(value); err == nil {
		return &build.StringExpr{Value: ShortenLabel(unquoted, pkg), TripleQuote: triple}
	}
	return &build.StringExpr{Value: ShortenLabel(value, pkg)}
}

func cmdCopy(opts *Options, env CmdEnvironment) (*build.File, error) {
	attrName := env.Args[0]
	from := env.Args[1]

	return copyAttributeBetweenRules(env, attrName, from)
}

func cmdCopyNoOverwrite(opts *Options, env CmdEnvironment) (*build.File, error) {
	attrName := env.Args[0]
	from := env.Args[1]

	if env.Rule.Attr(attrName) != nil {
		return env.File, nil
	}

	return copyAttributeBetweenRules(env, attrName, from)
}

// cmdDictAdd adds a key to a dict, if that key does _not_ exit already.
func cmdDictAdd(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	args := env.Args[1:]

	dict := &build.DictExpr{}
	currDict, ok := env.Rule.Attr(attr).(*build.DictExpr)
	if ok {
		dict = currDict
	}

	for _, x := range args {
		kv := strings.SplitN(x, ":", 2)
		expr := getStringExpr(kv[1], env.Pkg)

		prev := DictionaryGet(dict, kv[0])
		if prev == nil {
			// Only set the value if the value is currently unset.
			DictionarySet(dict, kv[0], expr)
		}
	}
	env.Rule.SetAttr(attr, dict)
	return env.File, nil
}

// cmdDictSet adds a key to a dict, overwriting any previous values.
func cmdDictSet(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	args := env.Args[1:]

	dict := &build.DictExpr{}
	currDict, ok := env.Rule.Attr(attr).(*build.DictExpr)
	if ok {
		dict = currDict
	}

	for _, x := range args {
		kv := strings.SplitN(x, ":", 2)
		expr := getStringExpr(kv[1], env.Pkg)
		// Set overwrites previous values.
		DictionarySet(dict, kv[0], expr)
	}
	env.Rule.SetAttr(attr, dict)
	return env.File, nil
}

// cmdDictRemove removes a key from a dict.
func cmdDictRemove(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	args := env.Args[1:]

	thing := env.Rule.Attr(attr)
	dictAttr, ok := thing.(*build.DictExpr)
	if !ok {
		return env.File, nil
	}

	for _, x := range args {
		// should errors here be flagged?
		DictionaryDelete(dictAttr, x)
		env.Rule.SetAttr(attr, dictAttr)
	}

	// If the removal results in the dict having no contents, delete the attribute (stay clean!)
	if dictAttr == nil || len(dictAttr.List) == 0 {
		env.Rule.DelAttr(attr)
	}

	return env.File, nil
}

// cmdDictListAdd adds an item to a list in a dict.
func cmdDictListAdd(opts *Options, env CmdEnvironment) (*build.File, error) {
	attr := env.Args[0]
	key := env.Args[1]
	args := env.Args[2:]

	dict := &build.DictExpr{}
	if currDict, ok := env.Rule.Attr(attr).(*build.DictExpr); ok {
		dict = currDict
	}

	prev := DictionaryGet(dict, key)
	if prev == nil {
		prev = &build.ListExpr{}
	}

	for _, val := range args {
		expr := getStringExpr(val, env.Pkg)
		prev = AddValueToList(prev, env.Pkg, expr, true)
	}

	DictionarySet(dict, key, prev)
	env.Rule.SetAttr(attr, dict)

	return env.File, nil
}

func copyAttributeBetweenRules(env CmdEnvironment, attrName string, from string) (*build.File, error) {
	fromRule := FindRuleByName(env.File, from)
	if fromRule == nil {
		return nil, fmt.Errorf("could not find rule '%s'", from)
	}
	attr := fromRule.Attr(attrName)
	if attr == nil {
		return nil, fmt.Errorf("rule '%s' does not have attribute '%s'", from, attrName)
	}

	ast, err := build.ParseBuild("" /* filename */, []byte(build.FormatString(attr)))
	if err != nil {
		return nil, fmt.Errorf("could not parse attribute value %v", build.FormatString(attr))
	}

	env.Rule.SetAttr(attrName, ast.Stmt[0])
	return env.File, nil
}

func cmdFix(opts *Options, env CmdEnvironment) (*build.File, error) {
	// Fix the whole file
	if env.Rule.Kind() == "package" {
		return FixFile(env.File, env.Pkg, env.Args), nil
	}
	// Fix a specific rule
	return FixRule(env.File, env.Pkg, env.Rule, env.Args), nil
}

// CommandInfo provides a command function and info on incoming arguments.
type CommandInfo struct {
	Fn       func(*Options, CmdEnvironment) (*build.File, error)
	PerRule  bool
	MinArg   int
	MaxArg   int
	Template string
}

// AllCommands associates the command names with their function and number
// of arguments.
var AllCommands = map[string]CommandInfo{
	"add":               {cmdAdd, true, 2, -1, "<attr> <value(s)>"},
	"new_load":          {cmdNewLoad, false, 1, -1, "<path> <[to=]from(s)>"},
	"replace_load":      {cmdReplaceLoad, false, 1, -1, "<path> <[to=]symbol(s)>"},
	"substitute_load":   {cmdSubstituteLoad, false, 2, 2, "<old_regexp> <new_template>"},
	"comment":           {cmdComment, true, 1, 3, "<attr>? <value>? <comment>"},
	"print_comment":     {cmdPrintComment, true, 0, 2, "<attr>? <value>?"},
	"delete":            {cmdDelete, true, 0, 0, ""},
	"fix":               {cmdFix, true, 0, -1, "<fix(es)>?"},
	"move":              {cmdMove, true, 3, -1, "<old_attr> <new_attr> <value(s)>"},
	"new":               {cmdNew, false, 2, 4, "<rule_kind> <rule_name> [(before|after) <relative_rule_name>]"},
	"print":             {cmdPrint, true, 0, -1, "<attribute(s)>"},
	"remove":            {cmdRemove, true, 1, -1, "<attr> <value(s)>"},
	"remove_comment":    {cmdRemoveComment, true, 0, 2, "<attr>? <value>?"},
	"rename":            {cmdRename, true, 2, 2, "<old_attr> <new_attr>"},
	"replace":           {cmdReplace, true, 3, 3, "<attr> <old_value> <new_value>"},
	"substitute":        {cmdSubstitute, true, 3, 3, "<attr> <old_regexp> <new_template>"},
	"set":               {cmdSet, true, 1, -1, "<attr> <value(s)>"},
	"set_if_absent":     {cmdSetIfAbsent, true, 1, -1, "<attr> <value(s)>"},
	"copy":              {cmdCopy, true, 2, 2, "<attr> <from_rule>"},
	"copy_no_overwrite": {cmdCopyNoOverwrite, true, 2, 2, "<attr> <from_rule>"},
	"dict_add":          {cmdDictAdd, true, 2, -1, "<attr> <(key:value)(s)>"},
	"dict_set":          {cmdDictSet, true, 2, -1, "<attr> <(key:value)(s)>"},
	"dict_remove":       {cmdDictRemove, true, 2, -1, "<attr> <key(s)>"},
	"dict_list_add":     {cmdDictListAdd, true, 3, -1, "<attr> <key> <value(s)>"},
}

var readonlyCommands = map[string]bool{
	"print":         true,
	"print_comment": true,
}

func expandTargets(f *build.File, rule string) ([]*build.Rule, error) {
	if r := FindRuleByName(f, rule); r != nil {
		return []*build.Rule{r}, nil
	} else if r := FindExportedFile(f, rule); r != nil {
		return []*build.Rule{r}, nil
	} else if rule == "all" || rule == "*" {
		// "all" is a valid name, it is a wildcard only if no such rule is found.
		return f.Rules(""), nil
	} else if strings.HasPrefix(rule, "%") {
		// "%java_library" will match all java_library functions in the package
		// "%<LINENUM>" will match the rule which begins at LINENUM.
		// This is for convenience, "%" is not a valid character in bazel targets.
		kind := rule[1:]
		if linenum, err := strconv.Atoi(kind); err == nil {
			if r := f.RuleAt(linenum); r != nil {
				return []*build.Rule{r}, nil
			}
		} else {
			return f.Rules(kind), nil
		}
	}
	return nil, fmt.Errorf("rule '%s' not found", rule)
}

func filterRules(opts *Options, rules []*build.Rule) (result []*build.Rule) {
	if len(opts.FilterRuleTypes) == 0 {
		return rules
	}
	for _, rule := range rules {
		for _, filterType := range opts.FilterRuleTypes {
			if rule.Kind() == filterType {
				result = append(result, rule)
				break
			}
		}
	}
	return
}

// command contains a list of tokens that describe a buildozer command.
type command struct {
	tokens []string
}

// checkCommandUsage checks the number of argument of a command.
// It prints an error and usage when it is not valid.
func checkCommandUsage(opts *Options, name string, cmd CommandInfo, count int) {
	if count >= cmd.MinArg && (cmd.MaxArg == -1 || count <= cmd.MaxArg) {
		return
	}

	if count < cmd.MinArg {
		fmt.Fprintf(opts.ErrWriter, "Too few arguments for command '%s', expected at least %d.\n",
			name, cmd.MinArg)
	} else {
		fmt.Fprintf(opts.ErrWriter, "Too many arguments for command '%s', expected at most %d.\n",
			name, cmd.MaxArg)
	}
	Usage()
	os.Exit(1)
}

// Match text that only contains spaces or line breaks if they're escaped with '\'.
var spaceRegex = regexp.MustCompile(`(\\ |\\\n|[^ \n])+`)

// SplitOnSpaces behaves like strings.Fields, except that spaces can be escaped.
// Also splits on linebreaks unless they are escaped too.
// " some dummy\\ string" -> ["some", "dummy string"]
func SplitOnSpaces(input string) []string {
	result := spaceRegex.FindAllString(input, -1)
	for i, s := range result {
		s = strings.Replace(s, `\ `, " ", -1)
		s = strings.Replace(s, "\\\n", "\n", -1)
		result[i] = s
	}
	return result
}

// parseCommands parses commands and targets they should be applied on from
// a list of arguments.
// Each argument can be either:
// - a command (as defined by AllCommands) and its parameters, separated by
//   whitespace
// - a target all commands that are parsed during one call to parseCommands
//   should be applied on
func parseCommands(opts *Options, args []string) (commands []command, targets []string) {
	for _, arg := range args {
		commandTokens := SplitOnSpaces(arg)
		cmd, found := AllCommands[commandTokens[0]]
		if found {
			checkCommandUsage(opts, commandTokens[0], cmd, len(commandTokens)-1)
			commands = append(commands, command{commandTokens})
		} else {
			targets = append(targets, arg)
		}
	}
	return
}

// commandsForTarget contains commands to be executed on the given target.
type commandsForTarget struct {
	target   string
	commands []command
}

// commandsForFile contains the file name and all commands that should be
// applied on that file, indexed by their target.
type commandsForFile struct {
	file     string
	commands []commandsForTarget
}

// commandError returns an error that formats 'err' in the context of the
// commands to be executed on the given target.
func commandError(commands []command, target string, err error) error {
	return fmt.Errorf("error while executing commands %s on target %s: %s", commands, target, err)
}

// rewriteResult contains the outcome of applying fixes to a single file.
type rewriteResult struct {
	file     string
	errs     []error
	modified bool
	records  []*apipb.Output_Record
}

// getGlobalVariables returns the global variable assignments in the provided list of expressions.
// That is, for each variable assignment of the form
//   a = v
// vars["a"] will contain the AssignExpr whose RHS value is the assignment "a = v".
func getGlobalVariables(exprs []build.Expr) (vars map[string]*build.AssignExpr) {
	vars = make(map[string]*build.AssignExpr)
	for _, expr := range exprs {
		if as, ok := expr.(*build.AssignExpr); ok {
			if lhs, ok := as.LHS.(*build.Ident); ok {
				vars[lhs.Name] = as
			}
		}
	}
	return vars
}

// When checking the filesystem, we need to look for any of the
// possible BuildFileNames. For historical reasons, the
// parts of the tool that generate paths that we may want to examine
// continue to assume that build files are all named "BUILD".

// BuildFileNames is exported so that users that want to override it
// in scripts are free to do so.
var BuildFileNames = [...]string{"BUILD.bazel", "BUILD", "BUCK"}

// Buildifier formats the build file using the buildifier logic.
type Buildifier interface {
	// Buildify formats the build file and returns the formatted contents.
	Buildify(*Options, *build.File) ([]byte, error)
}

var buildifier Buildifier = &defaultBuildifier{}
var buildifierRegistered = false

// RegisterBuildifier replaces the default buildifier with an
// alternative implementation.
//
// It may only be called once.
func RegisterBuildifier(b Buildifier) {
	if buildifierRegistered {
		panic("Only one call to RegisterBuildifier is allowed.")
	}
	buildifier = b
	buildifierRegistered = true
}

// rewrite parses the BUILD file for the given file, transforms the AST,
// and write the changes back in the file (or on stdout).
func rewrite(opts *Options, commandsForFile commandsForFile) *rewriteResult {
	name := commandsForFile.file
	var data []byte
	var err error
	var fi os.FileInfo
	records := []*apipb.Output_Record{}
	if name == stdinPackageName { // read on stdin
		data, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return &rewriteResult{file: name, errs: []error{err}}
		}
	} else {
		origName := name
		for _, suffix := range BuildFileNames {
			if strings.HasSuffix(name, "/"+suffix) {
				name = strings.TrimSuffix(name, suffix)
				break
			}
		}
		for _, suffix := range BuildFileNames {
			name = name + suffix
			data, fi, err = file.ReadFile(name)
			if err == nil {
				break
			}
			name = strings.TrimSuffix(name, suffix)
		}
		if err != nil {
			data, fi, err = file.ReadFile(name)
		}
		if err != nil {
			err = errors.New("file not found or not readable")
			return &rewriteResult{file: origName, errs: []error{err}}
		}
	}

	f, err := build.ParseBuild(name, data)
	if err != nil {
		return &rewriteResult{file: name, errs: []error{err}}
	}
	f.WorkspaceRoot, f.Pkg, f.Label = wspace.SplitFilePath(name)

	vars := map[string]*build.AssignExpr{}
	if opts.EditVariables {
		vars = getGlobalVariables(f.Stmt)
	}
	var errs []error
	changed := false
	for _, commands := range commandsForFile.commands {
		target := commands.target
		commands := commands.commands
		_, absPkg, rule := InterpretLabelForWorkspaceLocation(opts.RootDir, target)
		if label := labels.Parse(target); label.Package == stdinPackageName {
			// Special-case: This is already absolute
			absPkg = stdinPackageName
		}
		if strings.HasSuffix(absPkg, "...") {
			// Special case: the provided target contains an ellipsis, use the file package
			absPkg = f.Pkg
		}

		targets, err := expandTargets(f, rule)
		if err != nil {
			cerr := commandError(commands, target, err)
			errs = append(errs, cerr)
			if !opts.KeepGoing {
				return &rewriteResult{file: name, errs: errs, records: records}

			}
		}
		targets = filterRules(opts, targets)
		for _, cmd := range commands {
			cmdInfo := AllCommands[cmd.tokens[0]]
			// Depending on whether a transformation is rule-specific or not, it should be applied to
			// every rule that satisfies the filter or just once to the file.
			cmdTargets := targets
			if !cmdInfo.PerRule {
				cmdTargets = []*build.Rule{nil}
			}
			for _, r := range cmdTargets {
				record := &apipb.Output_Record{}
				newf, err := cmdInfo.Fn(opts, CmdEnvironment{f, r, vars, absPkg, cmd.tokens[1:], record})
				if len(record.Fields) != 0 {
					records = append(records, record)
				}
				if err != nil {
					cerr := commandError([]command{cmd}, target, err)
					if opts.KeepGoing {
						errs = append(errs, cerr)
					} else {
						return &rewriteResult{file: name, errs: []error{cerr}, records: records}
					}
				}
				if newf != nil {
					changed = true
					f = newf
				}
			}
		}
	}
	if !changed {
		return &rewriteResult{file: name, errs: errs, records: records}
	}
	f = RemoveEmptyPackage(f)
	ndata, err := buildifier.Buildify(opts, f)
	if err != nil {
		return &rewriteResult{file: name, errs: []error{fmt.Errorf("running buildifier: %v", err)}, records: records}
	}

	if opts.Stdout || name == stdinPackageName {
		opts.OutWriter.Write(ndata)
		return &rewriteResult{file: name, errs: errs, records: records}
	}

	if bytes.Equal(data, ndata) {
		return &rewriteResult{file: name, errs: errs, records: records}
	}

	if err := EditFile(fi, name); err != nil {
		return &rewriteResult{file: name, errs: []error{err}, records: records}
	}

	if err := file.WriteFile(name, ndata); err != nil {
		return &rewriteResult{file: name, errs: []error{err}, records: records}
	}

	return &rewriteResult{file: name, errs: errs, modified: true, records: records}
}

// EditFile is a function that does any prework needed before editing a file.
// e.g. "checking out for write" from a locking source control repo.
var EditFile = func(fi os.FileInfo, name string) error {
	return nil
}

// Given a target, whose package may contain a trailing "/...", returns all
// existing BUILD file paths which match the package.
func targetExpressionToBuildFiles(rootDir string, target string) []string {
	file, _, _ := InterpretLabelForWorkspaceLocation(rootDir, target)
	if rootDir == "" {
		var err error
		if file, err = filepath.Abs(file); err != nil {
			fmt.Printf("Cannot make path absolute: %s\n", err.Error())
			os.Exit(1)
		}
	}

	suffix := filepath.Join("", "...", "BUILD") // /.../BUILD
	if !strings.HasSuffix(file, suffix) {
		return []string{file}
	}

	return findBuildFiles(strings.TrimSuffix(file, suffix))
}

// Given a root directory, returns all "BUILD" files in that subtree recursively.
func findBuildFiles(rootDir string) []string {
	var buildFiles []string
	searchDirs := []string{rootDir}

	for len(searchDirs) != 0 {
		lastIndex := len(searchDirs) - 1
		dir := searchDirs[lastIndex]
		searchDirs = searchDirs[:lastIndex]

		dirFiles, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, dirFile := range dirFiles {
			if dirFile.IsDir() {
				searchDirs = append(searchDirs, filepath.Join(dir, dirFile.Name()))
			} else {
				for _, buildFileName := range BuildFileNames {
					if dirFile.Name() == buildFileName {
						buildFiles = append(buildFiles, filepath.Join(dir, dirFile.Name()))
					}
				}
			}
		}
	}

	return buildFiles
}

// appendCommands adds the given commands to be applied to each of the given targets
// via the commandMap.
func appendCommands(opts *Options, commandMap map[string][]commandsForTarget, args []string) {
	commands, targets := parseCommands(opts, args)
	for _, target := range targets {
		for _, buildFileName := range BuildFileNames {
			if strings.HasSuffix(target, filepath.FromSlash("/"+buildFileName)) {
				target = strings.TrimSuffix(target, filepath.FromSlash("/"+buildFileName)) + ":__pkg__"
			} else if strings.HasSuffix(target, "/"+buildFileName) {
				target = strings.TrimSuffix(target, "/"+buildFileName) + ":__pkg__"
			}
		}
		var buildFiles []string
		if label := labels.Parse(target); label.Package == stdinPackageName {
			buildFiles = []string{stdinPackageName}
		} else {
			buildFiles = targetExpressionToBuildFiles(opts.RootDir, target)
		}

		for _, file := range buildFiles {
			commandMap[file] = append(commandMap[file], commandsForTarget{target, commands})
		}
	}
}

func appendCommandsFromFiles(opts *Options, commandsByFile map[string][]commandsForTarget, labels []string) {
	for _, fileName := range opts.CommandsFiles {
		var reader io.Reader
		if fileName == stdinPackageName {
			reader = os.Stdin
		} else {
			rc := file.OpenReadFile(fileName)
			reader = rc
			defer rc.Close()
		}
		appendCommandsFromReader(opts, reader, commandsByFile, labels)
	}
}

func appendCommandsFromReader(opts *Options, reader io.Reader, commandsByFile map[string][]commandsForTarget, labels []string) {
	r := bufio.NewReader(reader)
	atEOF := false
	for !atEOF {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			atEOF = true
			err = nil
		}
		if err != nil {
			fmt.Fprintf(opts.ErrWriter, "Error while reading commands file: %v", err)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		args := strings.Split(line, "|")
		if len(args) > 1 && args[1] == "*" {
			cmd := append([]string{args[0]}, labels...)
			appendCommands(opts, commandsByFile, cmd)
		} else {
			appendCommands(opts, commandsByFile, args)
		}
	}
}

func printRecord(writer io.Writer, record *apipb.Output_Record) {
	fields := record.Fields
	line := make([]string, len(fields))
	for i, field := range fields {
		switch value := field.Value.(type) {
		case *apipb.Output_Record_Field_Text:
			if field.QuoteWhenPrinting && strings.ContainsRune(value.Text, ' ') {
				line[i] = fmt.Sprintf("%q", value.Text)
			} else {
				line[i] = value.Text
			}
			break
		case *apipb.Output_Record_Field_Number:
			line[i] = strconv.Itoa(int(value.Number))
			break
		case *apipb.Output_Record_Field_Error:
			switch value.Error {
			case apipb.Output_Record_Field_UNKNOWN:
				line[i] = "(unknown)"
				break
			case apipb.Output_Record_Field_MISSING:
				line[i] = "(missing)"
				break
			}
			break
		case *apipb.Output_Record_Field_List:
			line[i] = fmt.Sprintf("[%s]", strings.Join(value.List.Strings, " "))
			break
		}
	}

	fmt.Fprint(writer, strings.Join(line, " ")+"\n")
}

// Buildozer loops over all arguments on the command line fixing BUILD files.
func Buildozer(opts *Options, args []string) int {
	if opts.OutWriter == nil {
		opts.OutWriter = os.Stdout
	}
	if opts.ErrWriter == nil {
		opts.ErrWriter = os.Stderr
	}
	commandsByFile := make(map[string][]commandsForTarget)
	if len(opts.CommandsFiles) > 0 {
		appendCommandsFromFiles(opts, commandsByFile, args)
	} else {
		if len(args) == 0 {
			Usage()
		}
		appendCommands(opts, commandsByFile, args)
	}

	numFiles := len(commandsByFile)
	if opts.Parallelism > 0 {
		runtime.GOMAXPROCS(opts.Parallelism)
	}
	results := make(chan *rewriteResult, numFiles)
	data := make(chan commandsForFile)

	if opts.NumIO < 1 {
		fmt.Fprintf(opts.ErrWriter, "NumIO must be at least 1; got %d (are you using `NewOpts`?)\n", opts.NumIO)
		return 1
	}
	for i := 0; i < opts.NumIO; i++ {
		go func(results chan *rewriteResult, data chan commandsForFile) {
			for commandsForFile := range data {
				results <- rewrite(opts, commandsForFile)
			}
		}(results, data)
	}

	for file, commands := range commandsByFile {
		data <- commandsForFile{file, commands}
	}
	close(data)
	records := []*apipb.Output_Record{}
	var hasErrors bool
	var fileModified bool
	for i := 0; i < numFiles; i++ {
		fileResults := <-results
		if fileResults == nil {
			continue
		}
		hasErrors = hasErrors || len(fileResults.errs) > 0
		fileModified = fileModified || fileResults.modified
		for _, err := range fileResults.errs {
			fmt.Fprintf(opts.ErrWriter, "%s: %s\n", fileResults.file, err)
		}
		if fileResults.modified && !opts.Quiet {
			fmt.Fprintf(opts.ErrWriter, "fixed %s\n", fileResults.file)
		}
		if fileResults.records != nil {
			records = append(records, fileResults.records...)
		}
	}

	if opts.IsPrintingProto {
		data, err := proto.Marshal(&apipb.Output{Records: records})
		if err != nil {
			log.Fatal("marshaling error: ", err)
		}
		fmt.Fprintf(opts.OutWriter, "%s", data)
	} else {
		for _, record := range records {
			printRecord(opts.OutWriter, record)
		}
	}

	if hasErrors {
		return 2
	}
	if fileModified || opts.Stdout {
		return 0
	}
	// The file is not modified, check if there were any non-readonly commands
	nonReadonlyCommands := false
	for _, commandsByTarget := range commandsByFile {
		for _, commands := range commandsByTarget {
			for _, command := range commands.commands {
				if _, ok := readonlyCommands[command.tokens[0]]; !ok {
					nonReadonlyCommands = true
					break
				}
			}
		}
	}
	if nonReadonlyCommands {
		return 3
	}
	return 0
}

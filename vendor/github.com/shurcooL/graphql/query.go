package graphql

import (
	"bytes"
	"io"
	"reflect"
	"sort"

	"github.com/shurcooL/graphql/ident"
)

func constructQuery(qctx *queryContext, v interface{}, variables map[string]interface{}) string {
	query := qctx.Query(v)
	if variables != nil {
		return "query(" + queryArguments(variables) + ")" + query
	}
	return query
}

func constructMutation(qctx *queryContext, v interface{}, variables map[string]interface{}) string {
	query := qctx.Query(v)
	if variables != nil {
		return "mutation(" + queryArguments(variables) + ")" + query
	}
	return "mutation" + query
}

// queryArguments constructs a minified arguments string for variables.
//
// E.g., map[string]interface{}{"a": Int(123), "b": NewBoolean(true)} -> "$a:Int!$b:Boolean".
func queryArguments(variables map[string]interface{}) string {
	sorted := make([]string, 0, len(variables))
	for k := range variables {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var s string
	for _, k := range sorted {
		v := variables[k]
		s += "$" + k + ":"
		t := reflect.TypeOf(v)
		switch t.Kind() {
		case reflect.Slice, reflect.Array:
			// TODO: Support t.Elem() being a pointer, if needed. Probably want to do this recursively.
			s += "[" + t.Elem().Name() + "!]" // E.g., "[IssueState!]".
		case reflect.Ptr:
			// Pointer is an optional type, so no "!" at the end.
			s += t.Elem().Name() // E.g., "Int".
		default:
			name := t.Name()
			if name == "string" { // HACK: Workaround for https://github.com/shurcooL/githubql/issues/12.
				name = "ID"
			}
			// Value is a required type, so add "!" to the end.
			s += name + "!" // E.g., "Int!".
		}
	}
	return s
}

type queryContext struct {
	// Scalars are Go types that map to GraphQL scalars, and therefore we don't want to expand them.
	Scalars []reflect.Type
}

// Query uses writeQuery to recursively construct
// a minified query string from the provided struct v.
//
// E.g., struct{Foo Int, BarBaz *Boolean} -> "{foo,barBaz}".
func (c *queryContext) Query(v interface{}) string {
	var buf bytes.Buffer
	c.writeQuery(&buf, reflect.TypeOf(v), false)
	return buf.String()
}

// writeQuery writes a minified query for t to w. If inline is true,
// the struct fields of t are inlined into parent struct.
func (c *queryContext) writeQuery(w io.Writer, t reflect.Type, inline bool) {
	switch t.Kind() {
	case reflect.Ptr, reflect.Slice:
		c.writeQuery(w, t.Elem(), false)
	case reflect.Struct:
		// Special handling of scalar struct types. Don't expand them.
		for _, scalar := range c.Scalars {
			if t == scalar {
				return
			}
		}
		if !inline {
			io.WriteString(w, "{")
		}
		for i := 0; i < t.NumField(); i++ {
			if i != 0 {
				io.WriteString(w, ",")
			}
			f := t.Field(i)
			value, ok := f.Tag.Lookup("graphql")
			inlineField := f.Anonymous && !ok
			if !inlineField {
				if ok {
					io.WriteString(w, value)
				} else {
					io.WriteString(w, ident.ParseMixedCaps(f.Name).ToLowerCamelCase())
				}
			}
			c.writeQuery(w, f.Type, inlineField)
		}
		if !inline {
			io.WriteString(w, "}")
		}
	}
}

package errorx

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	SelfKey = "."
)

// Errors is a nested error container, to be used with structs, maps, or slices.
type Errors map[string]error

// Insert inserts an error at nested path, creating child Errors as needed.
func (es Errors) Insert(err error, path []interface{}) {
	spath := make([]string, len(path))
	for i := 0; i < len(path); i++ {
		spath[i] = fmt.Sprintf("%v", path[i])
	}
	es.InsertStr(err, spath)
}

// InsertVar inserts an error at nested path, creating child Errors as needed.
func (es Errors) InsertVar(err error, path ...interface{}) {
	es.Insert(err, path)
}

// InsertRoot inserts an error at the root level.
func (es Errors) InsertRoot(err error) {
	es.InsertStr(err, []string{SelfKey})
}

// InsertStr inserts an error at nested path, creating child Errors as needed.
func (es Errors) InsertStr(err error, path []string) {
	if len(path) == 0 {
		return
	}

	key := path[0]
	if len(path) == 1 {
		es.merge(err, key)
		return
	}

	// create nested err on demand
	sub, ok := es[key]
	if !ok {
		sub = Errors{}
		es[key] = sub
	}

	// check if sub err is nested
	subErrs, ok := sub.(Errors)
	if !ok {
		// rewrite plain errors as nested errors with self keys
		subErrs = Errors{SelfKey: sub}
		es[key] = subErrs
	}

	subErrs.InsertStr(err, path[1:])
}

func (es Errors) merge(err error, key string) {
	subErrs, ok := es[key].(Errors)
	if !ok {
		// path ends in empty or non-nested error, overwrite
		es[key] = err
		return
	}

	// path ends in an existing nested error, write to self key
	subErrs.merge(err, SelfKey)
}

// Error returns the error string of Errors.
func (es Errors) Error() string {
	if len(es) == 0 {
		return ""
	}

	var keys []string
	for key := range es {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	b := &strings.Builder{}
	for i, k := range keys {
		if i > 0 {
			b.WriteString("; ")
		}
		err := es[k]
		if errs, ok := err.(Errors); ok {
			fmt.Fprintf(b, "%s: (%v)", k, errs)
		} else {
			fmt.Fprintf(b, "%s: %v", k, err)
		}
	}
	return b.String()
}

// Normalize recursively filters all nils from Errors and returns back the updated Errors as an error.
// If the length of Errors becomes 0, it will return nil.
func (es Errors) Normalize() error {
	for k, err := range es {
		if err == nil {
			delete(es, k)
		}
	}
	if len(es) == 0 {
		return nil
	}
	return es
}

// MarshalJSON converts the Errors into a valid JSON.
func (es Errors) MarshalJSON() ([]byte, error) {
	errs := map[string]interface{}{}
	for key, err := range es {
		if ms, ok := err.(json.Marshaler); ok {
			errs[key] = ms
		} else {
			errs[key] = err.Error()
		}
	}
	return json.Marshal(errs)
}

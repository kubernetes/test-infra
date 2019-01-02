package v1beta1

import (
	"fmt"
	"regexp"
	"strings"

	valid "github.com/go-ozzo/ozzo-validation"
	"github.com/traiana/okro/okro/pkg/util/errorx"
)

const (
	maxNameLength = 31
	nameError     = "must consist of lower case alphanumeric characters and dashes (-), starting with an alphabet character"
)

var (
	nameRegex = regexp.MustCompile(`^[a-z]+[a-z0-9]*(-([a-z0-9]+))*$`)
	nameRule  = valid.By(func(value interface{}) error {
		return valid.Validate(value,
			valid.Match(nameRegex).Error(nameError),
			valid.Length(1, maxNameLength),
		)
	})

	// https://github.com/jonschlinkert/is-git-url/blob/master/index.js
	repoRegex  = regexp.MustCompile(`^(?:git|ssh|https?|git@[-\w.]+):(//)?(.*?)(\.git)(/?|#[-\d\w._]+?)$`)
	gitHubRepo = valid.By(func(value interface{}) error {
		return valid.Validate(value, valid.Match(repoRegex).Error("must be a valid GitHub repo URL"))
	})

	protocols    = []interface{}{PROTOCOL_HTTP, PROTOCOL_GRPC, PROTOCOL_TCP}
	protocolRule = valuesIn(protocols)
)

func valuesIn(values []interface{}) *valid.InRule {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = fmt.Sprint(v)
	}
	err := fmt.Sprintf("must be a valid value (%s)", strings.Join(s, ", "))
	return valid.In(values...).Error(err)
}

// helps check for duplicate keys in a nested structure
type deduper map[string][]interface{}

func (d deduper) add(key string, path ...interface{}) *dup {
	if prev, ok := d[key]; ok {
		return &dup{
			Key:   key,
			Path1: prev,
			Path2: path,
		}
	}
	d[key] = path
	return nil
}

func (d deduper) has(key string) bool {
	_, ok := d[key]
	return ok
}

type dup struct {
	Key   string
	Path1 []interface{}
	Path2 []interface{}
}

func (d *dup) asNested(subject string) error {
	cause := fmt.Errorf("duplicate %s found: %q", subject, d.Key)
	es := errorx.Errors{}
	es.Insert(cause, d.Path1)
	es.Insert(cause, d.Path2)
	return es
}

func aggr(v ...interface{}) string {
	fmts := strings.Repeat("%v.", len(v))
	str := fmt.Sprintf(fmts, v...)
	return str[:len(str)-1]
}

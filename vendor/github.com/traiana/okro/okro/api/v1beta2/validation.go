package v1beta2

import (
	"fmt"
	"regexp"
	"strings"

	. "github.com/go-ozzo/ozzo-validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/traiana/okro/okro/pkg/util/errorx"
)

const (
	nameMaxLength = 31
	nameFmt       = `^[a-z]([-a-z0-9]*[a-z0-9])?$`
	nameErrMsg    = "must consist of lower case alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character"

	// arbitrary length to keep by-name references from being abused
	nameRefMaxLength = 256
	nameRefErrMsg    = "by-name reference is too long"

	displayNameMaxLength = 31
	displayNameFmt       = `^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`
	displayNameErrMsg    = "must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character"

	// https://github.com/jonschlinkert/is-git-url/blob/master/index.js
	gitRepoFmt    = `^(?:git|ssh|https?|git@[-\w.]+):(//)?(.*?)(\.git)(/?|#[-\d\w._]+?)$`
	gitRepoErrMsg = "must be a valid git repository URL"
)

var (
	labelsRule = By(func(value interface{}) error {
		labels := value.(map[string]string)
		return validateLabels(labels)
	})

	nameRegexp = regexp.MustCompile(nameFmt)
	nameRule   = By(func(value interface{}) error {
		return Validate(value,
			Required,
			Match(nameRegexp).Error(nameErrMsg),
			Length(1, nameMaxLength),
		)
	})

	nameRefRule = Length(0, nameRefMaxLength).Error(nameRefErrMsg)

	displayNameRegexp = regexp.MustCompile(displayNameFmt)
	displayNameRule   = By(func(value interface{}) error {
		return Validate(value,
			Required,
			Match(displayNameRegexp).Error(displayNameErrMsg),
			Length(1, displayNameMaxLength),
		)
	})

	gitRepoRegexp = regexp.MustCompile(gitRepoFmt)
	gitURLRule    = By(func(value interface{}) error {
		return Validate(value,
			Match(gitRepoRegexp).Error(gitRepoErrMsg),
		)
	})

	protocols    = []interface{}{ProtocolHTTP, ProtocolGRPC, ProtocolTCP}
	protocolRule = valuesIn(protocols)
)

func valuesIn(values []interface{}) *InRule {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = fmt.Sprint(v)
	}
	err := fmt.Sprintf("must be a valid value (%s)", strings.Join(s, ", "))
	return In(values...).Error(err)
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

func validateLabels(labels map[string]string) error {
	errlist := validation.ValidateLabels(labels, field.NewPath("labels"))
	return errlist.ToAggregate()
}

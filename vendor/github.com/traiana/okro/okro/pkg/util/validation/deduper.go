package validation

import (
	"fmt"

	"github.com/traiana/okro/okro/pkg/util/errorx"
)

// helps check for duplicate keys in a nested structure
type Deduper map[string][]interface{}

func (d Deduper) Add(key string, path ...interface{}) *dup {
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

func (d Deduper) Has(key string) bool {
	_, ok := d[key]
	return ok
}

type dup struct {
	Key   string
	Path1 []interface{}
	Path2 []interface{}
}

func (d *dup) AsNested(subject string) error {
	cause := fmt.Errorf("duplicate %s found: %q", subject, d.Key)
	es := errorx.Errors{}
	es.Insert(cause, d.Path1)
	es.Insert(cause, d.Path2)
	return es
}

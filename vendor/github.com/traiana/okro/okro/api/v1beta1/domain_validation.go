package v1beta1

import (
	"fmt"
	"regexp"

	valid "github.com/go-ozzo/ozzo-validation"
	"github.com/traiana/okro/okro/pkg/util/errorx"
)

var (
	// local, main, or tenant/realm
	lookupRegex   = regexp.MustCompile(fmt.Sprintf(`(?i)(\w+\/\w+)|^%s$|^%s$`, LookupLocal, LookupMain))
	lookupMessage = fmt.Sprintf("lookup must be %q, %q, or an explicit \"tenant/realm\"", LookupLocal, LookupMain)
	lookupRule    = valid.Match(lookupRegex).Error(lookupMessage)
)

func (d Domain) Validate() error {
	return valid.ValidateStruct(&d,
		valid.Field(&d.Meta),
		valid.Field(&d.DisplayName, valid.Required),
	)
}

func (td TenantDomain) Validate() error {
	if err := valid.ValidateStruct(&td,
		valid.Field(&td.Meta),
		valid.Field(&td.Lookup, valid.Required, lookupRule), // required only in top level
		valid.Field(&td.Realms),
	); err != nil {
		return err
	}

	// unique realm name (per tenant)
	ddRealm := deduper{}
	for i, r := range td.Realms {
		if dup := ddRealm.add(r.Name, "realms", i); dup != nil {
			return dup.asNested("realm")
		}
	}

	// main realm exists
	if !ddRealm.has(LookupMain) {
		return errorx.New("must define main realm")
	}

	return nil
}

func (r Realm) Validate() error {
	if err := valid.ValidateStruct(&r,
		valid.Field(&r.Meta),
		valid.Field(&r.Lookup, lookupRule),
		valid.Field(&r.Apps),
		valid.Field(&r.Pubsubs),
	); err != nil {
		return err
	}

	// unique app name (per Realm)
	ddApp := deduper{}
	for i, a := range r.Apps {
		if dup := ddApp.add(a.Name, "apps", i); dup != nil {
			return dup.asNested("app")
		}
	}

	// unique pubsub name (per Realm)
	ddPubsub := deduper{}
	for i, p := range r.Pubsubs {
		if dup := ddPubsub.add(p.Name, "pubsubs", i); dup != nil {
			return dup.asNested("pubsub")
		}
	}

	return nil
}

func (a AppProjection) Validate() error {
	if err := valid.ValidateStruct(&a,
		valid.Field(&a.Meta),
		valid.Field(&a.Lookup, lookupRule),
		valid.Field(&a.Modules),
		// don't need to validate build/release, they're just pointers
	); err != nil {
		return err
	}

	if a.Build == "" && a.Release == "" {
		return errorx.New("must specify build or release")
	}

	// unique module name (per App)
	ddModule := deduper{}
	for i, m := range a.Modules {
		if dup := ddModule.add(m.Name, "modules", i); dup != nil {
			return dup.asNested("module")
		}
	}
	return nil
}

func (m ModuleProjection) Validate() error {
	return valid.ValidateStruct(&m,
		valid.Field(&m.Meta),
		valid.Field(&m.Lookup, lookupRule),
		valid.Field(&m.Binds),
	)
}

func (b Binds) Validate() error {
	if err := valid.ValidateStruct(&b,
		valid.Field(&b.Topics),
		valid.Field(&b.APIs),
	); err != nil {
		return err
	}

	// unique topic name per Module (Binds)
	ddTopic := deduper{}
	for i, t := range b.Topics {
		key := aggr(t.Tenant, t.Pubsub, t.Topic)
		if dup := ddTopic.add(key, "topics", i); dup != nil {
			return dup.asNested("topic")
		}
	}
	// unique topic name per Module (Binds)
	ddAPI := deduper{}
	for i, a := range b.APIs {
		key := aggr(a.Tenant, a.App, a.API, a.Version)
		if dup := ddAPI.add(key, "apis", i); dup != nil {
			return dup.asNested("api")
		}
	}

	return nil
}

func (t TopicBinding) Validate() error {
	return valid.ValidateStruct(&t,
		valid.Field(&t.Lookup, lookupRule),
		valid.Field(&t.Tenant, valid.Required, nameRule),
		valid.Field(&t.Pubsub, valid.Required, nameRule),
		valid.Field(&t.Topic, valid.Required, nameRule),
	)
}

func (a APIBinding) Validate() error {
	return valid.ValidateStruct(&a,
		valid.Field(&a.Lookup, lookupRule),
		valid.Field(&a.Tenant, valid.Required, nameRule),
		valid.Field(&a.App, valid.Required, nameRule),
		valid.Field(&a.API, valid.Required, nameRule),
		valid.Field(&a.Version, valid.Required, nameRule),
	)
}

func (p PubsubProjection) Validate() error {
	if err := valid.ValidateStruct(&p,
		valid.Field(&p.Meta),
		valid.Field(&p.Topics),
	); err != nil {
		return err
	}

	// unique topic name (per Pubsub)
	ddApp := deduper{}
	for i, t := range p.Topics {
		if dup := ddApp.add(t.Name, "topics", i); dup != nil {
			return dup.asNested("topic")
		}
	}
	return nil
}

func (t TopicProjection) Validate() error {
	return valid.ValidateStruct(&t,
		valid.Field(&t.Meta),
	)
}

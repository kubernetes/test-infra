package v1beta1

import (
	"fmt"
	"regexp"

	valid "github.com/go-ozzo/ozzo-validation"
	"github.com/traiana/okro/okro/pkg/util/errorx"
)

var (
	// tenant/app or tenant/*
	WhitelistRegex = regexp.MustCompile(`\w+/(\*|\w+)`)

	visibilityLevels    = []interface{}{VISIBILITY_PRIVATE, VISIBILITY_WHITELIST, VISIBILITY_PUBLIC}
	visibilityLevelRule = valuesIn(visibilityLevels)
)

func (c Catalog) Validate() error {
	if err := valid.ValidateStruct(&c,
		valid.Field(&c.Meta),
		valid.Field(&c.Apps),
		valid.Field(&c.Pubsubs),
	); err != nil {
		return err
	}

	// unique app name (per catalog)
	ddApp := deduper{}
	for i, a := range c.Apps {
		if dup := ddApp.add(a.Name, "apps", i); dup != nil {
			return dup.asNested("app")
		}
	}

	// unique pubsub name (per catalog)
	ddPubsub := deduper{}
	for i, p := range c.Pubsubs {
		if dup := ddPubsub.add(p.Name, "pubsubs", i); dup != nil {
			return dup.asNested("pubsub")
		}
	}

	return nil
}

func (a App) Validate() error {
	if err := valid.ValidateStruct(&a,
		valid.Field(&a.Meta),
		valid.Field(&a.RepoURL, valid.Required, gitHubRepo),
		valid.Field(&a.APIs),
	); err != nil {
		return err
	}

	// unique api name and path (per app)
	ddAPI := deduper{}
	ddPath := deduper{}
	for i, api := range a.APIs {
		if dup := ddAPI.add(api.Name, "apis", i); dup != nil {
			return dup.asNested("api")
		}
		switch api.Protocol {
		case PROTOCOL_HTTP, PROTOCOL_GRPC:
			for j, v := range api.Versions {
				if dup := ddPath.add(v.Path, "apis", i, "versions", j); dup != nil {
					return dup.asNested("cross-version api path")
				}
			}
		case PROTOCOL_TCP:
			// don't care
		}
	}

	return nil
}

func (a API) Validate() error {
	if err := valid.ValidateStruct(&a,
		valid.Field(&a.Meta),
		valid.Field(&a.Protocol, valid.Required, protocolRule),
		valid.Field(&a.PreferredVersion, valid.Required),
		valid.Field(&a.Versions, valid.Required),
	); err != nil {
		return err
	}

	// unique version name (per api)
	ddVersion := deduper{}
	for i, v := range a.Versions {
		if dup := ddVersion.add(v.Name, "versions", i); dup != nil {
			return dup.asNested("api version")
		}
	}

	// preferred version declared
	if !ddVersion.has(a.PreferredVersion) {
		return errorx.Errors{
			"preferred_version": fmt.Errorf("unknown api version %q", a.PreferredVersion),
		}
	}

	return nil
}

func (v APIVersion) Validate() error {
	return valid.ValidateStruct(&v,
		valid.Field(&v.Meta),
		valid.Field(&v.Exposure),
	)
}

func (ps Pubsub) Validate() error {
	if err := valid.ValidateStruct(&ps,
		valid.Field(&ps.Meta),
		valid.Field(&ps.Topics),
	); err != nil {
		return err
	}

	// unique topic name (per pubsub)
	ddTopic := deduper{}
	for i, t := range ps.Topics {
		if dup := ddTopic.add(t.Name, "topics", i); dup != nil {
			return dup.asNested("topic")
		}
	}

	return nil
}

func (t Topic) Validate() error {
	return valid.ValidateStruct(&t,
		valid.Field(&t.Meta),
		valid.Field(&t.Schema, valid.Required),
		valid.Field(&t.Partitions, valid.Required, valid.Min(1)),
		valid.Field(&t.Exposure),
	)
}

func (e Exposure) Validate() error {
	return valid.ValidateStruct(&e,
		valid.Field(&e.VisibilityLevel, valid.Required, visibilityLevelRule),
		valid.Field(&e.VisibilityWhitelist, whitelistValidator{e.VisibilityLevel}),
	)
}

type whitelistValidator struct {
	visibilityLevel string
}

func (v whitelistValidator) Validate(value interface{}) error {
	w, _ := value.([]string)
	if len(w) == 0 {
		return nil
	}
	if v.visibilityLevel != VISIBILITY_WHITELIST {
		return fmt.Errorf("must be blank for visibility level other than %q", VISIBILITY_WHITELIST)
	}
	const regexError = "must be in a valid format ('tenant/app' or 'tenant/*')"
	errs := errorx.Errors{}
	for i, s := range w {
		errs.InsertVar(valid.Validate(s, valid.Match(WhitelistRegex).Error(regexError)), i)
	}
	return errs.Normalize()
}

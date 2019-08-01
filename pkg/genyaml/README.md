# Genyaml

## Description
`genyaml` is a simple documentation tool used to marshal YAML from Golang structs. It extracts *doc comments* from `.go` sources files and adds them as *comment nodes* in the YAML output. 

## Usage

First, assume there is a Go file `config.go` with the following contents:
> NOTE: `genyaml` reads **json** tags for maximum portability.

```go
// config.go

package example

type Configuration struct {
	// Plugin comment
	Plugin []Plugin `json:"plugin,omitempty"`
}

type Plugin struct {
	// StringField comment
	StringField string `json:"string,omitempty"`
	// BooleanField comment
	BooleanField bool `json:"boolean,omitempty"`
	// IntegerField comment
	IntegerField int `json:"integer,omitempty"`
}
//...
```

Next, in a separate `example.go` file, initialize a `Configuration` struct and marshal it to *commented* YAML. 

```go
// example.go

package example

// Import genyaml
import "k8s.io/test-infra/pkg/genyaml"

//...

// Initialize a `Configuration` struct:
config := &example.Configuration{
    Plugin: []example.Plugin{
        {
            StringField:  "string",
            BooleanField: true,
            IntegerField: 1,
        },
    },
}

// Initialize a CommentMap instance from the `config.go` source file:
cm := genyaml.NewCommentMap("config.go")

// Generate a commented YAML snippet:
yamlSnippet, err := cm.GenYaml(config)
```

The doc comments are extracted from the `config.go` file and attached to the corresponding YAML fields:

```go
fmt.Println(yamlSnippet)
```

```yaml
# Plugin comment
plugin:
  - # BooleanField comment
    boolean: true

    # IntegerField comment
    integer: 1
    
    # StringField comment
    string: string
    
```

# Genyaml

## Description
`genyaml` is a simple documentation tool used to marshal YAML from Golang structs. It extracts *doc comments* from `.go` sources files and adds them as *comment nodes* in the YAML output.

## Usage

TODOs are ignored (e.g. TODO(clarketm)... or TODO...) if and only if they are on a **single** line.

```go
type Employee struct {
	// Name of employee
	Name string
	// Age of employee
	Age int
    // TODO(clarketm): change this to a float64
	// Salary of employee
	Salary int
}
```

```yaml
# Age of employee
Age: 22

# Name of employee
Name: Jim

# Salary of employee
Salary: 100000
```

Multiline comments are preserved, albeit *tabs* are converted to *spaces* and *multiple* spaces are compressed into a *single* line.

```go
type Multiline struct {
	// StringField1 comment
	// second line
	// third line
	StringField1 string `json:"string1"`

	/* StringField2 comment
	second line
	third line
	*/
	StringField2 string `json:"string2"`

	/* StringField3 comment
			second line
			third line
	*/
	StringField3 string `json:"string3"`
}
```

```yaml
# StringField1 comment
# second line
# third line
string1: string1

# StringField2 comment
# second line
# third line
string2: string2

# StringField3 comment
# second line
# third line
string3: string3
```

All subsequent lines and blocks after a `---` will be ignored.

```go
type Person struct {
	// Name of person
	// ---
	// The name of the person is both the first and last name separated
	// by a space character
	Name string
}
```

```yaml
# Name of person
Name: Frank
```

Generator instructions prefixed with a `+` are ignored.

```go
type Dog struct {
	// Gender of dog (male|female)
	// +optional
	Gender string `json:"gender,omitempty"`
	// Weight in pounds of dog
	Weight int `json:"weight,omitempty"`
}
```

```yaml
# Gender of dog (male|female)
gender: male

# Weight in pounds of dog
weight: 150
```

## Example

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
cm, err := genyaml.NewCommentMap("config.go")

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

## Limitations / Going Forward

- [ ] Embedded structs must include a tag name (i.e. must not be *spread*), otherwise the type can not be inferred from the YAML output.
- [ ] Interface types, more specifically concrete types implementing a particular interface, can can not be inferred from the YAML output.
- [ ] Upstream this functionality to `go-yaml` (or a fork) to leverage custom formatting of YAML and direct reflection on types for resolving embedded structs and interface types across multiple source files.

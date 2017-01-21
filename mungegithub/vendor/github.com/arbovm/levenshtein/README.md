Levenshtein Distance
====================

[Go](http://golang.org) package to calculate the [Levenshtein Distance](http://en.wikipedia.org/wiki/Levenshtein_distance)

Install
-------

    go get github.com/arbovm/levenshtein

Example
-------

```go
package main

import (
	"fmt"
	"github.com/arbovm/levenshtein"
)

func main() {
	s1 := "kitten"
	s2 := "sitting"
	fmt.Printf("The distance between %v and %v is %v\n",
		s1, s2, levenshtein.Distance(s1, s2))
	// -> The distance between kitten and sitting is 3
}

```

Documentation
-------------

Located [here](http://godoc.org/github.com/arbovm/levenshtein)


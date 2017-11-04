package inflection_test

import (
	"strings"
	"testing"

	"github.com/qor/inflection"
)

var inflections = map[string]string{
	"star":        "stars",
	"STAR":        "STARS",
	"Star":        "Stars",
	"bus":         "buses",
	"fish":        "fish",
	"mouse":       "mice",
	"query":       "queries",
	"ability":     "abilities",
	"agency":      "agencies",
	"movie":       "movies",
	"archive":     "archives",
	"index":       "indices",
	"wife":        "wives",
	"safe":        "saves",
	"half":        "halves",
	"move":        "moves",
	"salesperson": "salespeople",
	"person":      "people",
	"spokesman":   "spokesmen",
	"man":         "men",
	"woman":       "women",
	"basis":       "bases",
	"diagnosis":   "diagnoses",
	"diagnosis_a": "diagnosis_as",
	"datum":       "data",
	"medium":      "media",
	"stadium":     "stadia",
	"analysis":    "analyses",
	"node_child":  "node_children",
	"child":       "children",
	"experience":  "experiences",
	"day":         "days",
	"comment":     "comments",
	"foobar":      "foobars",
	"newsletter":  "newsletters",
	"old_news":    "old_news",
	"news":        "news",
	"series":      "series",
	"species":     "species",
	"quiz":        "quizzes",
	"perspective": "perspectives",
	"ox":          "oxen",
	"photo":       "photos",
	"buffalo":     "buffaloes",
	"tomato":      "tomatoes",
	"dwarf":       "dwarves",
	"elf":         "elves",
	"information": "information",
	"equipment":   "equipment",
	"criterion":   "criteria",
}

func init() {
	inflection.AddIrregular("criterion", "criteria")
}

func TestPlural(t *testing.T) {
	for key, value := range inflections {
		if v := inflection.Plural(strings.ToUpper(key)); v != strings.ToUpper(value) {
			t.Errorf("%v's plural should be %v, but got %v", strings.ToUpper(key), strings.ToUpper(value), v)
		}

		if v := inflection.Plural(strings.Title(key)); v != strings.Title(value) {
			t.Errorf("%v's plural should be %v, but got %v", strings.Title(key), strings.Title(value), v)
		}

		if v := inflection.Plural(key); v != value {
			t.Errorf("%v's plural should be %v, but got %v", key, value, v)
		}
	}
}

func TestSingular(t *testing.T) {
	for key, value := range inflections {
		if v := inflection.Singular(strings.ToUpper(value)); v != strings.ToUpper(key) {
			t.Errorf("%v's singular should be %v, but got %v", strings.ToUpper(value), strings.ToUpper(key), v)
		}

		if v := inflection.Singular(strings.Title(value)); v != strings.Title(key) {
			t.Errorf("%v's singular should be %v, but got %v", strings.Title(value), strings.Title(key), v)
		}

		if v := inflection.Singular(value); v != key {
			t.Errorf("%v's singular should be %v, but got %v", value, key, v)
		}
	}
}

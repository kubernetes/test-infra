Inflection
=========

Inflection pluralizes and singularizes English nouns

## Basic Usage

```go
inflection.Plural("person") => "people"
inflection.Plural("Person") => "People"
inflection.Plural("PERSON") => "PEOPLE"
inflection.Plural("bus")    => "buses"
inflection.Plural("BUS")    => "BUSES"
inflection.Plural("Bus")    => "Buses"

inflection.Singularize("people") => "person"
inflection.Singularize("People") => "Person"
inflection.Singularize("PEOPLE") => "PERSON"
inflection.Singularize("buses")  => "bus"
inflection.Singularize("BUSES")  => "BUS"
inflection.Singularize("Buses")  => "Bus"

inflection.Plural("FancyPerson") => "FancyPeople"
inflection.Singularize("FancyPeople") => "FancyPerson"
```

## Register Rules

Standard rules are from Rails's ActiveSupport (https://github.com/rails/rails/blob/master/activesupport/lib/active_support/inflections.rb)

If you want to register more rules, follow:

```
inflection.AddUncountable("fish")
inflection.AddIrregular("person", "people")
inflection.AddPlural("(bu)s$", "${1}ses") # "bus" => "buses" / "BUS" => "BUSES" / "Bus" => "Buses"
inflection.AddSingular("(bus)(es)?$", "${1}") # "buses" => "bus" / "Buses" => "Bus" / "BUSES" => "BUS"
```


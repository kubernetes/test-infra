package versioner

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalidNonSemanticVer is returned if a version is found to be invalid when
	// being parsed.
	ErrInvalidNonSemanticVer = fmt.Errorf("Invalid NonSemantic Version")

	// ErrInvalidDate is returned if the date being parsed is in a different
	// format than expected.
	ErrInvalidDate = fmt.Errorf("Invalid date format")

	// ErrEmptyString is returned when an empty string is passed in for parsing.
	ErrEmptyString = errors.New("Version string empty")

	// ErrInvalidCharacters is returned when invalid characters are found as
	// part of a version
	ErrInvalidCharacters = errors.New("Invalid characters in version")
)

const (
	DateFormat = "20060102"
)

// NonSemanticVer is for packages that follow nonsemantic version release
type NonSemanticVer struct {
	// in yyyyMMdd format
	date     string
	buildNum int
}

type VersionSorter []NonSemanticVer

func (a VersionSorter) Len() int {
	return len(a)
}

func (a VersionSorter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a VersionSorter) Less(i, j int) bool {
	if a[i].date == a[j].date {
		return a[i].buildNum < a[j].buildNum
	}
	ti, _ := time.Parse(DateFormat, a[i].date)
	tj, _ := time.Parse(DateFormat, a[j].date)

	return tj.After(ti)
}

// NewNonSemanticVer returns a new non semantic version object
func NewNonSemanticVer(v string) (*NonSemanticVer, error) {
	if len(v) == 0 {
		return nil, ErrEmptyString
	}

	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidNonSemanticVer
	}

	// we do not check if it is a valid date in yyyyMMdd format
	// because we never use it for any computation
	_, err := strconv.Atoi(parts[0])
	if err != nil || len(parts[0]) != 8 {
		return nil, ErrInvalidNonSemanticVer
	}

	bn, err := strconv.Atoi(parts[1])
	if err != nil || bn < 0 {
		return nil, ErrInvalidCharacters
	}

	return &NonSemanticVer{parts[0], bn}, nil
}

// IncrementVersion increases takes the current version and
// returns the next release version
func (v NonSemanticVer) IncrementVersion() NonSemanticVer {
	today := time.Now().Format(DateFormat)
	if strings.Compare(today, v.date) == 0 {
		return NonSemanticVer{date: v.date, buildNum: v.buildNum + 1}
	}
	return NonSemanticVer{date: today, buildNum: 0}
}

// String returns the  in string format
func (v NonSemanticVer) String() string {
	return fmt.Sprintf("%s.%02d", v.date, v.buildNum)
}

func (v NonSemanticVer) deepEquals(a NonSemanticVer) bool {
	return strings.Compare(v.date, a.date) == 0 && v.buildNum == a.buildNum
}

func (v NonSemanticVer) IsLesser(a NonSemanticVer) bool {
	if v.date == a.date {
		return v.buildNum < a.buildNum
	}
	tv, _ := time.Parse(DateFormat, v.date)
	ta, _ := time.Parse(DateFormat, a.date)
	return ta.After(tv)
}

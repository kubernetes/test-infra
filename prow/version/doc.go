// version holds variables that identify a Prow binary's name and version
package version

var (
	// Name is the colloquial identifier for the compiled component
	Name = "unset"
	// Version is a concatenation of the commit SHA and date for the build
	Version = "0"
)

func UserAgent() string {
	return Name + "/" + Version
}

package opts

type MungeOptions struct {
	MaxPRNumber    int
	MinPRNumber    int
	MinIssueNumber int
	Dryrun         bool
	Org            string
	Project        string
}

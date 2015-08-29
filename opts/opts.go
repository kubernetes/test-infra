package opts

type MungeOptions struct {
	MinPRNumber    int
	MinIssueNumber int
	Dryrun         bool
	Org            string
	Project        string
}

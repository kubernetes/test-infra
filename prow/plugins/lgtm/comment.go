package lgtm

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/test-infra/prow/github"
	"k8s.io/utils/strings/slices"
)

// lgtmTimelineNotificationName defines the name used in the title for the lgtm notifications.
const lgtmTimelineNotificationName = "LGTM Timeline notifier"
const lgtmTimelineNotificationHeader = "[" + lgtmTimelineNotificationName + "]\n---\nTimeline:\n"

var notificationRegex = regexp.MustCompile(`(?is)^\[` + lgtmTimelineNotificationName + `\] *?([^\n]*)(?:\n[-]{3}\n(.*))?`)

type comment struct {
	Body        string
	Author      string
	CreatedAt   time.Time
	HTMLURL     string
	ID          int
	ReviewState github.ReviewState
}

func filterComments(comments []*comment, filter func(*comment) bool) []*comment {
	filtered := make([]*comment, 0, len(comments))
	for _, c := range comments {
		if filter(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func getLast(cs []*comment) *comment {
	if len(cs) == 0 {
		return nil
	}
	return cs[len(cs)-1]
}

func commentsFromIssueComments(ics []github.IssueComment) []*comment {
	comments := make([]*comment, 0, len(ics))
	for i := range ics {
		comments = append(comments, commentFromIssueComment(&ics[i]))
	}
	return comments
}

func commentFromIssueComment(ic *github.IssueComment) *comment {
	if ic == nil {
		return nil
	}
	return &comment{
		Body:      ic.Body,
		Author:    ic.User.Login,
		CreatedAt: ic.CreatedAt,
		HTMLURL:   ic.HTMLURL,
		ID:        ic.ID,
	}
}

func notificationMatcher(isBot func(string) bool) func(*comment) bool {
	return func(c *comment) bool {
		if !isBot(c.Author) {
			return false
		}
		match := notificationRegex.FindStringSubmatch(c.Body)
		return len(match) > 0
	}
}

func parseValidLGTMFromTimelines(commentBody string) []string {
	var ret []string

	for _, line := range strings.Split(commentBody, "\n") {
		agreed, login := parseLgtmTimelineRecordLine(line)
		if login == "" {
			continue
		}

		if agreed {
			ret = append(ret, login)
		} else {
			// reset when it received a new deny voting.
			ret = []string{}
		}
	}

	return ret
}

func hasDumpLGTMs(gc githubClient, org, repo string, number int, login string) (bool, error) {
	comments, err := listTimelineComments(gc, org, repo, number)
	if err != nil {
		return false, err
	}

	comment := getLast(comments)
	if comment == nil {
		return false, err
	}
	// found the last agreed timeline in timeline comment, then compare it.
	duplicate := slices.Contains(parseValidLGTMFromTimelines(comment.Body), login)

	return duplicate, nil
}

func updateTimelineComment(gc githubClient, org, repo string, number int, login string, wantLGTM bool) error {
	notifications, err := listTimelineComments(gc, org, repo, number)
	if err != nil {
		return err
	}
	latestNotification := getLast(notifications)

	var messageLines []string
	if latestNotification == nil {
		messageLines = append(messageLines, lgtmTimelineNotificationHeader)
	} else {
		messageLines = append(messageLines, latestNotification.Body)
	}

	messageLines = append(messageLines, stringifyLgtmTimelineRecordLine(time.Now(), wantLGTM, login))

	for _, notif := range notifications {
		if err := gc.DeleteComment(org, repo, notif.ID); err != nil {
			return err
		}
	}

	return gc.CreateComment(org, repo, number, strings.Join(messageLines, "\n"))
}

func stringifyLgtmTimelineRecordLine(lgtmTime time.Time, wantLGTM bool, login string) string {
	tpl := "- `%s`: %s %s by [%s](https://github.com/%s)."

	actionStr := "reset"
	actionEmoji := ":heavy_multiplication_x::repeat:"
	if wantLGTM {
		actionStr = "agreed"
		actionEmoji = `:ballot_box_with_check:`
	}

	return fmt.Sprintf(tpl, lgtmTime, actionEmoji, actionStr, login, login)
}

func parseLgtmTimelineRecordLine(line string) (bool, string) {
	reg := regexp.MustCompile(`^\- .* (agreed|reset) by \[([-_a-zA-Z\d\.]+(\[bot\])?)\]\(https://github\.com/.*`)

	submatches := reg.FindStringSubmatch(line)
	if len(submatches) < 2 {
		return false, ""
	}

	action := submatches[1]
	login := submatches[2]
	switch action {
	case "agreed":
		return true, login
	case "reset":
		return false, login
	default:
		return false, ""
	}
}

func listTimelineComments(gc githubClient, org, repo string, number int) ([]*comment, error) {
	issueComments, err := gc.ListIssueComments(org, repo, number)
	if err != nil {
		return nil, err
	}

	botUserChecker, err := gc.BotUserChecker()
	if err != nil {
		return nil, err
	}

	commentsFromIssueComments := commentsFromIssueComments(issueComments)
	return filterComments(commentsFromIssueComments, notificationMatcher(botUserChecker)), nil
}

package github

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	signOffRegexp          = `(?im)^Signed-off-by: (?P<sign_name>.*) <(?P<sign_email>\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*)>$`
	signNameGroupName      = "sign_name"
	signEmailGroupName     = "sign_email"
	coAuthorRegexp         = `(?im)^Co-authored-by: (?P<co_author_name>.*) <(?P<co_author_email>\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*)>$`
	coAuthorNameGroupName  = "co_author_name"
	coAuthorEmailGroupName = "co_author_email"
)

type SignedAuthor struct {
	Name  string
	Email string
}

type CoAuthor struct {
	Name  string
	Email string
}

func NormalizeSignedOffBy(commitMessages []string) []SignedAuthor {
	combineMessage := strings.Join(commitMessages, "\n\n")
	authorSet := sets.String{}
	signedAuthors := make([]SignedAuthor, 0)

	compile := regexp.MustCompile(signOffRegexp)
	submatches := compile.FindAllStringSubmatch(combineMessage, -1)
	groupNames := compile.SubexpNames()

	for _, matches := range submatches {
		signName := ""
		signEmail := ""
		for i, match := range matches {
			groupName := groupNames[i]
			if groupName == signNameGroupName {
				signName = match
			} else if groupName == signEmailGroupName {
				signEmail = match
			}
		}

		key := getAuthorKey(signName, signEmail)
		if authorSet.Has(key) {
			continue
		}

		authorSet.Insert(key)
		signedAuthors = append(signedAuthors, SignedAuthor{
			Name:  signName,
			Email: signEmail,
		})
	}

	return signedAuthors
}

func NormalizeCoAuthorBy(commitAuthors []CommitAuthor, commitMessages []string, prAuthorLogin string) []CoAuthor {
	coAuthorSet := sets.String{}
	coAuthors := make([]CoAuthor, 0)

	// Get co-author information based on commit author information.
	for _, author := range commitAuthors {
		name := strings.TrimSpace(author.Name)
		email := strings.TrimSpace(author.Email)

		if len(email) == 0 {
			continue
		}

		key := getAuthorKey(name, email)
		if coAuthorSet.Has(key) {
			continue
		}

		if author.Login == nil || *author.Login != prAuthorLogin {
			coAuthorSet.Insert(key)
			coAuthors = append(coAuthors, CoAuthor{
				Name:  name,
				Email: email,
			})
		}
	}

	// Get co-author information based on the `Co-authored-by:` text in the commit message.
	combineMessage := strings.Join(commitMessages, "\n\n")
	compile := regexp.MustCompile(coAuthorRegexp)
	submatches := compile.FindAllStringSubmatch(combineMessage, -1)
	groupNames := compile.SubexpNames()

	for _, matches := range submatches {
		coAuthorName := ""
		coAuthorEmail := ""
		for i, match := range matches {
			groupName := groupNames[i]
			if groupName == coAuthorNameGroupName {
				coAuthorName = match
			} else if groupName == coAuthorEmailGroupName {
				coAuthorEmail = match
			}
		}

		key := getAuthorKey(coAuthorName, coAuthorEmail)
		if coAuthorSet.Has(key) {
			continue
		}

		coAuthorSet.Insert(key)
		coAuthors = append(coAuthors, CoAuthor{
			Name:  coAuthorName,
			Email: coAuthorEmail,
		})
	}

	return coAuthors
}

func getAuthorKey(name, email string) string {
	return strings.ToLower(fmt.Sprintf("%s<%s>", name, email))
}

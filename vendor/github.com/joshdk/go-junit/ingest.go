package junit

import (
	"strconv"
	"time"
)

// findSuites performs a depth-first search through the XML document, and
// attempts to ingest any "testsuite" tags that are encountered.
func findSuites(nodes []xmlNode, suites chan Suite) {
	for _, node := range nodes {
		switch node.XMLName.Local {
		case "testsuite":
			suites <- ingestSuite(node)
		default:
			findSuites(node.Nodes, suites)
		}
	}
}

func ingestSuite(root xmlNode) Suite {
	suite := Suite{
		Name:    root.Attr("name"),
		Package: root.Attr("package"),
	}

	for _, node := range root.Nodes {
		switch node.XMLName.Local {
		case "testcase":
			testcase := ingestTestcase(node)
			suite.Tests = append(suite.Tests, testcase)
		case "properties":
			props := ingestProperties(node)
			suite.Properties = props
		case "system-out":
			suite.SystemOut = string(node.Content)
		case "system-err":
			suite.SystemErr = string(node.Content)
		}
	}

	suite.Aggregate()
	return suite
}

func ingestProperties(root xmlNode) map[string]string {
	props := make(map[string]string, len(root.Nodes))

	for _, node := range root.Nodes {
		switch node.XMLName.Local {
		case "property":
			name := node.Attr("name")
			value := node.Attr("value")
			props[name] = value
		}
	}

	return props
}

func ingestTestcase(root xmlNode) Test {
	test := Test{
		Name:      root.Attr("name"),
		Classname: root.Attr("classname"),
		Duration:  duration(root.Attr("time")),
		Status:    StatusPassed,
	}

	for _, node := range root.Nodes {
		switch node.XMLName.Local {
		case "skipped":
			test.Status = StatusSkipped
		case "failure":
			test.Error = ingestError(node)
			test.Status = StatusFailed
		case "error":
			test.Error = ingestError(node)
			test.Status = StatusError
		}
	}

	return test
}

func ingestError(root xmlNode) Error {
	return Error{
		Body:    string(root.Content),
		Type:    root.Attr("type"),
		Message: root.Attr("message"),
	}
}

func duration(t string) time.Duration {
	// Check if there was a valid decimal value
	if s, err := strconv.ParseFloat(t, 64); err == nil {
		return time.Duration(s*1000000) * time.Microsecond
	}

	// Check if there was a valid duration string
	if d, err := time.ParseDuration(t); err == nil {
		return d
	}

	return 0
}

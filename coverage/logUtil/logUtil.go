package logUtil

import (
	"os"

	"github.com/sirupsen/logrus"
)

// LogFatalf customizes the behavior of handling Fatal error
var LogFatalf = func(format string, v ...interface{}) {
	logrus.Infof(format, v...)
	logrus.Info("Exit program with code 0 regardless of the above error")
	os.Exit(0)
}

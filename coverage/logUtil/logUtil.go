package logUtil

import (
	"log"
	"os"
)

// LogFatalf customizes the behavior of handling Fatal error
var LogFatalf = func(format string, v ...interface{}) {
	log.Printf(format, v...)
	log.Println("Exit program with code 0 regardless of the above error")
	os.Exit(0)
}

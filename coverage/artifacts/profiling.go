package artifacts

import (
	"io"
	covIo "k8s.io/test-infra/coverage/io"
	"k8s.io/test-infra/coverage/logUtil"
	"log"
	"os"
	"os/exec"
)

type ProfileReader struct {
	io.ReadCloser
}

func NewProfileReader(reader io.ReadCloser) *ProfileReader {
	return &ProfileReader{reader}
}

// runProfiling writes coverage profile (&its stdout) by running go test on
// target package
func runProfiling(covTargets []string, localArts *LocalArtifacts) {
	log.Println("\nStarts calc.runProfiling(...)")

	cmdArgs := []string{"test"}

	cmdArgs = append(cmdArgs, covTargets...)
	cmdArgs = append(cmdArgs, []string{"-covermode=count",
		"-coverprofile", localArts.ProfilePath()}...)

	log.Printf("go cmdArgs=%v\n", cmdArgs)
	cmd := exec.Command("go", cmdArgs...)

	goTestCoverStdout, errCmdOutput := cmd.Output()

	if errCmdOutput != nil {
		logUtil.LogFatalf("Error running 'go test -coverprofile ': error='%v'; stdout='%s'; stderr='%v'",
			errCmdOutput, goTestCoverStdout, cmd.Stderr)
	} else {
		log.Printf("coverage profile created @ '%s'", localArts.ProfilePath())
		covIo.CreateMarker(localArts.Directory(), CovProfileCompletionMarker)
	}

	stdoutPath := localArts.CovStdoutPath()
	stdoutFile, err := os.Create(stdoutPath)
	if err == nil {
		stdoutFile.Write(goTestCoverStdout)
	} else {
		log.Printf("Error creating stdout file: %v", err)
	}
	defer stdoutFile.Close()
	log.Printf("stdout of test coverage stored in %s\n", stdoutPath)
	log.Printf("Ends calc.runProfiling(...)\n\n")
	return
}

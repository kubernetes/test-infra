package test

const (
	ProfileName      = "cov-profile.txt"
	KeyProfileName   = "key-cov-profile.txt"
	StdoutName       = "stdout.txt"
	CovTargetRootRel = "testTarget"
	CovTargetRelPath = CovTargetRootRel + "/presubmit"
)

var (
	//ArtifactsDir      = AbsPath("test_output/artifacts")
	tmpArtsDir        = AbsPath("test_output/tmp_artifacts")
	InputArtifactsDir = AbsPath("testdata/artifacts")
	CovTargetDir      = AbsPath(CovTargetRelPath) + "/"
)

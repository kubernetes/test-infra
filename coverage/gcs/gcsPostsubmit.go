package gcs

import (
	"context"
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/logUtil"
	"log"
	"path"
	"sort"
	"strconv"
)

type PostSubmit struct {
	GcsBuild
	covProfileName   string
	ArtifactsDirName string
	BuildsSorted     *[]int
	Ctx              context.Context
}

func NewPostSubmit(ctx context.Context, client StorageClientIntf,
	bucket, prowJobName, artifactsDirName, covProfileName string) (p *PostSubmit) {

	log.Println("NewPostSubmit(Ctx, client StorageClientIntf, ...) started")
	gcsBuild := GcsBuild{
		StorageClient: client,
		Bucket:        bucket,
		Build:         -1,
		Job:           prowJobName,
	}
	p = &PostSubmit{
		GcsBuild:         gcsBuild,
		ArtifactsDirName: artifactsDirName,
		covProfileName:   covProfileName,
		Ctx:              ctx,
		BuildsSorted:     nil,
	}
	p.searchForLatestHealthyBuild()
	return
}

// listBuilds returns all builds in descending order and stores the result in
// .BuildsSorted
func (p *PostSubmit) listBuilds() (res []int) {
	lstBuildStrs := p.StorageClient.ListGcsObjects(p.Ctx, p.Bucket, p.dirOfJob()+"/", "/")
	for _, buildStr := range lstBuildStrs {
		num, err := strconv.Atoi(buildStr)
		if err != nil {
			log.Printf("None int build number found: '%s'", buildStr)
		} else {
			res = append(res, num)
		}
	}
	if len(res) == 0 {
		logUtil.LogFatalf("No build found for bucket '%s' and object '%s'\n",
			p.Bucket, p.dirOfJob())
	}
	sort.Sort(sort.Reverse(sort.IntSlice(res)))
	p.BuildsSorted = &res
	log.Printf("Sorted Builds: %v\n", res)
	return res
}

func (p *PostSubmit) dirOfJob() (result string) {
	return path.Join("logs", p.Job)
}

func (p *PostSubmit) dirOfBuild(build int) (result string) {
	return path.Join(p.dirOfJob(), strconv.Itoa(build))
}

func (p *PostSubmit) dirOfArtifacts(build int) (result string) {
	return path.Join(p.dirOfBuild(build), p.ArtifactsDirName)
}

func (p *PostSubmit) dirOfCompletionMarker(build int) (result string) {
	return path.Join(p.dirOfArtifacts(build), artifacts.CovProfileCompletionMarker)
}

func (p *PostSubmit) isBuildHealthy(build int) bool {
	return p.StorageClient.DoesObjectExist(p.Ctx, p.Bucket,
		p.dirOfCompletionMarker(build))
}

func (p *PostSubmit) pathToGoodCoverageArtifacts() (result string) {
	return p.dirOfArtifacts(p.Build)
}

func (p *PostSubmit) pathToGoodCoverageProfile() (result string) {
	return path.Join(p.pathToGoodCoverageArtifacts(), p.covProfileName)
}

func (p *PostSubmit) searchForLatestHealthyBuild() int {
	builds := p.listBuilds()
	for _, build := range builds {
		if p.isBuildHealthy(build) {
			p.Build = build
			return build
		}
	}
	logUtil.LogFatalf("No healthy build found, builds=%v\n", builds)
	return -1
}

// ProfileReader returns the reader for the most recent healthy profile
func (p *PostSubmit) ProfileReader() *artifacts.ProfileReader {
	profilePath := p.pathToGoodCoverageProfile()
	log.Printf("Reading base (master) coverage from <%s>...\n", profilePath)
	return p.StorageClient.ProfileReader(p.Ctx, p.Bucket, profilePath)
}

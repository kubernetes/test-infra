/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	resultstore "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"sigs.k8s.io/yaml"
)

// See https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2

func main() {
	var invID, tok string
	creds := os.Args[1]
	if len(os.Args) > 2 {
		tok = os.Args[2]
	}
	if len(os.Args) > 3 {
		invID = os.Args[3]
	}
	if err := setup(creds, tok, invID); err != nil {
		log.Fatal(err)
	}
}

func newUUID() string {
	return uuid.New().String()
}

func prop(key, value string) *resultstore.Property {
	return &resultstore.Property{
		Key:   key,
		Value: value,
	}
}

func timing(when time.Time) *resultstore.Timing {
	return &resultstore.Timing{
		StartTime: stamp(when),
	}
}

func timingEnd(when time.Time, seconds int64, nanos int32) *resultstore.Timing {
	t := timing(when)
	t.Duration = dur(seconds, nanos)
	return t
}

func stamp(when time.Time) *timestamp.Timestamp {
	return &timestamp.Timestamp{
		Seconds: when.Unix(),
	}
}

func dur(seconds int64, nanos int32) *duration.Duration {
	return &duration.Duration{
		Seconds: seconds,
		Nanos:   nanos,
	}
}

func grpcConn(accountPath string) (*grpc.ClientConn, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("system cert pool: %v", err)
	}
	creds := credentials.NewClientTLSFromCert(pool, "")
	const scope = "https://www.googleapis.com/auth/cloud-platform"
	perRPC, err := oauth.NewServiceAccountFromFile(accountPath, scope)
	if err != nil {
		return nil, fmt.Errorf("create oauth: %v", err)
	}
	conn, err := grpc.Dial(
		"resultstore.googleapis.com:443",
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(perRPC),
	)
	if err != nil {
		return nil, fmt.Errorf("dial: %v", err)
	}

	return conn, nil
}

type (
	uploadClient   = resultstore.ResultStoreUploadClient
	downloadClient = resultstore.ResultStoreDownloadClient
)

func clients(conn *grpc.ClientConn) (uploadClient, downloadClient) {
	up := resultstore.NewResultStoreUploadClient(conn)
	down := resultstore.NewResultStoreDownloadClient(conn)
	return up, down
}

func createInvocation(ctx context.Context, up uploadClient) (*resultstore.Invocation, string, error) {
	tok := newUUID()
	fmt.Println("Auth token", tok)
	inv, err := up.CreateInvocation(ctx, &resultstore.CreateInvocationRequest{
		Invocation: &resultstore.Invocation{
			InvocationAttributes: &resultstore.InvocationAttributes{
				ProjectId:   "fejta-prod",
				Description: "hello world",
			},
			Timing: timing(time.Now()),
		},
		AuthorizationToken: tok,
	})
	return inv, tok, err
}

func str(inv interface{}) string {
	buf, err := yaml.Marshal(inv)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func print(inv ...interface{}) {
	for _, i := range inv {
		fmt.Println(str(i))
	}
}

const (
	finishConfiguredTarget = false
	finishTarget           = false
	finishInvocation       = false
)

func pick(choices ...string) string {
	return choices[rand.Intn(len(choices))]
}

func caseName() string {
	return pick(
		"testFoo",
		"testBar",
		"testSanta",
		"testGeorge",
		"testAbstractFactoryBuilderBuilderBuilderFactory",
		"testCRUD [Feature:Demo]",
	)
}

func className() string {
	return pick(
		"com.google.omg",
		"com.google.whynot",
		"io.k8s.java",
		"k8s.io/golang",
		"ignore",
	)
}

func flip() bool {
	return rand.Int()%2 == 0
}

func randomTestCase() *resultstore.Test {
	c := &resultstore.TestCase{
		CaseName:  caseName(),
		ClassName: className(),
		Timing:    timingEnd(time.Now(), 15, 30),
		Properties: []*resultstore.Property{
			prop("whatever", "yo"),
		},
		Files: manyFiles,
	}

	failed, errored := flip(), flip()
	switch {
	case errored:
		if flip() {
			c.Result = resultstore.TestCase_INTERRUPTED
		} else {
			c.Result = resultstore.TestCase_CANCELLED
		}
	case failed || flip():
		c.Result = resultstore.TestCase_COMPLETED
	case flip():
		c.Result = resultstore.TestCase_SKIPPED
	case flip():
		c.Result = resultstore.TestCase_SUPPRESSED
	case flip():
		c.Result = resultstore.TestCase_FILTERED
	default:
		c.Result = resultstore.TestCase_COMPLETED
	}

	if failed {
		c.Failures = []*resultstore.TestFailure{
			{
				FailureMessage: "Expected err not to have occurred",
				ExceptionType:  "NotEverythingIsJavaException",
				StackTrace:     "TODO: stacktrace joke",
				Expected: []string{
					"foo",
					"bar",
				},
				Actual: []string{
					"spam",
					"eggs",
				},
			},
		}
	}

	if errored {
		c.Errors = []*resultstore.TestError{
			{
				ErrorMessage:  "true != false",
				ExceptionType: "panic",
				StackTrace:    "lines",
			},
		}
	}

	return &resultstore.Test{
		TestType: &resultstore.Test_TestCase{
			TestCase: c,
		},
	}
}

const (
	e2eLog        = "gs://kubernetes-jenkins/logs/ci-kubernetes-local-e2e/3355/build-log.txt"
	compressedLog = e2eLog
	pushLog       = "gs://kubernetes-jenkins/logs/post-test-infra-push-prow/1079/build-log.txt"
	bumpLog       = "gs://kubernetes-jenkins/logs/ci-test-infra-autobump-prow/67/build-log.txt"
	oldPushLog    = "gs://kubernetes-jenkins/logs/post-test-infra-push-prow/1077/build-log.txt"
	erickLog      = "gs://kubernetes-jenkins/erick.txt"
	fejtaLog      = "gs://kubernetes-jenkins/erick-fejta.txt"
)

var (
	testLog = &resultstore.File{
		Uid:           "test.log",
		Uri:           pushLog,
		ContentViewer: "https://storage.googleapis.com/kubernetes-jenkins/logs/ci-kubernetes-local-e2e/3355/build-log.txt",
	}
	buildLog = &resultstore.File{
		Uid: "build.log",
		Uri: bumpLog,
	}
	stdout = &resultstore.File{
		Uid: "stdout",
		Uri: erickLog,
	}
	stderr = &resultstore.File{
		Uid: "stderr",
		Uri: fejtaLog,
	}
	manyFiles = []*resultstore.File{ // special ids: build: stdout,stderr,baseline.lcov; test: test.xml,test.log,test.lcov
		buildLog, testLog, stdout, stderr,
	}
)

func setup(account, tok, invID string) error {
	// create connection and clients
	conn, err := grpcConn(account)
	if err != nil {
		return fmt.Errorf("setup: %v", err)
	}
	up, down := clients(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ensure invocation

	var inv *resultstore.Invocation
	if invID == "" {
		inv, tok, err = createInvocation(ctx, up)
		if err != nil {
			return fmt.Errorf("create invocation: %v", err)
		}
		invID = inv.Id.InvocationId
	}
	invName := "invocations/" + invID
	inv, err = down.GetInvocation(ctx, &resultstore.GetInvocationRequest{
		Name: invName,
	})
	if err != nil {
		return fmt.Errorf("get invocation %s: %v", invName, err)
	}
	print("gi", tok, inv)

	inv.Files = manyFiles
	inv, err = up.UpdateInvocation(ctx, &resultstore.UpdateInvocationRequest{
		Invocation: inv,
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{"files"},
		},
		AuthorizationToken: tok,
	})
	if err != nil {
		return fmt.Errorf("update invocation %s: %v", invName, err)
	}

	// add a target

	t, err := up.CreateTarget(ctx, &resultstore.CreateTargetRequest{
		Parent:   inv.Name,
		TargetId: "//erick//fejta:stardate-" + strconv.FormatInt(time.Now().Unix(), 10),
		Target: &resultstore.Target{ // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#Target
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_BUILDING,
				Description: "fun fun",
			},
			Visible: true, // Will not appear in UI otherwise
			//   Timing
			//   TargetAttributes
			//   TestAttributes
			//   Properties
			//   Files
		},
		AuthorizationToken: tok,
	})
	if err != nil {
		return fmt.Errorf("create target: %v", err)
	}
	print("ct", t)

	tr, err := down.ListTargets(ctx, &resultstore.ListTargetsRequest{
		Parent: inv.Name,
		// PageSize
		// PageStart
	})
	if err != nil {
		return fmt.Errorf("list targets: %v", err)
	}
	print("lt", inv, tr)

	// create a configuration
	c, err := up.CreateConfiguration(ctx, &resultstore.CreateConfigurationRequest{
		Parent:             inv.Name,
		ConfigId:           "default",
		AuthorizationToken: tok,
		Configuration: &resultstore.Configuration{
			// Overall status of this config
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_BUILDING, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#Status
				Description: "very exciting",
			},
			ConfigurationAttributes: &resultstore.ConfigurationAttributes{
				Cpu: "amd64", // this is the only value, LOL
			},
			Properties: []*resultstore.Property{
				prop("something", fmt.Sprintf("exciting-%d", time.Now().Unix())),
				prop("more", "excitement"),
			},
		},
	})
	if err != nil {
		print("create failed (already exists?", err)
	}
	print("cc", c)

	lr, err := down.ListConfigurations(ctx, &resultstore.ListConfigurationsRequest{
		Parent: inv.Name,
		// PageSize, PageStart
	})
	if err != nil {
		return fmt.Errorf("list configurations: %v", err)
	}
	print("lc", lr)

	cfgs := lr.Configurations

	// create a configured target

	ct, err := up.CreateConfiguredTarget(ctx, &resultstore.CreateConfiguredTargetRequest{
		Parent:             t.Name,
		ConfigId:           cfgs[len(cfgs)-1].Id.ConfigurationId,
		AuthorizationToken: tok,
		ConfiguredTarget: &resultstore.ConfiguredTarget{
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_TESTING,
				Description: "oh wow",
			},
			Timing: timingEnd(time.Now(), 50, 7),
			TestAttributes: &resultstore.ConfiguredTestAttributes{
				TotalRunCount:   1,
				TotalShardCount: 1,
				TimeoutDuration: dur(500, 0),
			},
			Properties: []*resultstore.Property{
				prop("fun", fmt.Sprintf("times-%d", time.Now().Unix())),
			},
			Files: []*resultstore.File{
				{
					Uid: newUUID(),
					Uri: "gs://erick/fejta/likes/uuids",
					Length: &wrappers.Int64Value{
						Value: 19,
					},
					ContentType: "orange",
					ArchiveEntry: &resultstore.ArchiveEntry{
						Path: "/freedom",
						Length: &wrappers.Int64Value{
							Value: 10000,
						},
						ContentType: "text/plain",
					},
					ContentViewer: "https://prow.k8s.io/tide",
					Hidden:        false,
					Description:   "many thanks",
					Digest:        "yes", // what is hexadecimal-"like"
					HashType:      resultstore.File_SHA256,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create configured target: %v", err)
	}
	print("cct", ct)

	lctr, err := down.ListConfiguredTargets(ctx, &resultstore.ListConfiguredTargetsRequest{
		Parent: t.Name,
		// pagesize/start
	})
	if err != nil {
		return fmt.Errorf("list %s configured targets: %v", t.Name, err)
	}
	print("lct", lctr)

	cts := lctr.ConfiguredTargets
	ct = cts[len(cts)-1]

	a, err := up.CreateAction(ctx, &resultstore.CreateActionRequest{
		Parent:             ct.Name,
		ActionId:           "build",
		AuthorizationToken: tok,
		Action: &resultstore.Action{
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_BUILT,
				Description: "so built",
			},
			// Timing
			ActionType: &resultstore.Action_BuildAction{
				BuildAction: &resultstore.BuildAction{
					Type:              "javac",
					PrimaryInputPath:  "java/com/google/whatever/foo.java",
					PrimaryOutputPath: "whatever.o",
				},
			},
			// ActionType: Action_BuildAction / Test
			ActionAttributes: &resultstore.ActionAttributes{
				ExecutionStrategy: resultstore.ExecutionStrategy_LOCAL_SEQUENTIAL, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#ExecutionStrategy
				ExitCode:          1,
				Hostname:          "foo",
				InputFileInfo: &resultstore.InputFileInfo{
					Count:             3,
					DistinctCount:     2,
					CountLimit:        12,
					DistinctBytes:     100,
					DistinctByteLimit: 1000,
				},
			},
			ActionDependencies: []*resultstore.Dependency{
				{
					Resource: &resultstore.Dependency_Target{
						Target: t.Name,
					},
					Label: "Root Cause", // exact resource that caused falure
				},
			},
			Properties: nil,
			Files: []*resultstore.File{ // special ids: build: stdout,stderr,baseline.lcov; test: test.xml,test.log,test.lcov
				stdout,
				stderr,
			},
			Coverage: nil, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#ActionCoverage
		},
	})
	if err != nil {
		return fmt.Errorf("create build action: %v", err)
	}
	print("ca#build", a)

	a, err = up.CreateAction(ctx, &resultstore.CreateActionRequest{
		Parent:             ct.Name,
		ActionId:           "test",
		AuthorizationToken: tok,
		Action: &resultstore.Action{
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_PASSED,
				Description: "hello again",
			},
			// Timing
			ActionType: &resultstore.Action_TestAction{
				TestAction: &resultstore.TestAction{
					TestTiming: &resultstore.TestTiming{
						Location: &resultstore.TestTiming_Remote{
							// Local: &resultstore.LocalTestTiming{
							//   TestProcessDuration: &duration.Duration{},
							// }
							Remote: &resultstore.RemoteTestTiming{
								LocalAnalysisDuration: dur(2, 0),
								Attempts: []*resultstore.RemoteTestAttemptTiming{
									{
										// https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#RemoteTestAttemptTiming
										QueueDuration:        dur(3, 4),
										UploadDuration:       dur(30, 40),
										MachineSetupDuration: dur(5, 0),
										TestProcessDuration:  dur(6, 0),
										DownloadDuration:     dur(7, 0),
									},
								},
							},
						},
						SystemTimeDuration: dur(5, 0),
						UserTimeDuration:   dur(10, 0),
						TestCaching:        resultstore.TestCaching_CACHE_MISS, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#TestCaching
					},
					ShardNumber:          0,
					RunNumber:            1,
					AttemptNumber:        2,
					EstimatedMemoryBytes: 3,
					Warnings: []*resultstore.TestWarning{
						{
							WarningMessage: "google sure loves themselves some proto fields",
						},
					},
					TestSuite: &resultstore.TestSuite{
						SuiteName: "sweeeeet",
						Tests: []*resultstore.Test{
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							randomTestCase(),
							// TestType: &resultstore.Test_TestSuite{TestSuite: &resultstore.TestSuite{}},
						},
						Failures: []*resultstore.TestFailure{ // suite level failures
							{
								FailureMessage: "bitter",
								ExceptionType:  "VegetableException",
								Expected: []string{
									"candy",
									"fruit",
									"meat",
								},
								Actual: []string{
									"broccoli",
									"kale",
									"cucumber",
								},
							},
						},
						Errors: []*resultstore.TestError{ // suite level errors
							{
								ErrorMessage:  "salty",
								ExceptionType: "wounded",
							},
						},
						Timing: timingEnd(time.Now(), 173, 0), // time to complete suite
						Properties: []*resultstore.Property{
							prop("sweet", "success"),
						},
						Files: manyFiles, // files produced by this test suite
					},
				},
			},
			ActionAttributes: &resultstore.ActionAttributes{
				ExecutionStrategy: resultstore.ExecutionStrategy_LOCAL_SEQUENTIAL, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#ExecutionStrategy
				ExitCode:          1,
				Hostname:          "foo",
				InputFileInfo: &resultstore.InputFileInfo{
					Count:             3,
					DistinctCount:     2,
					CountLimit:        12,
					DistinctBytes:     100,
					DistinctByteLimit: 1000,
				},
			},
			ActionDependencies: []*resultstore.Dependency{
				{
					Resource: &resultstore.Dependency_Target{
						Target: t.Name,
					},
					Label: "Root Cause", // exact resource that caused falure
				},
			},
			Properties: nil,
			Files: []*resultstore.File{ // special ids: build: stdout,stderr,baseline.lcov; test: test.xml,test.log,test.lcov
				testLog,
			},
			Coverage: nil, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#ActionCoverage
		},
	})
	if err != nil {
		return fmt.Errorf("create test action: %v", err)
	}
	print("ca#test", a)

	lar, err := down.ListActions(ctx, &resultstore.ListActionsRequest{
		Parent: ct.Name,
		// pagesizestart
	})
	print("la", lar, err)

	if finishConfiguredTarget {
		fct, err := up.FinishConfiguredTarget(ctx, &resultstore.FinishConfiguredTargetRequest{
			Name:               ct.Name,
			AuthorizationToken: tok,
		})
		if err != nil {
			return fmt.Errorf("finish %s: %v", ct.Name, err)
		}
		print("fct", fct)
	}

	if finishTarget {
		ft, err := up.FinishTarget(ctx, &resultstore.FinishTargetRequest{
			Name:               t.Name,
			AuthorizationToken: tok,
		})
		if err != nil {
			return fmt.Errorf("finish %s: %v", t.Name, err)
		}
		print("ft", ft)
	}

	if finishInvocation {
		fi, err := up.FinishInvocation(ctx, &resultstore.FinishInvocationRequest{
			Name:               inv.Name,
			AuthorizationToken: tok,
		})
		if err != nil {
			return fmt.Errorf("finish %s: %v", inv.Name, err)
		}
		print("fi", fi)

	}

	fmt.Println("Token: " + tok)
	fmt.Printf("See https://source.cloud.google.com/results/invocations/%s\n", inv.Id.InvocationId)
	return nil
}

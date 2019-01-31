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
	"os"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	resultstore "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"sigs.k8s.io/yaml"
)

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

func stamp(when time.Time) *timestamp.Timestamp {
	return &timestamp.Timestamp{
		Seconds: when.Unix(),
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
			Timing: &resultstore.Timing{
				StartTime: stamp(time.Now()),
			},
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
	s := "invocations/" + invID
	inv, err = down.GetInvocation(ctx, &resultstore.GetInvocationRequest{
		Name: s,
	})
	if err != nil {
		return fmt.Errorf("get invocation %s: %v", err, s)
	}
	print("gi", tok, inv)

	// add a target

	t, err := up.CreateTarget(ctx, &resultstore.CreateTargetRequest{
		Parent:   inv.Name,
		TargetId: "//erick//fejta:stardate-" + strconv.FormatInt(time.Now().Unix(), 10),
		Target: &resultstore.Target{
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_BUILDING,
				Description: "fun fun",
			},
			Visible: true,
		},
		// Target - https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#Target
		//   StatusAttributes
		//   Timing
		//   TargetAttributes
		//   TestAttributes
		//   Properties
		//   Files
		//   Visible
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
				{
					Key:   "something",
					Value: fmt.Sprintf("exciting-%d", time.Now().Unix()),
				},
				{
					Key:   "more",
					Value: "excitement",
				},
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
	print("lc", lr, err)

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
			Timing: &resultstore.Timing{
				StartTime: stamp(time.Now()),
				Duration: &duration.Duration{
					Seconds: 50,
					Nanos:   7,
				},
			},
			TestAttributes: &resultstore.ConfiguredTestAttributes{
				TotalRunCount:   1,
				TotalShardCount: 1,
				TimeoutDuration: &duration.Duration{
					Seconds: 500,
				},
			},
			Properties: []*resultstore.Property{
				{
					Key:   "fun",
					Value: fmt.Sprintf("times-%d", time.Now().Unix()),
				},
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

	print("cct", ct, err)

	lctr, err := down.ListConfiguredTargets(ctx, &resultstore.ListConfiguredTargetsRequest{
		Parent: t.Name,
		// pagesize/start
	})

	print("lct", lctr, err)

	cts := lctr.ConfiguredTargets
	ct = cts[len(cts)-1]

	a, err := up.CreateAction(ctx, &resultstore.CreateActionRequest{
		Parent:             ct.Name,
		ActionId:           "danger",
		AuthorizationToken: tok,
		Action: &resultstore.Action{
			StatusAttributes: &resultstore.StatusAttributes{
				Status:      resultstore.Status_PASSED,
				Description: "hello again",
			},
			// Timing
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
			Files:      nil, // special ids: build: stdout,stderr,baseline.lcov; test: test.xml,test.log,test.lcov
			Coverage:   nil, // https://godoc.org/google.golang.org/genproto/googleapis/devtools/resultstore/v2#ActionCoverage
		},
	})
	print("ca", a, err)

	lar, err := down.ListActions(ctx, &resultstore.ListActionsRequest{
		Parent: ct.Name,
		// pagesizestart
	})
	print("la", lar, err)

	fct, err := up.FinishConfiguredTarget(ctx, &resultstore.FinishConfiguredTargetRequest{
		Name:               ct.Name,
		AuthorizationToken: tok,
	})
	print("fct", fct, err)

	ft, err := up.FinishTarget(ctx, &resultstore.FinishTargetRequest{
		Name:               t.Name,
		AuthorizationToken: tok,
	})
	print("ft", ft, err)

	fi, err := up.FinishInvocation(ctx, &resultstore.FinishInvocationRequest{
		Name:               inv.Name,
		AuthorizationToken: tok,
	})
	print("fi", fi, err)

	fmt.Printf("See https://source.cloud.google.com/results/invocations/%s\n", inv.Id.InvocationId)

	return nil
}

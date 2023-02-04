/*
Copyright 2023 The Kubernetes Authors.

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
	"flag"
	"log"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	pb "k8s.io/test-infra/prow/gangway"
	gangwayGoogleClient "k8s.io/test-infra/prow/gangway/client/google"
)

var (
	addr      = flag.String("addr", "127.0.0.1:50051", "Address of grpc server.")
	apiKey    = flag.String("api-key", "", "API key.")
	audience  = flag.String("audience", "", "Audience.")
	keyFile   = flag.String("key-file", "", "Path to a Google service account key file.")
	clientPem = flag.String("client-pem", "", "Path to a client.pem file.")
)

// Use like this:
//
// go run main.go --key-file=key.json \
// 		--audience=SERVICE_NAME.endpoints.PROJECT_NAME.cloud.goog \
// 		--addr=12.34.56.78:443 --api-key=API_KEY --client-pem=client.pem \
// 		<CreateJobExecutionRequest>
//
// where <CreateJobExecutionRequest> is the protobuf (in textpb format) message
// you want to send over. For example, if you want to run the periodic job named
// "foo", use:
//
// 		'job_name: "foo", job_execution_type: 1'
//
// as the <CreateJobExecutionRequest>. The "1" here for "job_execution_type"
// denotes the periodic job type (because this field is an enum, not a string).

func main() {
	flag.Parse()

	jobName := "some-job"
	jobExecutionType := pb.JobExecutionType_PERIODIC

	// Set default values.
	cjer := pb.CreateJobExecutionRequest{
		JobName:          jobName,
		JobExecutionType: jobExecutionType,
	}

	// Read in string version of a CreateJobExecutionRequest.
	if len(flag.Args()) > 0 {
		textpb := flag.Arg(0)
		if err := prototext.Unmarshal([]byte(textpb), &cjer); err != nil {
			log.Fatalf("could not unmarshal textpb %q: %v", textpb, err)
		}
	}

	log.Printf("creating job execution with %+v", &cjer)

	// Create a Prow API gRPC client that's able to authenticate to Gangway (the
	// Prow API Server).
	prowClient, err := gangwayGoogleClient.NewFromFile(*addr, *keyFile, *audience, *clientPem, *apiKey)
	if err != nil {
		log.Fatalf("Prow API client creation failed: %v", err)
	}

	defer prowClient.Close()

	// Create a Context that has credentials injected inside it.
	ctx, err := prowClient.EmbedCredentials(context.Background())
	if err != nil {
		log.Fatalf("could not create a context with embedded credentials: %v", err)
	}

	// Trigger job! Because this is gRPC it's just a function call.
	jobExecution, err := prowClient.GRPC.CreateJobExecution(ctx, &cjer)
	if err != nil {
		log.Fatalf("could not trigger job: %v", err)
	}

	log.Printf("triggered job: %v", jobExecution)

	// Poll to see if our job has succeeded.
	pollInterval := 2 * time.Second
	timeout := 90 * time.Second
	expectedStatus := pb.JobExecutionStatus_SUCCESS

	if err := prowClient.WaitForJobExecutionStatus(ctx, jobExecution.Id, pollInterval, timeout, expectedStatus); err != nil {
		log.Fatalf("failed: %v", err)
	}

	log.Printf("job succeeded!")
}

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

// The example command runs a sample client that interacts with the summarizer
// server to fetch a mocked summary object for demo purpose. Clients that
// depend on the summarizer service can use this as a starting point.
package main

import (
	"context"
	"errors"
	"flag"
	"time"

	"github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	responsepb "k8s.io/test-infra/testgrid/cmd/summarizer/response"
	grpcpb "k8s.io/test-infra/testgrid/cmd/summarizer/summary"
)

// options configures the server
type options struct {
	tls                bool
	caFile             string
	serverAddr         string
	serverHostOverride string
}

// validate ensures sane options
func (o *options) validate() error {
	if o.tls == true && o.caFile == "" {
		return errors.New("--ca_file must contain a valid CA root cert file when the --tls flag is set")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	flag.BoolVar(&o.tls, "tls", false, "Connection uses TLS if true, else plain TCP")
	flag.StringVar(&o.caFile, "ca_file", "", "The file containing the CA root cert file")
	flag.StringVar(&o.serverAddr, "server_addr", "127.0.0.1:10000", "The server address in the format of host:port")
	flag.StringVar(&o.serverHostOverride, "server_host_override", "x.test.youtube.com", "The server name use to verify the hostname returned by TLS handshake")
	flag.Parse()
	return o
}

func collectTestResult() (*responsepb.Response, error) {
	// Read fake response data. When building an application client, replace
	// with real test result aggregated into the Response struct
	response, err := fakeResponse()
	return response, err
}

func getSummary(c grpcpb.SummarizerServiceClient) {
	logrus.Println("Get the translated summary object")
	query, err := collectTestResult()
	if err != nil {
		logrus.Fatalf("failed to fetch test results: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	request := &grpcpb.SummarizerRequest{
		Response: query,
	}
	response, err := c.GetSummary(ctx, request)
	if err != nil {
		logrus.Fatalf("%v.GetSummary() failed, %v", c, err)
	}
	logrus.Println(response)
}

func main() {
	logrus.Println("Starting the Client")

	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("bad flags: %v", err)
	}

	var grpcOptions []grpc.DialOption
	if opt.tls {
		creds, err := credentials.NewClientTLSFromFile(opt.caFile, opt.serverHostOverride)
		if err != nil {
			logrus.Fatalf("failed to create TLS credentials %v", err)
		}
		grpcOptions = append(grpcOptions, grpc.WithTransportCredentials(creds))
	} else {
		grpcOptions = append(grpcOptions, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(opt.serverAddr, grpcOptions...)
	if err != nil {
		logrus.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := grpcpb.NewSummarizerServiceClient(conn)
	getSummary(client)
}

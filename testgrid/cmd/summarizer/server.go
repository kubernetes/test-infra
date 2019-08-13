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

// The summarizer command is the gRPC server entry point that serves the
// test summary. The summary is converted from the input table data aggregated
// from test results, which is then translated into the structured
// DashboardTabSummary proto for Testgrid consumption.
package main

import (
	"context"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"

	"google.golang.org/grpc"

	grpcpb "k8s.io/test-infra/testgrid/cmd/summarizer/summary"
)

var (
	port = 10000
)

type summarizerServer struct{}

func (s *summarizerServer) GetSummary(ctx context.Context, request *grpcpb.SummarizerRequest) (*grpcpb.SummarizerResponse, error) {
	summary, err := TableToSummary(request.Response)
	response := &grpcpb.SummarizerResponse{
		DashboardTabSummary: &summary,
	}
	return response, err
}

func newServer() *summarizerServer {
	s := &summarizerServer{}
	return s
}

func main() {
	logrus.Println("Starting the Server")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logrus.Fatalf("failed to listen: %v", err)
	}
	logrus.Println("Server is now listening on port:", port)
	grpcServer := grpc.NewServer()
	grpcpb.RegisterSummarizerServiceServer(grpcServer, newServer())
	grpcServer.Serve(lis)
}

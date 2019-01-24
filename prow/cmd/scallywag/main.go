package main

import (
	"net"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"k8s.io/test-infra/prow/scallywag"
)

func main() {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		logrus.Fatalf("tcp listener failed: %s", err)
	}

	grpcServer := grpc.NewServer()
	scallywag.RegisterGitServiceServer(grpcServer, &scallywag.GitService{})

	logrus.Info("starting up scallywag git service...")

	err = grpcServer.Serve(listener)
	if err != nil {
		logrus.Fatalf("gRPC server failed: %s", err)
	}

}

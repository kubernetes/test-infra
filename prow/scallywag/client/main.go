package main

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/scallywag"

	"google.golang.org/grpc"
)

func main() {
	connection, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		logrus.Fatalf("connection to gRPC server failed: %s", err)
	}

	defer connection.Close()

	c := scallywag.NewGitServiceClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	issue := &scallywag.Issue{Body: "This issue has NOT been updated"}
	updatedIssue, err := c.UpdateIssue(ctx, issue)
	if err != nil {
		logrus.Fatalf("call to UpdateIssue failed: %s", err)
	}

	logrus.Printf("Body of issue: %s\n", updatedIssue.Body)
}

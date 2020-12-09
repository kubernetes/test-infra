package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"
)

const (
	projectID = "kubernetes-jenkins"
	subID     = "kettle"
)

func pullMsgs(w io.Writer, projectID, subID string) error {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("pubsub.NewClient: %v", err)
	}
	sub := client.Subscription(subID)
	// Must set ReceiveSettings.Synchronous to false (or leave as default) to enable
	// concurrency settings. Otherwise, NumGoroutines will be set to 1.
	sub.ReceiveSettings.Synchronous = false
	// NumGoroutines is the number of goroutines sub.Receive will spawn to pull
	// messages concurrently.
	sub.ReceiveSettings.NumGoroutines = runtime.NumCPU()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var counter int32

	fmt.Print(sub)
	// Receive blocks until the context is cancelled or an error occurs.
	err = sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
		w.Write(msg.Data)
		atomic.AddInt32(&counter, 1)
		msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("pubsub: Receive returned error: %v", err)
	}
	fmt.Fprintf(w, "Received %d messages\n", counter)

	return nil
}

func main() {
	file, err := os.Create("./foo.txt")
	if err != nil {
		return
	}
	err = pullMsgs(file, projectID, subID)
	if err != nil {
		fmt.Printf("pubsub: Receive returned error: %v", err)
	}
}

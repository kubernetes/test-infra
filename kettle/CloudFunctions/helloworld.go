// Package helloworld provides a set of Cloud Functions samples.
package helloworld

import (
	"context"
	"log"
)

// PubSubMessage is the payload of a Pub/Sub event.
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// HelloPubSub consumes a Pub/Sub message.
func HelloPubSub(ctx context.Context, m PubSubMessage) error {
	name := string(m.Data) // Automatically decoded from base64.
	if name == "" {
		name = "World"
	}
	log.Printf("Hello, %s!", name)
	return nil
}

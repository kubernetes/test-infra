/*
Copyright 2022 The Kubernetes Authors.

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
	"fmt"
	"log"
	"sync/atomic"
	"time"

	automl "cloud.google.com/go/automl/apiv1"
	"google.golang.org/api/option"
	automlpb "google.golang.org/genproto/googleapis/cloud/automl/v1"
)

func defaultPredictionClient(ctx context.Context) (*predictionClient, error) {
	return newPredictionClient(ctx, *projectID, *location, *model, *quotaProjectID)
}

func newPredictionClient(ctx context.Context, projectID, location, model, quotaProject string) (*predictionClient, error) {
	var opts []option.ClientOption
	if quotaProject != "" {
		opts = append(opts, option.WithQuotaProject(quotaProject))
	}
	client, err := automl.NewPredictionClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return &predictionClient{
		client:    client,
		modelName: modelName(projectID, location, model),
		ch:        throttle(ctx, *qps**burst, time.Second/time.Duration(*qps), *warmup),
	}, nil
}

type predictionClient struct {
	client    *automl.PredictionClient
	modelName string
	requests  int64
	ch        <-chan time.Time
}

func modelName(projectID, location, model string) string {
	return fmt.Sprintf("projects/%s/locations/%s/models/%s", projectID, location, model)
}

func throttle(ctx context.Context, capacity int, wait time.Duration, warmup bool) <-chan time.Time {
	ch := make(chan time.Time, capacity)
	go func() {
		tick := time.NewTicker(wait)
		now := time.Now()
		if warmup {
			for len(ch) < cap(ch) {
				select {
				case <-ctx.Done():
					return
				case ch <- now:
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case now = <-tick.C:
			}
			select {
			case <-ctx.Done():
				return
			case ch <- now:
			}
		}
	}()
	return ch
}

func (pc *predictionClient) predict(ctx context.Context, sentence string) (map[string]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-pc.ch:
	}
	n := atomic.AddInt64(&pc.requests, 1)
	req := automlpb.PredictRequest{
		Name: pc.modelName,
		Payload: &automlpb.ExamplePayload{
			Payload: &automlpb.ExamplePayload_TextSnippet{
				TextSnippet: &automlpb.TextSnippet{
					Content:  sentence,
					MimeType: "text/plain",
				},
			},
		},
	}

	resp, err := pc.client.Predict(ctx, &req)
	if n%100 == 0 || err != nil {
		log.Println("Prediction request", n, "remaining quota", len(pc.ch), err)
	}
	if err != nil {
		return nil, err
	}
	payloads := resp.GetPayload()
	out := make(map[string]float32, len(payloads))
	for _, payload := range payloads {
		out[payload.GetDisplayName()] = payload.GetClassification().GetScore()
	}
	return out, nil
}

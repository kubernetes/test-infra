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
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	"cloud.google.com/go/storage"
	ts "github.com/golang/protobuf/ptypes/timestamp"
	gzip "github.com/klauspost/pgzip"
	pb "github.com/kzmrv/logviewer/gcsreader/work"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	log "k8s.io/klog"
)

const (
	port       = ":17654"
	bucketName = "kubernetes-jenkins"
	lineBuffer = 100000
)

type serverType struct{}

type lineFilter struct {
	regex *regexp.Regexp
	since time.Time
	until time.Time
}

func main() {
	log.InitFlags(nil)

	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Infof("Listening on port: %v", port)
	server := grpc.NewServer()
	pb.RegisterWorkerServer(server, &serverType{})
	err = server.Serve(listener)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func (*serverType) DoWork(request *pb.Work, server pb.Worker_DoWorkServer) error {
	defer timeTrack(time.Now(), "Call duration")
	log.Infof("Received: file %v, substring %v, since %v, until %v",
		request.File, request.TargetSubstring, ptypes.TimestampString(request.Since), ptypes.TimestampString(request.Until))

	reader, err := downloadAndDecompress(request.File)
	if err != nil {
		return err
	}
	lineChannel := make(chan *lineEntry, lineBuffer)
	regex, err := regexp.Compile(request.TargetSubstring)
	if err != nil {
		return err
	}

	since, _ := ptypes.Timestamp(request.Since)
	until, _ := ptypes.Timestamp(request.Until)
	filters := &lineFilter{
		regex: regex,
		since: since,
		until: until,
	}

	go getMatchingLines(reader, lineChannel, filters)
	batchAndSend(lineChannel, server)

	return nil
}

func batchAndSend(ch chan *lineEntry, server pb.Worker_DoWorkServer) {
	lineCounter := 0
	const batchSize = 100
	for hasMoreBatches := true; hasMoreBatches; {
		batches := make([]*pb.LogLine, batchSize)
		i := 0
		for i < batchSize {
			line, hasMore := <-ch
			if line.err == io.EOF || !hasMore {
				hasMoreBatches = false
				break
			}
			if line.err != nil {
				log.Errorf("Failed to parse line with error %v", line.err)
				continue
			}

			entry := line.logEntry
			pbLine := &pb.LogLine{
				Entry:     *entry.log,
				Timestamp: &ts.Timestamp{Seconds: entry.time.Unix(), Nanos: int32(entry.time.Nanosecond())}}

			batches[i] = pbLine
			i++
		}

		if i != 0 {
			err := server.Send(&pb.WorkResult{LogLines: batches[:i]})
			if err != nil {
				log.Errorf("Failed to send result with: %v", err)
			}
			lineCounter += i
		}
	}

	log.Infof("Finished with %v lines", lineCounter)
}

func downloadAndDecompress(objectPath string) (io.Reader, error) {
	//return loadFromLocalFS(objectPath)
	reader, err := download(objectPath)
	if err != nil {
		return nil, err
	}

	decompressed, err := decompress(reader)
	if err != nil {
		return nil, err
	}
	return decompressed, nil
}

func download(objectPath string) (io.Reader, error) {
	context := context.Background()
	client, err := storage.NewClient(context, option.WithoutAuthentication())
	if err != nil {
		return nil, err
	}

	bucket := client.Bucket(bucketName)

	remoteFile := bucket.Object(objectPath).ReadCompressed(true)
	reader, err := remoteFile.NewReader(context)
	if err != nil {
		return nil, err
	}

	return reader, err
}

func decompress(reader io.Reader) (io.Reader, error) {
	newReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	return newReader, nil
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Infof("%s took %s", name, elapsed)
}

// for testing purposes
func loadFromLocalFS(objectPath string) (io.Reader, error) {
	const folder = "/Downloads/kubernetes-jenkins-310"
	idx := strings.LastIndex(objectPath, "/") + 1
	fileName := strings.TrimSuffix(objectPath[idx:], ".gz")
	home, _ := os.UserHomeDir()
	return os.Open(filepath.Join(home, folder, fileName))
}

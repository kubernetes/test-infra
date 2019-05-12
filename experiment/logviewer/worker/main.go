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
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	ts "github.com/golang/protobuf/ptypes/timestamp"
	pb "github.com/kzmrv/logviewer/worker/work"
	"google.golang.org/grpc"
	log "k8s.io/klog"
)

type dataReader interface {
	downloadAndDecompress(objectPath string) (io.Reader, error)
}

var (
	port = flag.Int("port", 17654, "Port to run grpc worker service")
)

const (
	bucketName = "kubernetes-jenkins"
	lineBuffer = 100000
)

type serverType struct {
	logReader dataReader
	send      func(ch chan *lineEntry, server pb.Worker_DoWorkServer)
}

type lineFilter struct {
	regex *regexp.Regexp
	since time.Time
	until time.Time
}

func main() {
	log.InitFlags(nil)
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Infof("Listening on port: %v", *port)
	server := grpc.NewServer()

	pb.RegisterWorkerServer(server, &serverType{logReader: &gcsReader{}, send: batchAndSend})
	err = server.Serve(listener)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func (srv *serverType) DoWork(request *pb.Work, server pb.Worker_DoWorkServer) error {
	defer timeTrack(time.Now(), "Call duration")
	log.Infof("Received: file %v, substring %v, since %v, until %v",
		request.File, request.TargetSubstring, ptypes.TimestampString(request.Since), ptypes.TimestampString(request.Until))

	reader, err := srv.logReader.downloadAndDecompress(request.File)
	if err != nil {
		return err
	}
	lineChannel := make(chan *lineEntry, lineBuffer)
	regex, err := regexp.Compile(request.TargetSubstring)
	if err != nil {
		return err
	}
	filters := &lineFilter{
		regex: regex,
	}

	if request.Since != nil {
		since, err := ptypes.Timestamp(request.Since)
		if err != nil {
			return fmt.Errorf("Unable to parse request.Since: %v", err)
		}
		filters.since = since
	}

	if request.Until != nil {
		until, err := ptypes.Timestamp(request.Until)
		if err != nil {
			return fmt.Errorf("Unable to parse request.Until: %v", err)
		}
		filters.until = until
	}

	go getMatchingLines(reader, lineChannel, filters)
	srv.send(lineChannel, server)

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

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Infof("%s took %s", name, elapsed)
}

type localFsReader struct{}

// for testing purposes
func (r *localFsReader) loadFromLocalFS(objectPath string) (io.Reader, error) {
	const folder = "/Downloads/kubernetes-jenkins-310"
	idx := strings.LastIndex(objectPath, "/") + 1
	fileName := strings.TrimSuffix(objectPath[idx:], ".gz")
	home, _ := os.UserHomeDir()
	return os.Open(filepath.Join(home, folder, fileName))
}

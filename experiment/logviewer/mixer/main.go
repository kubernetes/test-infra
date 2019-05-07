package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"

	"cloud.google.com/go/storage"
	pb "github.com/kzmrv/gcsreader/proto"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	log "k8s.io/klog"
)

const (
	address        = "localhost:17654"
	timeoutSeconds = 240
	bucketName     = "kubernetes-jenkins"
)

type userRequest struct {
	buildNumber     int
	filePrefix      string
	targetSubstring string
	since           time.Time
	until           time.Time
}

type callResult struct {
	workResult *pb.WorkResult
	err        error
}

func main() {
	log.InitFlags(nil)
	connections, err := initWorkers()
	if err != nil {
		log.Fatalln(err)
	}

	workers := make([]pb.WorkerClient, len(connections))
	for i, connection := range connections {
		workers[i] = pb.NewWorkerClient(connection)
		defer connection.Close()
	}

	request := getNextRequest()
	works, err := getWorks(request)

	if err != nil {
		log.Fatalln(err)
	}
	rpcResponses := make([]chan *callResult, len(works))
	var wg sync.WaitGroup
	wg.Add(len(works))
	for i, work := range works {
		rpcResponses[i] = make(chan *callResult, 100000)
		go dispatch(&wg, work, workers, rpcResponses[i])
	}

	wg.Wait()
	processWorkResults(rpcResponses, works)

	log.Info("App finished")
}

func processWorkResults(rpcResponses []chan *callResult, works []*pb.Work) {
	matchingLines := make([]*pb.LogLine, 0)
	for i := 0; i < len(works); i++ {
		counter := 0
		for {
			batchResult, hasMore := <-rpcResponses[i]
			if !hasMore {
				break
			}
			if batchResult.err != nil {
				log.Errorf("Error in result batch: %v", batchResult.err)
			} else {
				matchingLines = append(matchingLines, batchResult.workResult.LogLines...)
				counter += len(batchResult.workResult.LogLines)
			}

		}
		log.Infof("File %v found %d matching lines", works[i].File, len(matchingLines))
	}

	sort.Slice(matchingLines, func(less, greater int) bool {
		tsLess := *matchingLines[less].Timestamp
		tsGreater := *matchingLines[greater].Timestamp
		return tsLess.Seconds < tsGreater.Seconds ||
			(tsLess.Seconds == tsGreater.Seconds && tsLess.Nanos < tsGreater.Nanos)
	})

	for _, line := range matchingLines {
		log.Infof("%v : %v", ptypes.TimestampString(line.Timestamp), line.Entry)
	}
}

func initWorkers() ([]*grpc.ClientConn, error) {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	workers := []*grpc.ClientConn{
		conn,
	}
	return workers, nil
}

// TODO replace with real requests
func getNextRequest() *userRequest {
	since, _ := time.Parse(time.RFC3339Nano, "2019-02-15T15:38:48.908485Z")
	until, _ := time.Parse(time.RFC3339Nano, "2019-02-15T18:38:48.908485Z")

	return &userRequest{
		buildNumber:     310,
		filePrefix:      "kube-apiserver-audit.log-",
		targetSubstring: "9a27",
		since:           since,
		until:           until,
	}
}

var dispatchCounter = 0

// Round robin dispatch
func dispatch(wg *sync.WaitGroup, work *pb.Work, workers []pb.WorkerClient, rpcResponses chan *callResult) {
	defer close(rpcResponses)
	defer wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*timeoutSeconds)
	defer cancel()
	client, err := workers[dispatchCounter%len(workers)].DoWork(ctx, work)
	dispatchCounter++
	if err != nil {
		rpcResponses <- &callResult{err: err}
		return
	}
	for {
		workResult, err := client.Recv()
		if err == io.EOF {
			return
		}
		rpcResponses <- &callResult{workResult: workResult, err: err}
	}
}

func getWorks(request *userRequest) ([]*pb.Work, error) {
	prefix := fmt.Sprintf("logs/ci-kubernetes-e2e-gce-scale-performance/%v/artifacts/gce-scale-cluster-master/", request.buildNumber)
	files, err := getFiles(prefix, request.filePrefix)
	if err != nil {
		return nil, err
	}

	works := make([]*pb.Work, len(files))

	since, _ := ptypes.TimestampProto(request.since)
	until, _ := ptypes.TimestampProto(request.until)
	for i, file := range files {
		work := &pb.Work{
			File:            file.Name,
			TargetSubstring: request.targetSubstring,
			Since:           since,
			Until:           until,
		}
		works[i] = work
	}
	return works, nil
}

func getFiles(prefix string, substring string) ([]*storage.ObjectAttrs, error) {
	context := context.Background()
	client, err := storage.NewClient(context, option.WithoutAuthentication())
	if err != nil {
		return nil, err
	}

	bucket := client.Bucket(bucketName)
	allFiles := bucket.Objects(context, &storage.Query{Prefix: prefix})
	result := make([]*storage.ObjectAttrs, 0, allFiles.PageInfo().Remaining())
	var attr *storage.ObjectAttrs
	for {
		attr, err = allFiles.Next()
		if err != nil {
			break
		}
		if strings.Contains(attr.Name, substring) {
			result = append(result, attr)
		}
	}
	if err == iterator.Done {
		return result, nil
	}
	return nil, err
}

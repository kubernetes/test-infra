package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"

	"cloud.google.com/go/storage"
	mixerPb "github.com/kzmrv/logviewer/mixer/request"
	pb "github.com/kzmrv/logviewer/worker/work"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	log "k8s.io/klog"
)

var (
	// Todo support & discovery for multiple workers
	address         = flag.String("address", "localhost:17654", "Worker address")
	mixerServerPort = flag.Int("port", 17655, "Mixer server port")
)

const (
	timeoutSeconds = 300
	bucketName     = "kubernetes-jenkins"
	batchSize      = 100
)

func main() {
	setup()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *mixerServerPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	mixerPb.RegisterMixerServiceServer(grpcServer, &mixerServer{})
	log.Infof("Listening on port %v", *mixerServerPort)
	grpcServer.Serve(lis)
}

func (*mixerServer) DoWork(request *mixerPb.MixerRequest, server mixerPb.MixerService_DoWorkServer) error {
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
	lines := processWorkResults(rpcResponses)

	batchCount := (len(lines) + 1) / batchSize
	for i := 0; i < batchCount; i++ {
		err := server.Send(&mixerPb.MixerResult{LogLines: lines[batchSize*i : batchSize*(i+1)]})
		if err != nil {
			return err
		}
	}
	return nil
}

var workers []pb.WorkerClient

func setup() func() {
	log.InitFlags(nil)
	flag.Parse()

	connections, err := initWorkers()
	if err != nil {
		log.Fatalln(err)
	}

	workers = make([]pb.WorkerClient, len(connections))
	for i, connection := range connections {
		workers[i] = pb.NewWorkerClient(connection)
	}

	cl := func() {
		for _, conn := range connections {
			conn.Close()
		}
	}
	return cl
}

func localMain() {
	closeConns := setup()
	defer closeConns()
	request := getSampleRequest()
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
	processWorkResults(rpcResponses)

	log.Info("App finished")
}

func processWorkResults(rpcResponses []chan *callResult) []*pb.LogLine {
	matchingLines := make([]*pb.LogLine, 0)
	for i := 0; i < len(rpcResponses); i++ {
		for {
			batchResult, hasMore := <-rpcResponses[i]
			if !hasMore {
				break
			}
			if batchResult.err != nil {
				log.Errorf("Error in result batch: %v", batchResult.err)
				continue
			}
			matchingLines = append(matchingLines, batchResult.workResult.LogLines...)
		}
		log.Infof("Work #%v found %v matching lines", i, len(matchingLines))
	}

	sort.Slice(matchingLines, func(less, greater int) bool {
		tsLess := *matchingLines[less].Timestamp
		tsGreater := *matchingLines[greater].Timestamp
		return tsLess.Seconds < tsGreater.Seconds ||
			(tsLess.Seconds == tsGreater.Seconds && tsLess.Nanos < tsGreater.Nanos)
	})

	return matchingLines
}

func initWorkers() ([]*grpc.ClientConn, error) {
	conn, err := grpc.Dial(*address, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	workers := []*grpc.ClientConn{
		conn,
	}
	return workers, nil
}

func getSampleRequest() *mixerPb.MixerRequest {
	since, _ := time.Parse(time.RFC3339Nano, "2019-02-15T15:38:48.908485Z")
	until, _ := time.Parse(time.RFC3339Nano, "2019-02-15T18:38:48.908485Z")
	pSince, _ := ptypes.TimestampProto(since)
	pUntil, _ := ptypes.TimestampProto(until)

	return &mixerPb.MixerRequest{
		BuildNumber:     310,
		FilePrefix:      "kube-apiserver-audit.log-",
		TargetSubstring: "9a27",
		Since:           pSince,
		Until:           pUntil,
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

func getWorks(request *mixerPb.MixerRequest) ([]*pb.Work, error) {
	prefix := fmt.Sprintf("logs/ci-kubernetes-e2e-gce-scale-performance/%v/artifacts/gce-scale-cluster-master/", request.BuildNumber)
	files, err := getFiles(prefix, request.FilePrefix)
	if err != nil {
		return nil, err
	}

	works := make([]*pb.Work, len(files))

	for i, file := range files {
		work := &pb.Work{
			File:            file.Name,
			TargetSubstring: request.TargetSubstring,
			Since:           request.Since,
			Until:           request.Until,
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

type mixerServer struct{}

type callResult struct {
	workResult *pb.WorkResult
	err        error
}

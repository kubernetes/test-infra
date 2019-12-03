package main

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
)


func listInstances(computeService *compute.Service, project string, zone string) []*compute.Instance {
	instanceListCall := computeService.Instances.List(project, zone)
	instanceList, err := instanceListCall.Do()
	if err != nil {
		logrus.WithError(err).Infof("error listing VM Instances in project %s, zone %s", project, zone)
	}
	return instanceList.Items
}

func deleteInstance(ctx context.Context, computeService *compute.Service, project string, zone string, instanceName string) *compute.Operation {
	resp, err := computeService.Instances.Delete(project, zone, instanceName).Context(ctx).Do()
	if err != nil {
		logrus.WithError(err).Infof("error listing VM Instances in project %s, zone %s", project, zone)
	}
	waitForOperationToComplete(resp, ctx, computeService, project, zone)
	return resp
}

func waitForOperationToComplete(operation *compute.Operation, ctx context.Context, computeService *compute.Service, project string, zone string) {
	opName := operation.Name
	status := ""
	for status != "DONE" {
		logrus.Infof("Waiting for operation %s to complete.\n", opName)
		time.Sleep(time.Millisecond * 5000)
		resp, err := computeService.ZoneOperations.Get(project, zone, opName).Context(ctx).Do()
		if err != nil {
			logrus.WithError(err).Infof("error getting info for operation %s", opName)
		}
		logrus.Infof("%#v\n", resp.Status)
		status = resp.Status
	}

}

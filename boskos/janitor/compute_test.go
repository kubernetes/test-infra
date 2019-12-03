package main

import (
	"testing"

	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

func TestDelete(t *testing.T) {
	ctx := context.Background()

	c, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		t.Errorf("Error creating Google Cloud client: %s", err)
	}

	computeService, err := compute.New(c)
	if err != nil {
		t.Errorf("Error creating Compute API object: %s", err)
	}
	project := "hercules-anthos"
	zone := "us-west1-a"
	name := "test-" + uuid.New()
	rb := &compute.Instance{
		MachineType: "zones/us-west1-a/machineTypes/n1-standard-1",
		Name:        name,
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "external-nat",
					},
				},
				Network: "global/networks/default",
			},
		},
		Disks: []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				Type:       "PERSISTENT",
				InitializeParams: &compute.AttachedDiskInitializeParams{
					DiskSizeGb:  100,
					SourceImage: "projects/debian-cloud/global/images/family/debian-9",
				},
			},
		},
	}
	resp, err := computeService.Instances.Insert(project, zone, rb).Context(ctx).Do()
	if err != nil {
		t.Errorf("Error creating VM Instance in project %s, zone %s: %s", project, zone, err)
	}
	waitForOperationToComplete(resp, ctx, computeService, project, zone)
	deleteInstance(ctx, computeService, project, zone, name)
}

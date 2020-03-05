package compute_cleaner

import (
	"testing"

	"github.com/pborman/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"

	"k8s.io/test-infra/boskos/cmd/janitor/gcp"
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

	cleaner, err := compute_cleaner.NewCleaner(project, zone, computeService, ctx)
	if err != nil {
		t.Fatalf("failed to create the Compute cleaner")
	}
	logrus.Infof("Creating instance %s", name)
	cleaner.Create(rb)
	if err != nil {
		t.Errorf("Error creating VM Instance in project %s, zone %s: %s", project, zone, err)
	}
	logrus.Infof("Deleting instance %s", name)
	cleaner.Delete(name)
}

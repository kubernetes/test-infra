package compute_cleaner

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

type ComputeCleaner struct {
	project        string
	zone           string
	computeService *compute.Service
	ctx            context.Context
	opName         string
}

func NewCleaner(project, zone string, computeService *compute.Service, ctx context.Context) (*ComputeCleaner, error) {
	if (project == "") != (zone == "") {
		return nil, fmt.Errorf("project and zone must be specified together")
	}

	cleaner := &ComputeCleaner{
		project:        project,
		zone:           zone,
		ctx:            ctx,
		computeService: computeService,
	}
	return cleaner, nil
}

func (c *ComputeCleaner) List() ([]*compute.Instance, error) {
	instanceListCall := c.computeService.Instances.List(c.project, c.zone)
	instanceList, err := instanceListCall.Do()
	if err != nil {
		return nil, err
	}
	return instanceList.Items, nil
}

func (c *ComputeCleaner) Delete(instanceName string) error {
	resp, err := c.computeService.Instances.Delete(c.project, c.zone, instanceName).Context(c.ctx).Do()
	if err != nil {
		return err
	}
	c.opName = resp.Name
	wait.PollImmediate(10*time.Second, 15*time.Minute,
		c.waitForOperationComplete)
	return nil
}

func (c *ComputeCleaner) Create(info *compute.Instance) error {
	resp, err := c.computeService.Instances.Insert(c.project, c.zone, info).Context(c.ctx).Do()
	if err != nil {
		return err
	}
	c.opName = resp.Name
	wait.PollImmediate(10*time.Second, 15*time.Minute, c.waitForOperationComplete)
	return nil
}

func (c *ComputeCleaner) waitForOperationComplete() (bool, error) {
	resp, err := c.computeService.ZoneOperations.Get(c.project, c.zone, c.opName).Context(c.ctx).Do()
	if err != nil {
		return false, err
	}
	if resp.Status != "DONE" {
		return false, nil
	}
	return true, nil
}

package cleaners

import (
	"google.golang.org/api/compute/v1"
)

type cleaners interface {
	List() ([]*compute.Instance, error)
	Delete(instanceName string) error
	Create(instanceInfo *compute.Instance) error
}

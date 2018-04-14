package options

import (
	"flag"
	"fmt"
	"net"
)

type Options struct {
	SideloadImage bool
	DinDNodeImage string
	ProxyAddr     string
	Version       string
	NumNodes      int
}

// New takes a FlagSet, parses it, and creates an Options struct. This calls Parse, so add any external flags before creating an Options object.
func New(set *flag.FlagSet, args []string) (*Options, error) {
	o := Options{}

	set.BoolVar(&o.SideloadImage, "side-load-image", true, "If the image needs to be side-loaded from the file-system.")
	set.StringVar(&o.DinDNodeImage, "dind-node-image", "k8s.gcr.io/dind-node-amd64", "The dind node image to use.")
	set.StringVar(&o.ProxyAddr, "proxy-addr", "", "The externally facing address for kubeadm to add to SAN.")
	set.StringVar(&o.Version, "k8s-version", "", "The kubernetes version to spin up.")
	set.IntVar(&o.NumNodes, "num-nodes", 4, "The number of nodes to make, including the master if applicable.")

	if err := set.Parse(args); err != nil {
		return nil, err
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return &o, nil
}

func (o *Options) Validate() error {
	if o.NumNodes < 1 {
		return fmt.Errorf("Must provide at least 1 node, got %d", o.NumNodes)
	}

	// The ParseIP function returns a nil if it's not a valid ipv4 or ipv6 address.
	ip := net.ParseIP(o.ProxyAddr)
	if o.ProxyAddr != "" && ip == nil {
		return fmt.Errorf("the external proxy %q is not a valid ip address", o.ProxyAddr)
	}

	return nil
}

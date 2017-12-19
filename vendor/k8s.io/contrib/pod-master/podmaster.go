/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

// podmaster is a simple utility, it attempts to acquire and maintain a lease-lock from etcd using compare-and-swap.
// if it is the master, it copies a source file into a destination file.  If it is not the master, it makes sure it is removed.
//
// typical usage is to copy a Pod manifest from a staging directory into the kubelet's directory, for example:
//   podmaster --etcd-servers=http://127.0.0.1:4001 --key=scheduler --source-file=/kubernetes/kube-scheduler.manifest --dest-file=/manifests/kube-scheduler.manifest
package main

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	etcdutil "k8s.io/kubernetes/pkg/storage/etcd/util"

	etcd "github.com/coreos/etcd/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/net/context"
)

type config struct {
	etcdServers []string
	key         string
	whoami      string
	ttl         uint64
	src         string
	dest        string
	sleep       time.Duration
	lastLease   time.Time
}

// runs the election loop. never returns.
func (c *config) leaseAndUpdateLoop(etcdClient *etcd.Client) {
	for {
		master, err := c.acquireOrRenewLease(etcdClient)
		if err != nil {
			glog.Errorf("Error in master election: %v", err)
			if uint64(time.Now().Sub(c.lastLease).Seconds()) < c.ttl {
				continue
			}
			// Our lease has expired due to our own accounting, pro-actively give it
			// up, even if we couldn't contact etcd.
			glog.Infof("Too much time has elapsed, giving up lease.")
			master = false
		}
		if err := c.update(master); err != nil {
			glog.Errorf("Error updating files: %v", err)
		}
		time.Sleep(c.sleep)
	}
}

// acquireOrRenewLease either races to acquire a new master lease, or update the existing master's lease
// returns true if we have the lease, and an error if one occurs.
// TODO: use the master election utility once it is merged in.
func (c *config) acquireOrRenewLease(etcdClient *etcd.Client) (bool, error) {
	keysAPI := etcd.NewKeysAPI(*etcdClient)
	resp, err := keysAPI.Get(context.TODO(), c.key, nil)
	if err != nil {
		if etcdutil.IsEtcdNotFound(err) {
			// there is no current master, try to become master, create will fail if the key already exists
			opts := etcd.SetOptions{
				TTL:       time.Duration(c.ttl) * time.Second,
				PrevExist: "",
			}
			_, err := keysAPI.Set(context.TODO(), c.key, c.whoami, &opts)
			if err != nil {
				return false, err
			}
			c.lastLease = time.Now()
			return true, nil
		}
		return false, err
	}
	if resp.Node.Value == c.whoami {
		glog.Infof("key already exists, we are the master (%s)", resp.Node.Value)
		// we extend our lease @ 1/2 of the existing TTL, this ensures the master doesn't flap around
		if resp.Node.Expiration.Sub(time.Now()) < time.Duration(c.ttl/2)*time.Second {
			opts := etcd.SetOptions{
				TTL:       time.Duration(c.ttl) * time.Second,
				PrevValue: c.whoami,
				PrevIndex: resp.Node.ModifiedIndex,
			}
			_, err := keysAPI.Set(context.TODO(), c.key, c.whoami, &opts)
			if err != nil {
				return false, err
			}
		}
		c.lastLease = time.Now()
		return true, nil
	}
	glog.Infof("key already exists, the master is %s, sleeping.", resp.Node.Value)
	return false, nil
}

// update enacts the policy, copying a file if we are the master, and it doesn't exist.
// deleting a file if we aren't the master and it does.
func (c *config) update(master bool) error {
	exists, err := exists(c.dest)
	if err != nil {
		return err
	}
	switch {
	case master && !exists:
		return copyFile(c.src, c.dest)
	case master && exists && sum(c.src) != sum(c.dest):
		return copyFile(c.src, c.dest)
	case !master && exists:
		return os.Remove(c.dest)
	}
	return nil
}

// sum returns in string the SHA-1 hash for the input file, error if there is any
func sum(file string) string {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		glog.Errorf("Error reading file %s: %v", file, err)
	}
	hash := sha1.New()
	_, err = hash.Write(data)
	if err != nil {
		glog.Errorf("Error copying file %s: %v", file, err)
	}
	return fmt.Sprintf("% x", hash.Sum(nil))
}

// exists tests to see if a file exists.
func exists(file string) (bool, error) {
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func copyFile(src, dest string) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dest, data, 0755)
}

func initFlags(c *config) {
	pflag.StringSliceVar(&c.etcdServers, "etcd-servers", []string{}, "The comma-seprated list of etcd servers to use")
	pflag.StringVar(&c.key, "key", "", "The key to use for the lock")
	pflag.StringVar(&c.whoami, "whoami", "", "The name to use for the reservation.  If empty use os.Hostname")
	pflag.Uint64Var(&c.ttl, "ttl-secs", 30, "The time to live for the lock.")
	pflag.StringVar(&c.src, "source-file", "", "The source file to copy from.")
	pflag.StringVar(&c.dest, "dest-file", "", "The destination file to copy to.")
	pflag.DurationVar(&c.sleep, "sleep", 5*time.Second, "The length of time to sleep between checking the lock.")
}

func validateFlags(c *config) {
	if len(c.etcdServers) == 0 {
		glog.Fatalf("--etcd-servers=<server-list> is required")
	}
	if len(c.key) == 0 {
		glog.Fatalf("--key=<some-key> is required")
	}
	if len(c.src) == 0 {
		glog.Fatalf("--source-file=<some-file> is required")
	}
	if len(c.dest) == 0 {
		glog.Fatalf("--dest-file=<some-file> is required")
	}
	if len(c.whoami) == 0 {
		hostname, err := os.Hostname()
		if err != nil {
			glog.Fatalf("Failed to get hostname: %v", err)
		}
		c.whoami = hostname
		glog.Infof("--whoami is empty, defaulting to %s", c.whoami)
	}
}

func main() {
	c := config{}
	initFlags(&c)
	pflag.Parse()
	validateFlags(&c)

	cfg := etcd.Config{
		Endpoints: c.etcdServers,
	}
	etcdClient, err := etcd.New(cfg)
	if err != nil {
		glog.Fatalf("misconfigured etcd: %v", err)
	}

	c.leaseAndUpdateLoop(&etcdClient)
}

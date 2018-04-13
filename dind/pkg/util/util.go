/*
Copyright 2018 The Kubernetes Authors.

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

package util

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// Version reads the version from the metadata stored in the container.
func DockerImageVersion() (string, error) {
	f, err := os.Open("/docker_version")
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		return scanner.Text(), nil
	}
	return "", fmt.Errorf("Empty version file")
}

func NonLocalUnicastAddr(intfName string) (string, error) {
	// Finds a non-localhost unicast address on the given interface.
	intf, err := net.InterfaceByName(intfName)
	if err != nil {
		return "", err
	}
	addrs, err := intf.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if !strings.HasPrefix("127", addr.String()) {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return "", err
			}

			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("No non-localhost unicast addresses found on interface %s", intfName)
}

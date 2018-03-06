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

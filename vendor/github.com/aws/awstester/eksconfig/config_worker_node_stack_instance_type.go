package eksconfig

import "github.com/aws/awstester/pkg/awsapi/ec2"

func checkEC2InstanceType(s string) (ok bool) {
	_, ok = ec2.InstanceTypes[s]
	return ok
}

func checkMaxPods(s string, nodesN, serverReplicas int) (ok bool) {
	var v *ec2.InstanceType
	v, ok = ec2.InstanceTypes[s]
	if !ok {
		return false
	}
	maxPods := v.MaxPods * int64(nodesN)
	if int64(serverReplicas) > maxPods {
		return false
	}
	return true
}

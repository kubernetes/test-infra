package eksconfig

import "fmt"

func genServiceRoleWithPolicy(clusterName string) string {
	return fmt.Sprintf("%s-SERVICE-ROLE", clusterName)
}

const (
	serviceRolePolicyARNService = "arn:aws:iam::aws:policy/AmazonEKSServicePolicy"
	serviceRolePolicyARNCluster = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
)

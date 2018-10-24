package eksconfig

import "fmt"

func genCFStackVPC(clusterName string) string {
	return fmt.Sprintf("%s-VPC-STACK", clusterName)
}

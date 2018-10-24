package eksconfig

import "fmt"

func genCFStackWorkerNodeGroup(clusterName string) string {
	return fmt.Sprintf("%s-NODE-GROUP-STACK", clusterName)
}

package eksconfig

import "fmt"

func genNodeGroupKeyPairName(clusterName string) string {
	return fmt.Sprintf("%s-KEY-PAIR", clusterName)
}

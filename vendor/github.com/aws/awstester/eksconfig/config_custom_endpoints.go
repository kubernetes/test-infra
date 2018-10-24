package eksconfig

import "sort"

var (
	// supportedEKSEps maps each test environments to EKS endpoint.
	supportedEKSEps = map[string]struct{}{
		// TODO: support EKS testing endpoint
		// https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#custom-endpoint
		// "https://test.us-west-2.amazonaws.com" : struct{}{},
	}

	allEKSEps = []string{}
)

func init() {
	allEKSEps = make([]string, 0, len(supportedEKSEps))
	for k := range supportedEKSEps {
		allEKSEps = append(allEKSEps, k)
	}
	sort.Strings(allEKSEps)
}

func checkEKSEp(s string) (ok bool) {
	if s == "" { // prod
		return true
	}
	_, ok = supportedEKSEps[s]
	return ok
}

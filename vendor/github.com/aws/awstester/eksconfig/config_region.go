package eksconfig

var (
	// supportedRegions is a list of currently EKS supported AWS regions.
	// See https://aws.amazon.com/about-aws/global-infrastructure/regional-product-services.
	supportedRegions = map[string]struct{}{
		"us-west-2": {},
		"us-east-1": {},
		"eu-west-1": {},
	}
)

func checkRegion(s string) (ok bool) {
	_, ok = supportedRegions[s]
	return ok
}

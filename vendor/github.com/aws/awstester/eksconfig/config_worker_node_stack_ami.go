package eksconfig

// https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
// https://docs.aws.amazon.com/eks/latest/userguide/eks-optimized-ami.html
var regionToAMICPU = map[string]string{
	"us-west-2": "ami-0a54c984b9f908c81",
	"us-east-1": "ami-0440e4f6b9713faf6",
	"eu-west-1": "ami-0c7a4976cb6fafd3a",
}

// https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
// https://docs.aws.amazon.com/eks/latest/userguide/eks-optimized-ami.html
var regionToAMIGPU = map[string]string{
	"us-west-2": "ami-0731694d53ef9604b",
	"us-east-1": "ami-058bfb8c236caae89",
	"eu-west-1": "ami-0706dc8a5eed2eed9",
}

func checkAMI(region, imageID string) (ok bool) {
	var id string
	id, ok = regionToAMICPU[region]
	if !ok {
		id, ok = regionToAMIGPU[region]
		if !ok {
			return false
		}
	}
	return id == imageID
}

package eksconfig

// supportedKubernetesVersions is a list of EKS supported Kubernets versions.
var supportedKubernetesVersions = map[string]struct{}{
	"1.10": {},
}

func checkKubernetesVersion(s string) (ok bool) {
	_, ok = supportedKubernetesVersions[s]
	return ok
}

package integration

import (
	"testing"

	"k8s.io/test-infra/prow/gerrit/client"
)

const (
	gerritServer = "http://localhost/fakegerritserver"
)

func TestGerrit(t *testing.T) {
	t.Parallel()

	gerritClient, err := client.NewClient(map[string][]string{gerritServer: []string{"fakegerritserver"}})
	if err != nil {
		t.Fatalf("Failed creating gerritClient: %v", err)
	}

	resp, err := gerritClient.GetChange(gerritServer, "1")
	if err != nil {
		t.Fatalf("Failed getting gerrit change: %v", err)
	}
	if resp.ChangeID != "1" {
		t.Errorf("Did not return expected ChangeID. Want: %q, got: %q", "1", resp.ChangeID)
	}
}

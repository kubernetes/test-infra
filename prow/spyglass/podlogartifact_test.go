package spyglass

import (
	"bytes"
	"testing"
)

func TestPodLogReadAll(t *testing.T) {
	testCases := []struct {
		name     string
		artifact *PodLogArtifact
		expected []byte
	}{
		{
			name:     "Job Podlog readall",
			artifact: NewPodLogArtifact("job", "123", fakeJa),
			expected: []byte("clusterA"),
		},
		{
			name:     "Jib Podlog readall",
			artifact: NewPodLogArtifact("jib", "123", fakeJa),
			expected: []byte("clusterB"),
		},
	}
	for _, tc := range testCases {
		res, err := tc.artifact.ReadAll()
		if err != nil {
			t.Fatalf("%s failed reading bytes of log. err: %s", tc.name, err)
		}
		if !bytes.Equal(tc.expected, res) {
			t.Errorf("Unexpected result of reading pod logs, expected %s, got %s", string(tc.expected), string(res))
		}

	}

}

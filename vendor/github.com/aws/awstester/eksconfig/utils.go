package eksconfig

import (
	"math/rand"
	"os"
	"time"
)

func newInt(v int) *int {
	return &v
}

func exist(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

const ll = "0123456789abcdefghijklmnopqrstuvwxyz"

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		rand.Seed(time.Now().UTC().UnixNano())
		b[i] = ll[rand.Intn(len(ll))]
	}
	return string(b)
}

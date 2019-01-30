package hashx

import (
	"crypto/sha1"
	"fmt"
)

const (
	shortLen = 8
)

func Short(strs ...string) string {
	h := sha1.New()
	for _, s := range strs {
		h.Write([]byte(s))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:shortLen]
}

func PrefixShort(prefix string, strs ...string) string {
	h := sha1.New()
	for _, s := range strs {
		h.Write([]byte(s))
	}
	sum := fmt.Sprintf("%s%x", prefix, h.Sum(nil))
	return sum[:len(prefix)+shortLen]
}

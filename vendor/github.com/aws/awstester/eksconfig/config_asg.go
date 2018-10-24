package eksconfig

const (
	defaultASGMin = 2
	defaultASGMax = 2
)

func checkWorkderNodeASG(min, max int) (ok bool) {
	if min == 0 || max == 0 {
		return false
	}
	if min > max {
		return false
	}
	return true
}

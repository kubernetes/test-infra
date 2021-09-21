package fuzztest

func Fuzz(data []byte) int {
	testData := []string{}
	if len(data) < 2 {
		return 0
	}
	if string(data[0]) == "h" && string(data[1]) == "i" {
		x := testData[10000]
		return int(len(x))
	}
	return 0
}

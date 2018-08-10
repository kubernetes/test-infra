package testgrid

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// get a list a sub-directories that contains source code. The list will be shown on Testgrid
func getDirs(coverageStdout string) (res []string) {
	file, err := os.Open(coverageStdout)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		res = append(res, strings.Split(scanner.Text(), "\t")[1])
	}
	return
}

/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func annotateBuild(ctx context.Context, gcsClient gcs.ConditionalClient, predictor *predictionClient, build gcs.Path) ([]int, string, error) {

	log.Println("Analyzing:", build)

	sentences, err := readLines(ctx, gcsClient, build)
	if err != nil {
		return nil, "", fmt.Errorf("read lines: %v", err)
	}

	lines, err := predictByPage(ctx, predictor, sentences...)
	if err != nil {
		return nil, "", err
	}
	min, max := minMax(lines)
	const window = 5
	min -= window
	max += window
	if min < 0 {
		min = 0
	}
	if max >= len(sentences) {
		max = len(sentences) - 1
	}

	return lines, strings.Join(sentences[min:max+1], "\n"), nil
}

func readLines(ctx context.Context, client gcs.ConditionalClient, path gcs.Path) ([]string, error) {
	r, _, err := client.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	scanner := bufio.NewScanner(r)
	var sentences []string
	var lineno int
	for scanner.Scan() {
		lineno++
		txt := scanner.Text()
		if t := truncateLine(txt, *sentenceLen); t != nil {
			txt = *t
		}
		sentences = append(sentences, txt)
	}

	if err := scanner.Err(); err != nil {
		lineno++
		return sentences, fmt.Errorf("%d: %w", lineno, err)
	}

	return sentences, nil

}

func truncateLine(s string, n int) *string {
	if n <= 0 || len(s) <= n {
		return nil
	}
	half := n / 2
	s = strings.ToValidUTF8(s[:half-2]+"..."+s[len(s)-half+1:], "")
	return &s
}

var (
	predictLock sync.Mutex
)

func predictByPage(ctx context.Context, predictor *predictionClient, sentences ...string) ([]int, error) {
	predictLock.Lock() // allocate all quota to a single request at a time
	scores, err := predictSentencesByPage(ctx, predictor, sentences...)
	predictLock.Unlock()
	if err != nil {
		return nil, err
	}

	var maxScore float32
	var maxIdx int

	var more int

	const (
		threshold = 0.5
		window    = 5
	)
	for n, score := range scores {
		if score > maxScore {
			maxIdx = n
			maxScore = score
		}
		var notice string
		if score > threshold {
			notice = "+++"
			if more == 0 && !*additional {
				for i := n - window; i < n; i++ {
					if i < 0 {
						continue
					}
					println(i+1, "---", scores[i], sentences[i])
				}
			}
			more = window
		} else {
			notice = "---"
		}
		if more > 0 || *additional {
			println(n+1, notice, score, sentences[n])
			more--
		}
	}

	start, end := maxIdx, maxIdx
	for start > 0 && scores[start-1] >= threshold {
		start--
	}

	for end+1 < len(scores) && scores[end+1] >= threshold {
		end++
	}

	if !*additional {
		for i := start - window; i <= end+window; i++ {
			if i < 0 {
				continue
			}
			if i >= len(sentences) {
				break
			}
			var notice string
			score := scores[i]
			if score > threshold {
				notice = "+++"
			} else {
				notice = "---"
			}
			println(i+1, notice, score, sentences[i])
		}
	}

	return []int{start + 1, end + 1}, nil
}

func predictSentencesByPage(ctx context.Context, predictor *predictionClient, sentences ...string) ([]float32, error) {
	pages := splitPages(sentences, *sentenceLen, *documentLen)
	if len(pages) == 0 {
		return nil, nil
	}

	log.Printf("Found %d pages in %d lines", len(pages), len(sentences))

	const (
		maxRequestLen = 128000
		maxPages      = 100
	)
	if bytesPerPage := len(pages) * *documentLen / maxPages; bytesPerPage > maxRequestLen {
		return nil, fmt.Errorf("compressing %d pages to %d pages would make %d byte requests", len(pages), maxPages, bytesPerPage)
	}

	trunc := truncatePages(pages, maxPages)
	if len(trunc) != len(pages) {
		log.Printf("Truncated %d pages to %d", len(pages), len(trunc))
		pages = trunc
	}

	scores := make([]float32, len(sentences))
	highlights, err := predictPages(ctx, predictor, pages)
	if err != nil {
		return nil, fmt.Errorf("predict: %w", err)
	}

	var line int
	for n, score := range highlights {
		for more := len(pages[n]); more > 0; more-- {
			scores[line] = score
			line++
		}
	}

	return scores, nil
}

func splitPages(lines []string, lineLen, pageLen int) [][]string {
	var pages [][]string

	var working int

	var page []string
	for _, txt := range lines {
		if t := truncateLine(txt, lineLen); t != nil {
			txt = *t
		}
		n := len(txt)
		if n+working > pageLen {
			if len(page) > 0 {
				pages = append(pages, page)
			}
			page = nil
			working = 0
		}
		page = append(page, txt)
		working += n
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	return pages
}

func truncatePages(pages [][]string, maxPages int) [][]string {
	n := len(pages)
	if n <= maxPages {
		return pages
	}

	join := n / maxPages

	if n%maxPages != 0 {
		join++
	}

	out := make([][]string, 0, maxPages)

	for i := 0; i < n; i += join {
		chapter := pages[i : i+join]
		var total int
		for _, pages := range chapter {
			total += len(pages)
		}
		bigPage := make([]string, 0, total)
		for _, pages := range chapter {
			bigPage = append(bigPage, pages...)
		}

		out = append(out, bigPage)
	}

	return out
}

func predictPages(ctx context.Context, predictor *predictionClient, pages [][]string) ([]float32, error) {
	highlights := make([]float32, len(pages))

	ch := make(chan int)
	errCh := make(chan error)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	const workers = 10

	for i := 0; i < workers; i++ {
		go func() {
			for n := range ch {
				page := pages[n]
				txt := strings.Join(page, "\n")
				results, err := predictor.predict(ctx, txt)
				if err != nil {
					select {
					case <-ctx.Done():
					case errCh <- fmt.Errorf("%d (%s): %w", n, page, err):
					}
					return
				}
				const goal = "highlight"
				highlights[n] = results[goal]
			}
			select {
			case <-ctx.Done():
			case errCh <- nil:
			}
		}()
	}

	go func() {
		for n := range pages {
			select {
			case <-ctx.Done():
			case ch <- n:
			}
		}
		close(ch)
	}()

	for i := workers; i > 0; i-- {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		}
	}

	return highlights, nil
}

func println(stuff ...interface{}) {
	if !*shout {
		return
	}
	fmt.Println(stuff...)
}

func minMax(lines []int) (int, int) {
	var min, max int
	for i, l := range lines {
		if i == 0 || l < min {
			min = l
		}
		if i == 0 || l > max {
			max = l
		}
	}
	return min, max
}

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type WriteCounter struct {
	Filename   string
	Total      uint64
	Downloaded uint64
	Error      bool
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += uint64(n)
	return n, nil
}

func (wc *WriteCounter) CalculateProgressPercent() uint64 {
	return (wc.Downloaded * 100) / wc.Total
}

type Progress struct {
	Mutex     sync.RWMutex
	Counters  map[string]*WriteCounter
	FileNames []string
}

func (p *Progress) AddCounter(wc *WriteCounter) {
	p.Mutex.Lock()
	defer p.Mutex.Unlock()

	p.Counters[wc.Filename] = wc

	keys := make([]string, len(p.Counters))
	i := 0
	for k := range p.Counters {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	p.FileNames = keys
}

func (p *Progress) PrintProgress() {
	p.Mutex.RLock()
	defer p.Mutex.RUnlock()

	fmt.Printf("\r")
	for _, key := range p.FileNames {
		wc := p.Counters[key]
		if p.Counters[key].Error {
			fmt.Printf("%s error | ", wc.Filename)
		} else {
			fmt.Printf("%s %d%% | ", wc.Filename, wc.CalculateProgressPercent())
		}
	}
}

func main() {
	urls := withoutDuplicates(os.Args[1:])
	progress := &Progress{Counters: make(map[string]*WriteCounter)}
	wg := sync.WaitGroup{}

	fmt.Println("Download started...")

	for _, arg := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			downloadFile(url, progress)
		}(arg)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for range ticker.C {
			progress.PrintProgress()
		}
	}()

	wg.Wait()
	ticker.Stop()
	progress.PrintProgress()

	fmt.Println("\nDownload finished.")
}

func getFileName(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func withoutDuplicates(items []string) []string {
	duplicatesMap := make(map[string]bool)
	var list []string
	for _, item := range items {
		if _, value := duplicatesMap[item]; !value {
			duplicatesMap[item] = true
			list = append(list, item)
		}
	}
	return list
}

func downloadFile(url string, progress *Progress) {
	fileName := getFileName(url)
	wc := &WriteCounter{Filename: fileName}
	resp, err := http.Get(url)
	if err != nil {
		wc.Error = true
		progress.AddCounter(wc)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(fileName)
	if err != nil {
		wc.Error = true
		progress.AddCounter(wc)
		return
	}
	defer out.Close()

	wc.Total = uint64(resp.ContentLength)
	progress.AddCounter(wc)

	_, err = io.Copy(out, io.TeeReader(resp.Body, wc))
}

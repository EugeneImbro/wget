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
	Err        error
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += uint64(n)
	return n, nil
}

func (wc *WriteCounter) String() string {
	if wc.Err != nil {
		return wc.Err.Error()
	} else {
		return fmt.Sprintf("%d%%", wc.Downloaded*100/wc.Total)
	}
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

	p.FileNames = make([]string, 0, len(p.Counters))
	for k := range p.Counters {
		p.FileNames = append(p.FileNames, k)
	}
	sort.Strings(p.FileNames)
}

func (p *Progress) PrintProgress() {
	p.Mutex.RLock()
	defer p.Mutex.RUnlock()

	fmt.Printf("\r")
	for _, v := range p.FileNames {
		fmt.Printf("%s: %s | ", v, p.Counters[v])
	}
}

func main() {
	urls := unique(os.Args[1:])
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

func unique(s []string) []string {
	m := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))

	for _, v := range s {
		if _, ok := m[v]; !ok {
			m[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

func downloadFile(url string, progress *Progress) {
	fileName := getFileName(url)
	wc := &WriteCounter{Filename: fileName}
	progress.AddCounter(wc)
	resp, err := http.Get(url)
	if err != nil {
		wc.Err = err
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(fileName)
	if err != nil {
		wc.Err = err
		return
	}
	defer out.Close()

	wc.Total = uint64(resp.ContentLength)

	_, err = io.Copy(out, io.TeeReader(resp.Body, wc))
	wc.Err = err
}

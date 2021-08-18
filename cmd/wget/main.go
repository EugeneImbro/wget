package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CountingWriter struct {
	Filename   string
	Total      uint64
	Downloaded uint64
	Err        error
}

func (wc *CountingWriter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += uint64(n)
	return n, nil
}

func (wc *CountingWriter) String() string {
	if wc.Err != nil {
		return wc.Err.Error()
	} else {
		return fmt.Sprintf("%s %d%%", wc.Filename, wc.Downloaded*100/wc.Total)
	}
}

type Progress struct {
	mu       sync.RWMutex
	Counters []*CountingWriter
}

func (p *Progress) AddCounter(wc *CountingWriter) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Counters = append(p.Counters, wc)
}

func (p *Progress) PrintProgress() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	//todo tab writer
	fmt.Printf("\r")
	for _, v := range p.Counters {
		fmt.Printf("%s | ", v)
	}
}

func main() {
	urls := unique(os.Args[1:])
	progress := &Progress{Counters: make([]*CountingWriter, 0)}
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
	fileName := filepath.Base(url)
	wc := &CountingWriter{Filename: fileName}
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

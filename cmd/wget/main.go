package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type CountingWriter struct {
	mu         sync.RWMutex
	Filename   string
	Total      uint64
	Downloaded uint64
	Err        error
	Closer     io.Closer
}

func (cw *CountingWriter) Write(p []byte) (int, error) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	n := len(p)
	cw.Downloaded += uint64(n)
	return n, nil
}

func (cw *CountingWriter) String() string {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	if cw.Err != nil {
		return cw.Err.Error()
	} else {
		return fmt.Sprintf("%s %d%%", cw.Filename, cw.Downloaded*100/cw.Total)
	}
}

type Progress struct {
	mu       sync.RWMutex
	Counters []*CountingWriter
}

func (p *Progress) AddCounter(cw *CountingWriter) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Counters = append(p.Counters, cw)
}

func (p *Progress) PrintProgress() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	fmt.Printf("\r")
	for _, v := range p.Counters {
		fmt.Printf("%s | ", v)
	}
}

func main() {
	urls := unique(os.Args[1:])
	progress := &Progress{Counters: make([]*CountingWriter, 0)}
	wg := sync.WaitGroup{}
	ticker := time.NewTicker(500 * time.Millisecond)

	//interrupt cleanup
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		s := <-sigs
		ticker.Stop()

		fmt.Printf("\nTerminating by %s signal\n", s)
		exitCode := 0
		for _, counter := range progress.Counters {
			if counter.Err != nil || counter.Closer == nil {
				continue
			} else {
				if err := counter.Closer.Close(); err != nil {
					fmt.Printf("Cannot gracefully terminate the program. %s\n", err.Error())
					exitCode = 1
					continue
				}

				if err := os.Remove(counter.Filename); err != nil {
					fmt.Printf("Cannot gracefully terminate the program. %s\n", err.Error())
					exitCode = 1
				}
			}
		}
		fmt.Print("Terminated")
		os.Exit(exitCode)
	}()

	fmt.Println("Download started...")

	for _, arg := range urls {
		wg.Add(1)
		ch := make(chan *CountingWriter)
		go func(url string) {
			defer wg.Done()
			downloadFile(url, ch)
		}(arg)
		progress.AddCounter(<-ch)
	}

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

func downloadFile(url string, ch chan *CountingWriter) {
	fileName := filepath.Base(url)
	cw := &CountingWriter{Filename: fileName}
	ch <- cw
	resp, err := http.Get(url)
	if err != nil {
		cw.Err = err
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(fileName)
	if err != nil {
		cw.Err = err
		return
	}
	defer out.Close()
	cw.Closer = out
	cw.Total = uint64(resp.ContentLength)

	_, err = io.Copy(out, io.TeeReader(resp.Body, cw))
	cw.Err = err
}

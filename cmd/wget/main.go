package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sort"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
)

type WriterFn func(int64)

func (f WriterFn) Write(p []byte) (int, error) {
	n := len(p)
	f(int64(n))
	return n, nil
}

type ProgressStatus struct {
	Url string
	p   int
	err error
}

func (s *ProgressStatus) String() string {
	if s.err != nil {
		return s.err.Error()
	}
	return fmt.Sprintf("%d%%", s.p)
}

var writer = tabwriter.NewWriter(os.Stdout, 0, 0, 0, ' ', tabwriter.Debug)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		s := <-sigs
		fmt.Printf("\nTerminating by %s signal\n", s)
		cancel()
	}()

	fmt.Println("Download started")
	urls := os.Args[1:]
	urls = unique(urls)

	commonCh := make(chan *ProgressStatus, len(urls))

	go func() {
		progress := make(map[string]*ProgressStatus)
		t := time.NewTicker(500 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-commonCh:
				if !ok {
					printProgress(progress)
					return
				}
				progress[s.Url] = s
			case <-t.C:
				printProgress(progress)
			}
		}
	}()

	wg := &sync.WaitGroup{}
	for _, url := range urls {
		wg.Add(1)
		filePath := path.Base(url)
		p, e := download(ctx, url, filePath)
		url := url
		go func() {
			defer wg.Done()
			for {
				select {
				case v, ok := <-p:
					if !ok {
						return
					}
					commonCh <- &ProgressStatus{Url: url, p: v}
				case err, ok := <-e:
					if !ok {
						return
					}
					commonCh <- &ProgressStatus{Url: url, err: err}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(commonCh)
}

func unique(s []string) []string {
	var result []string
	keys := make(map[string]bool)
	for _, entry := range s {
		if _, ok := keys[entry]; !ok {
			keys[entry] = true
			result = append(result, entry)
		}
	}
	return result
}

func printProgress(p map[string]*ProgressStatus) {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println()
	for _, url := range keys {
		_, ok := p[url]
		if ok {
			fmt.Fprintln(writer, fmt.Sprintf("%s \t%s", url, p[url]))
		}
	}
	writer.Flush()
}

func download(ctx context.Context, url string, filePath string) (progress chan int, errors chan error) {
	progress = make(chan int, 1)
	errors = make(chan error, 1)

	go func() {
		defer close(progress)
		defer close(errors)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			errors <- err
			return
		}

		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			errors <- err
			return
		}
		defer resp.Body.Close()

		out, err := os.Create(filePath)
		if err != nil {
			errors <- err
			return
		}
		defer out.Close()

		_, err = io.Copy(out, io.TeeReader(
			resp.Body,
			func(total int64, ch chan int) WriterFn {
				downloaded := int64(0)
				return func(p int64) {
					downloaded += p
					ch <- int(downloaded * 100 / total)
				}
			}(resp.ContentLength, progress),
		))

		if err != nil {
			errors <- err
			defer os.Remove(filePath)
		}
	}()
	return
}

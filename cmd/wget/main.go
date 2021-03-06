package main

import (
	"context"
	"fmt"
	"github.com/EugeneImbro/go-wget/pkg/download"
	"os"
	"os/signal"
	"path"
	"sort"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
)

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

	urls := os.Args[1:]
	urls = unique(urls)

	//channel for progress from all download routines
	commonCh := make(chan *ProgressStatus, len(urls))
	//group to handle all downloads are done case
	wg := &sync.WaitGroup{}

	fmt.Println("Download started")

	for _, url := range urls {
		wg.Add(1)
		filePath := path.Base(url)
		p, e := download.DownloadFile(ctx, url, filePath)
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

	//wait for all downloads are done and close progress channel
	go func() {
		wg.Wait()
		close(commonCh)
	}()

	progress := make(map[string]*ProgressStatus)
	t := time.NewTicker(500 * time.Millisecond)
	//handle progress + modify map with current state and print by tick until progress chan is not closed.
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-commonCh:
			if !ok {
				printProgress(progress)
				fmt.Println("\nDownload finished.")
				return
			}
			progress[s.Url] = s
		case <-t.C:
			printProgress(progress)
		}
	}

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

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
)

type WriterFn func(int64)

func (f WriterFn) Write(p []byte) (int, error) {
	n := len(p)
	f(int64(n))
	return n, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		s := <-sigs
		fmt.Printf("\nTerminating by %s signal\n", s)
		_, cancel := context.WithCancel(ctx)
		cancel()
		os.Exit(0)
	}()

	url := os.Args[1]
	filePath := path.Base(url)

	fmt.Printf("Download: %s\n", url)
	p, e := download(ctx, url, filePath)
	for {
		select {
		case v, ok := <-p:
			if !ok {
				return
			}
			fmt.Printf("\r%s: %d%%", filePath, v)
		case err := <-e:
			fmt.Printf("%s", err.Error())
			return
		}
	}
}

func download(ctx context.Context, url string, filePath string) (progress chan int, errors chan error) {
	progress = make(chan int)
	errors = make(chan error)

	go func() {
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
			_ = os.Remove(filePath)
			errors <- err
		}
		close(progress)
	}()

	return
}

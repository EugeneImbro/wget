package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"
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
		//вот это по идее не правильно т.к. фактически вызывается не там, где создается.
		cancel()
	}()

	fmt.Println("Download started")
	urls := os.Args[1:]

	//хуй знает, как сделать периодический вывод без подобной структуры
	//и да, это должно быть thread safe над слайсом.
	progress := make([]string, len(urls))

	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				printProgress(progress)
			}
		}
	}()

	wg := &sync.WaitGroup{}
	for i, url := range urls {
		wg.Add(1)
		filePath := path.Base(url)
		p, e := download(ctx, url, filePath)
		progressIndex := i
		go func() {
			for {
				select {
				case v, ok := <-p:
					if !ok {
						wg.Done()
						return
					}
					progress[progressIndex] = fmt.Sprintf("%s %d%%", filePath, v)
				case err := <-e:
					progress[progressIndex] = fmt.Sprintf("%s", err.Error())
					wg.Done()
					return
				}
			}
		}()
	}
	wg.Wait()
	//такое себе, но выводит конечный стейт.
	printProgress(progress)

}

func printProgress(p []string) {
	fmt.Printf("\r")
	for _, v := range p {
		fmt.Printf("%s | ", v)
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
			defer os.Remove(filePath)
			errors <- err
		}
		close(progress)
	}()
	return
}

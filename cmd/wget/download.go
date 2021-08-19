package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

type FuncWriter func(int64)

func (f FuncWriter) Write(p []byte) (int, error) {
	n := len(p)
	f(int64(n))
	return n, nil
}

func main() {
	url := os.Args[1]
	p, e := download(url)
	fmt.Printf("Download: %s\n", url)
	for {
		select {
		case v, ok := <-p:
			if !ok {
				return
			}
			fmt.Printf("\r%d %%", v)
		case err := <-e:
			fmt.Printf("%s", err.Error())
			return
		}
	}
}

func download(url string) (progress chan int, errors chan error) {
	progress = make(chan int)
	errors = make(chan error)

	go func() {
		fileName := path.Base(url)
		resp, err := http.Get(url)
		if err != nil {
			errors <- err
			return
		}
		defer resp.Body.Close()

		out, err := os.Create(fileName)
		if err != nil {
			errors <- err
		}
		defer out.Close()

		_, err = io.Copy(out, io.TeeReader(
			resp.Body,
			func(total int64, ch chan int) FuncWriter {
				downloaded := int64(0)
				return func(p int64) {
					downloaded += p
					ch <- int(downloaded * 100 / total)
				}
			}(resp.ContentLength, progress),
		))

		if err != nil {
			errors <- err
		}
		close(progress)
	}()

	return
}

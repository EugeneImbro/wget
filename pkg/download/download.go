package download

import (
	"context"
	"io"
	"net/http"
	"os"
)

type WriterFn func(int64)

func (f WriterFn) Write(p []byte) (int, error) {
	n := len(p)
	f(int64(n))
	return n, nil
}

func DownloadFile(ctx context.Context, url string, filePath string) (progress chan int, errors chan error) {
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

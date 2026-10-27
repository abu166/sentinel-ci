package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trace-health URL")
		os.Exit(2)
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(os.Args[1])
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 128<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		os.Exit(1)
	}
}

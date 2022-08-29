package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	i := 0
	for {
		println("Making request")
		makeRequest()
		fmt.Printf("[%d] Request complete\n", i)
		time.Sleep(1 * time.Second)

		i++

	}
}

func makeRequest() {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctxTimeout, http.MethodGet, "http://http-server-test-internal:8111", nil)
	if err != nil {
		println(err.Error())
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		println(err.Error())
		return
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		println(err.Error())
	}
	println(string(data))
}

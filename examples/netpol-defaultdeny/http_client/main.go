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

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.ForceAttemptHTTP2 = false

	client := &http.Client{Transport: transport}

	for {
		println("Making request")
		makeRequest(client)
		fmt.Printf("[%d] Request complete\n", i)
		time.Sleep(1 * time.Second)

		i++

	}
}

func makeRequest(client *http.Client) {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctxTimeout, http.MethodGet, "http://http-server-test-internal:8111", nil)
	if err != nil {
		println(err.Error())
		return
	}

	resp, err := client.Do(req)
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

package main

import (
	"fmt"
	"net/http"
)

func main() {
	err := http.ListenAndServe(":8111", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("test"))
		fmt.Printf("Got request from %s\n", request.RemoteAddr)
	}))
	if err != nil {
		panic(err)
	}
}

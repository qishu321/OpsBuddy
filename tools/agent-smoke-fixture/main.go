package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"opsbuddy/internal/agentfixture"
	"os"
)

func main() {
	file := flag.String("url-file", "", "write local fixture URL to this file")
	flag.Parse()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	url := "http://" + listener.Addr().String()
	if *file != "" {
		if err = os.WriteFile(*file, []byte(url), 0600); err != nil {
			panic(err)
		}
	}
	fmt.Println(url)
	if err = http.Serve(listener, &agentfixture.Server{}); err != nil {
		panic(err)
	}
}

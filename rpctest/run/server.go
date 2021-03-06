package main

import (
	"gotest/rpctest"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"runtime"
	"time"
)

func main() {
	cpus := runtime.NumCPU()
	log.Println("CUPS:", cpus)
	runtime.GOMAXPROCS(cpus)
	u := new(rpctest.User)
	rpc.Register(u)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", ":1314")
	if e != nil {
		log.Fatal("listen error:", e)
	}
	defer l.Close()

	go http.Serve(l, nil)

	time.Sleep(10 * time.Minute)

}

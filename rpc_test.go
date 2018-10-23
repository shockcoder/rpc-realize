package labrpc

import (
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

type JunkArgs struct {
	X int
}

type JunkReply struct {
	X string
}

type JunkServer struct {
	mu   sync.Mutex
	log1 []string
	log2 []int
}

func (js *JunkServer) Handler1(args string, reply *int) {
	js.mu.Lock()
	defer js.mu.Unlock()

	js.log1 = append(js.log1, args)
	*reply, _ = strconv.Atoi(args)
}

func (js *JunkServer) Handler2(args int, reply *string) {
	js.mu.Lock()
	defer js.mu.Unlock()

	js.log2 = append(js.log2, args)
	*reply = "handler2-" + strconv.Itoa(args)
}

func (js *JunkServer) Handler3(args int, reply *int) {
	js.mu.Lock()
	defer js.mu.Unlock()
	time.Sleep(20 * time.Second)
	*reply = -args
}

// args is a pointer
func (js *JunkServer) Handler4(args *JunkArgs, reply *JunkReply) {
	reply.X = "pointer"
}

// args is a not pointer
func (js *JunkServer) Handler5(args JunkArgs, reply *JunkReply) {
	reply.X = "no pointer"
}

func TestBasic(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()

	e := rn.MakeEnd("end1-99")

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer("server99", rs)

	rn.Connect("end1-99", "server99")
	rn.Enable("end1-99", true)

	{
		reply := ""
		e.Call("JunkServer.Handler2", 111, &reply)
		if reply != "handler2-111" {
			t.Fatalf("wrong reply from Hander2")
		}
	}

	{
		reply := 0
		e.Call("JunkServer.Handler1", "9090", &reply)
		if reply != 9090 {
			t.Fatalf("wrong reply from Hander1")
		}
	}
}

func TestTypes(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()

	e := rn.MakeEnd("end1-99")

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer("server99", rs)

	rn.Connect("end1-99", "server99")
	rn.Enable("end1-99", true)

	{
		var args JunkArgs
		var reply JunkReply
		// args must match type (pointer or not) of handler.
		e.Call("JunkServer.Handler4", &args, &reply)
		if reply.X != "pointer" {
			t.Fatalf("wrong reply from Handler4")
		}
	}

	{
		var args JunkArgs
		var reply JunkReply
		// args must match type (pointer or not) of handler.
		e.Call("JunkServer.Handler5", args, &reply)
		if reply.X != "no pointer" {
			t.Fatalf("wrong reply from Handler5")
		}
	}
}

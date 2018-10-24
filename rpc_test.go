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

// rn.Enable(endname, false) really disconnect a client
func TestDisconnect(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()

	e := rn.MakeEnd("end1-99")

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer("server99", rs)

	rn.Connect("end1-99", "server99")
	{
		reply := ""
		e.Call("JunkServer.Handler2", 111, &reply)
		if reply != "" {
			t.Fatalf("unexpected reply from Handler2")
		}
	}

	rn.Enable("end1-99", true)

	{
		reply := 0
		e.Call("JunkServer.Handler1", "9099", &reply)
		if reply != 9099 {
			t.Fatalf("wrong reply from Handler1")
		}
	}
}

// test net.GetCount()
func TestCount(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()

	e := rn.MakeEnd("end1-99")

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer(99, rs)

	rn.Connect("end1-99", 99)
	rn.Enable("end1-99", true)

	for i := 0; i < 17; i++ {
		reply := ""
		e.Call("JunkServer.Handler2", i, &reply)
		wanted := "handler2-" + strconv.Itoa(i)
		if wanted != reply {
			t.Fatalf("wrong reply %v from Handler1, expecting %v", reply, wanted)
		}
	}

	n := rn.GetCount(99)
	if n != 17 {
		t.Fatalf("wrong GetCount() %v, expected 17\n", n)
	}
}

// test RPCs from concurrent ClientEnds
func TestConcurrentMany(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer(1000, rs)

	ch := make(chan int)

	nclients := 20
	nrpcs := 10
	for i := 0; i < nclients; i++ {
		go func(i int) {
			n := 0
			defer func() { ch <- n }()

			e := rn.MakeEnd(i)
			rn.Connect(i, 1000)
			rn.Enable(i, true)

			for j := 0; j < nrpcs; j++ {
				arg := i*100 + j
				reply := ""
				e.Call("JunkServer.Handler2", arg, &reply)
				wanted := "handler2-" + strconv.Itoa(arg)
				if reply != wanted {
					t.Fatalf("wrong reply %v from Handler2, expecting %v", reply, wanted)
				}
				n += 1
			}
		}(i)
	}

	total := 0
	for i := 0; i < nclients; i++ {
		x := <-ch
		total += x
	}

	if total != nclients*nrpcs {
		t.Fatalf("wrong number of RPCs completed, got %v, expected %v", total, nclients*nrpcs)
	}

	n := rn.GetCount(1000)
	if n != total {
		t.Fatalf("wrong GetCount() %v, expected %v\n", n, total)
	}
}

// test unreliable
func TestUnreliable(t *testing.T) {
	runtime.GOMAXPROCS(4)

	rn := MakeNetWork()
	rn.Reliable(false)

	js := &JunkServer{}
	svc := MakeService(js)

	rs := MakeServer()
	rs.AddService(svc)
	rn.AddServer(1000, rs)

	ch := make(chan int)

	nclients := 300
	for i := 0; i < nclients; i++ {
		go func(i int) {
			n := 0
			defer func() { ch <- n }()

			e := rn.MakeEnd(i)
			rn.Connect(i, 1000)
			rn.Enable(i, true)

			arg := i * 100
			reply := ""
			ok := e.Call("JunkServer.Handler2", arg, &reply)
			if ok {
				wanted := "handler2-" + strconv.Itoa(arg)
				if reply != wanted {
					t.Fatalf("wrong reply %v from Handler2, expecting %v", reply, wanted)
				}
				n += 1
			}
		}(i)
	}

	total := 0
	for i := 0; i < nclients; i++ {
		x := <-ch
		total += x
	}

	if total == nclients || total == 0 {
		t.Fatalf("all RPCs succeeded despite unreliable")
	}
}

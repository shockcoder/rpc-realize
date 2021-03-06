package labrpc

import (
	"bytes"
	"encoding/gob"
	"log"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"time"
)

// channel-based RPC, for 824 labs

// 自定义实现的 labrpc.go 来为代码进行阶段性测试
// 改编自 Go net/rpc/server.go

type reqMsg struct {
	endname  interface{}   // 请求的客户端名字
	svcMeth  string        // 方法 e.g. "Raft.AppendEntries" 通过反射去运行指定的方法
	argsType reflect.Type  //参数类型反射
	args     []byte        //序列化参数
	replyCh  chan replyMsg //client、server 通信channel
}

type replyMsg struct {
	ok    bool   //success or false
	reply []byte //result data serialize
}

type ClientEnd struct {
	endname interface{} //客户端的名字
	ch      chan reqMsg
}

// 发送 rpc请求，等待回复
// 返回值意味着成功，失败则表示 服务不可连接
func (e *ClientEnd) Call(svcMeth string, args interface{}, reply interface{}) bool {
	//序列化请求参数args
	qb := new(bytes.Buffer)
	encoder := gob.NewEncoder(qb)
	encoder.Encode(args)

	replyCh := make(chan replyMsg) //客户端、服务端通过该通道进行信息的交互

	req := reqMsg{
		endname:  e.endname,
		svcMeth:  svcMeth,
		argsType: reflect.TypeOf(args),
		args:     qb.Bytes(),
		replyCh:  replyCh, //该channel用于clent、server 通信
	}

	e.ch <- req //往channel中写入请求信息

	resp := <-req.replyCh //通过channel用于接收server返回的信息
	if resp.ok {
		rb := bytes.NewBuffer(resp.reply) //反序列化获取返回信息
		decoder := gob.NewDecoder(rb)
		if err := decoder.Decode(reply); err != nil {
			log.Fatalf("ClientEnd.Call(): decode reply : %v\n", err)
		}
		return true
	}
	return false
}

type Network struct {
	mu              sync.Mutex
	reliable        bool
	longDelays      bool                        //连接不可达时,停顿一段时间
	longRecordering bool                        //延迟回复的时间
	ends            map[interface{}]*ClientEnd  //客户端的 map集合 key: name of ClientEnd
	enabled         map[interface{}]bool        //by end name
	servers         map[interface{}]*Server     //服务器, by name
	connections     map[interface{}]interface{} //客户端 -> 服务端
	endCh           chan reqMsg
}

// 模拟一个网络
// 该网络包含客户端和服务端
func MakeNetWork() *Network {

	endCh := make(chan reqMsg)

	rn := &Network{
		reliable:    true,
		ends:        map[interface{}]*ClientEnd{},
		enabled:     map[interface{}]bool{},
		servers:     map[interface{}]*Server{},
		connections: map[interface{}]interface{}{},
		endCh:       endCh,
	}

	//开启一个goroutine 来处理所有的客户端的请求(Client.Call())
	go func() {
		for xreq := range rn.endCh {
			go rn.ProcessReq(xreq)
		}
	}()

	return rn
}

func (rn *Network) Reliable(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.reliable = yes
}

func (rn *Network) LongRecording(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longRecordering = yes
}

func (rn *Network) LongDelays(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longDelays = yes
}

func (rn *Network) ReadEndnameInfo(endname interface{}) (enabled bool, servername interface{},
	server *Server, reliable bool, longreordering bool) {

	rn.mu.Lock()
	defer rn.mu.Unlock()

	enabled = rn.enabled[endname]
	servername = rn.connections[endname]
	if servername != nil {
		server = rn.servers[servername]
	}
	reliable = rn.reliable
	longreordering = rn.longRecordering
	return
}

func (rn *Network) IsServerDead(endname interface{}, servername interface{}, server *Server) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.enabled[endname] == false || rn.servers[servername] != server {
		return true
	}

	return false
}

func (rn *Network) ProcessReq(req reqMsg) {
	enabled, servername, server, reliable, longrecordering := rn.ReadEndnameInfo(req.endname)

	if enabled && servername != nil && server != nil {
		if reliable == false {
			// 短暂的延迟, 等待响应
			ms := rand.Int() % 27
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}

		if reliable == false && rand.Int()%1000 < 100 {
			req.replyCh <- replyMsg{false, nil} // 如果超时，删除这个请求并返回 空的replyMsg
			return
		}

		// 响应客户端发来的请求(call the RPC handler) 开启一个协程去处理
		// 当服务不可用，  RPC请求 应该得到一个请求失败的reply
		ech := make(chan replyMsg)
		go func() {
			r := server.dispatch(req)
			ech <- r
		}()

		// 等待处理器的返回
		// 当DeleteServer()函数被执行时， 停止等待并返回一个错误
		var reply replyMsg
		replyOK := false
		serverDead := false
		for replyOK == false && serverDead == false {
			select {
			case reply = <-ech:
				replyOK = true
			case <-time.After(100 * time.Millisecond):
				serverDead = rn.IsServerDead(req.endname, servername, server)
			}
		}

		// 当DeleteServer()被执行，即服务器被杀死, 不用回复客户端请求
		// 这是为了避免客户端对Append的肯定回复的情况
		// 但是服务器将更新持久保存的保存到旧的Persister中.在执行DeleteServer()之前请慎重考虑
		serverDead = rn.IsServerDead(req.endname, servername, server)

		if replyOK == false || serverDead == true {
			// server was killed while we were waiting
			req.replyCh <- replyMsg{false, nil}
		} else if reliable == false && (rand.Int()%1000 < 100) {
			// 响应超时，放弃回复
			req.replyCh <- replyMsg{false, nil}
		} else if longrecordering == true && rand.Intn(900) < 600 {
			// 延长一点响应时间
			ms := 200 + rand.Intn(1+rand.Intn(2000))
			time.Sleep(time.Duration(ms) * time.Millisecond)
			req.replyCh <- reply
		} else {
			req.replyCh <- reply
		}
	} else {
		// 模拟没有回复 和 超时
		ms := 0
		if rn.longDelays {
			ms = rand.Int() % 7000
		} else {
			//模拟请求快速响应
			ms = rand.Int() % 100
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		req.replyCh <- replyMsg{false, nil}
	}
}

// 为该网络创建一个客户端
func (rn *Network) MakeEnd(endname interface{}) *ClientEnd {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if _, ok := rn.ends[endname]; ok {
		log.Fatalf("MakeEnd: %v already exists\n", endname)
	}

	e := &ClientEnd{
		endname: endname,
		ch:      rn.endCh,
	}
	rn.ends[endname] = e
	rn.enabled[endname] = false
	rn.connections[endname] = nil

	return e
}

func (rn *Network) AddServer(servername interface{}, rs *Server) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = rs
}

func (rn *Network) DeleteServer(servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = nil
}

// 将一个客户端连接到 server
// 在客户端的生命周期内 只能连接一次
func (rn *Network) Connect(endname interface{}, servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.connections[endname] = servername
}

// enable/disable a ClientEnd.
func (rn *Network) Enable(endname interface{}, enabled bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.enabled[endname] = enabled
}

// 获取连接server的rpcs 的数量
func (rn *Network) GetCount(servername interface{}) int {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	svr := rn.servers[servername]
	return svr.GetCount()
}

// server 是一个servers的组成，所有的server拥有同样的 rpc适配器
// 因此 例如  Raft和 k/v server 都可以监听相同的rpc 客户端
type Server struct {
	mu       sync.Mutex
	services map[string]*Service
	count    int //连接的 RPCs
}

func MakeServer() *Server {
	rs := &Server{}
	rs.services = map[string]*Service{}
	return rs
}

func (rs *Server) AddService(svc *Service) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.services[svc.name] = svc
}

func (rs *Server) GetCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.count
}

func (rs *Server) dispatch(req reqMsg) replyMsg {
	rs.mu.Lock()

	rs.count += 1

	// 将 Raft.AppendEntries 分离 到 服务和 方法中
	dot := strings.LastIndex(req.svcMeth, ".")
	serviceName := req.svcMeth[:dot]
	methodName := req.svcMeth[dot+1:]

	service, ok := rs.services[serviceName]
	rs.mu.Unlock()

	if ok {
		return service.dispatch(methodName, req)
	}

	//没有找到相对应的service
	choices := []string{}
	for k, _ := range rs.services {
		choices = append(choices, k)
	}
	log.Fatalf("labrpc.Server.dispatch(): unknown service %v in %v.%v; expecting one of %v\n",
		serviceName, serviceName, methodName, choices)
	return replyMsg{false, nil}

}

// 用于反射整个service
type Service struct {
	name    string                    // service name
	rcvr    reflect.Value             // Value类型
	typ     reflect.Type              // Type类型
	methods map[string]reflect.Method // 函数类型
}

// MakeService 通过反射获取传入的 rcvr的字段、方法
func MakeService(rcvr interface{}) *Service {
	svc := &Service{}
	svc.typ = reflect.TypeOf(rcvr)                      // reflect.Type
	svc.rcvr = reflect.ValueOf(rcvr)                    // reflect.Value
	svc.name = reflect.Indirect(svc.rcvr).Type().Name() // 返回svc.rcvr持有的指向的值 的Value的类型名
	svc.methods = map[string]reflect.Method{}

	for m := 0; m < svc.typ.NumMethod(); m++ { // NumMethod() 返回该类型的方法的数目
		method := svc.typ.Method(m) // 返回第m 个方法
		mtype := method.Type        // 方法类型
		mname := method.Name        // 方法名

		// PlgPath 类型的包路径 NumIn 返回func类型的参数个数 In(i)返回func类型的第i个参数的类型(Type) NumOut() 返回func类型的返回值个数
		if method.PkgPath != "" || mtype.NumIn() != 3 || mtype.In(2).Kind() != reflect.Ptr || mtype.NumOut() != 0 {
			// bad method  not for a handler
		} else {
			svc.methods[mname] = method
		}
	}

	return svc
}

// dispatch 通过反射执行Call("method", arg, &reply) 传入的方法
func (svc *Service) dispatch(methname string, req reqMsg) replyMsg {
	if method, ok := svc.methods[methname]; ok {
		// 读取参数
		// type 是一个 req.argsType的指针
		args := reflect.New(req.argsType)

		// 对参数进行反序列化
		buff := bytes.NewBuffer(req.args)
		decoder := gob.NewDecoder(buff)
		decoder.Decode(args.Interface())

		//为reply申请内存空间
		replyType := method.Type.In(2)   // 返回该method 的第2个参数的类型 Type
		replyType = replyType.Elem()     // 返回该Type的具体元素类型
		replyv := reflect.New(replyType) // 返回Value类型值，该值持有指向replyType的新申请的指针

		// 执行函数
		function := method.Func
		function.Call([]reflect.Value{svc.rcvr, args.Elem(), replyv}) // Call([]Value) 反射执行函数

		// 对reply进行反序列
		buf := new(bytes.Buffer)
		encoder := gob.NewEncoder(buf)
		encoder.EncodeValue(replyv)

		return replyMsg{true, buf.Bytes()}
	}

	//没有找到相对应的方法
	choices := []string{}
	for k, _ := range svc.methods {
		choices = append(choices, k)
	}
	log.Fatalf("labrpc.Service.dispatch(): unknown method %v in %v; expecting one of %v\n",
		methname, req.svcMeth, choices)
	return replyMsg{false, nil}
}

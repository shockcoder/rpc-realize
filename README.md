# 基于 MIT-6.824 的自定义RPC server

- net := MakeNetWork() -- 模拟网络环境，包括clients, servers
- end := MakeEnd(endname) -- 创建一个client, 与 server交互
- net.AddServer(servername, server) -- 向网络中添加一个server
- net.DeleteServer(servername) -- 网络中移除一个server
- net.Connect(endname, servername) -- 连接 一个client and server
- net.Enable(endname, enabled) -- enable/disable a client
- net.Reliable(bool) -- false 意味着 消息不可达或者有延迟

end.Call("Entries.DoMethod", args, &reply) -- send an RPC, wait for reply
Entries 是实体的名字 比如：<br>
 var MyType int  <br>
 type Task struct {} <br>
 MyType 、 Task 都是Entries , DoMethod 则是其方法, args 为参数，reply为返回值
 
 # RPC实现核心 -- 反射
 - MakeService(interface{})  -- 通过反射获取Server的方法、字段
 ````
 // MakeService 通过反射获取传入的 rcvr的字段、方法
 func MakeService(rcvr interface{}) *Service {
 	svc := &Service{}
 	svc.typ = reflect.TypeOf(rcvr)
 	svc.rcvr = reflect.ValueOf(rcvr)
 	svc.name = reflect.Indirect(svc.rcvr).Type().Name()
 	svc.methods = map[string]reflect.Method{}
 
 	for m := 0; m < svc.typ.NumMethod(); m++ {
 		method := svc.typ.Method(m)
 		mtype := method.Type
 		mname := method.Name
 
 		if method.PkgPath != "" || mtype.NumIn() != 3 || mtype.In(2).Kind() != reflect.Ptr || mtype.NumOut() != 0 {
 			// bad method  not for a handler
 		} else {
 			svc.methods[mname] = method
 		}
 	}
 
 	return svc
 }
 ````
 - service.dispatch() -- run by reflect
 
````
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
		replyType := method.Type.In(2)
		replyType = replyType.Elem()
		replyv := reflect.New(replyType)

		// 执行函数
		function := method.Func
		function.Call([]reflect.Value{svc.rcvr, args.Elem(), replyv})

		// 对reply进行编码
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
````

`这两个方法是整个RPC server的重点，通过反射完成client 和server的交互，server的结果通过 reply这个参数直接返回给client`
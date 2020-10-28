package main

import (
	"context"
	proxy2 "golang.org/x/net/proxy"
	"io/ioutil"
	"log"
	"net/http"
	"socksmitm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

}
func main() {
	var addr = "0.0.0.0:5678"
	pkcs12Data,err:=ioutil.ReadFile("charles-ssl-proxying.p12")
	if err != nil{
		log.Printf("%+v\n",err)
		return
	}
	var dialer = proxy2.FromEnvironment()
	//p,err:= url.Parse("socks5://localhost:1080")
	//if err != nil{
	//	log.Printf("%+v\n",err)
	//	return
	//}
	//dialer,err= proxy2.FromURL(p,proxy2.FromEnvironment())
	//if err != nil{
	//	log.Printf("%+v\n",err)
	//	return
	//}
	mux := socksmitm.NewMux(dialer)
	server,err := socksmitm.NewSocks5Server(mux, pkcs12Data, "DwCpsCLsZc7c")
	if err != nil{
	    log.Printf("%+v\n",err)
	    return
	}
	server.RegisterRootCa()// 注册 root.ca 处理器, 用于浏览器获取ca证书
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	err = server.Run(ctx, addr)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
}

func handler(writer http.ResponseWriter, request *http.Request) {
	log.Println(request.URL.String())
	writer.Write([]byte("mitm ok"))
	writer.WriteHeader(200)
}

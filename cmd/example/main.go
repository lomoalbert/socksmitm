package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/lomoalbert/socksmitm"
	proxy2 "golang.org/x/net/proxy"
	"golang.org/x/xerrors"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

}

var pacPort = 4567
var socksPort = 5678

func main() {
	err := socksmitm.PacListenAndServe(pacPort, socksPort)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	pkcs12Data, err := ioutil.ReadFile("charles-ssl-proxying.p12")
	if err != nil {
		log.Printf("%+v\n", err)
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
	mux.SetDefaultHTTPRoundTrip(socksmitm.BlockRoundTrip)
	mux.Register("ip.fm", ChangeRespRoutdTrip)
	//mux.Register("genresp.test",TestComRoutdTrip)
	server, err := socksmitm.NewSocks5Server(mux, pkcs12Data, "DwCpsCLsZc7c")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	server.RegisterRootCa() // 注册 root.ca 处理器, 用于浏览器获取ca证书
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	err = server.Run(ctx, fmt.Sprintf("0.0.0.0:%d", socksPort))
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
}

func ChangeRespRoutdTrip(req *http.Request) (*http.Response, error) {
	log.Println("req:", req.Method, req.Proto, req.URL.Scheme, req.Host, req.URL.Path)
	resp, err := socksmitm.NormalRoundTrip(req)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	orgBody := resp.Body
	defer orgBody.Close()
	_, err = ioutil.ReadAll(orgBody)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	log.Println("orgHeader:", resp.Header, resp.ContentLength)
	newData := []byte("mitm works!")
	if resp.Header.Get("Content-Encoding") == "gzip" {
		newReader, err := gzip.NewReader(bytes.NewReader(newData))
		if err != nil {
			resp.Header.Del("Content-Encoding")
		} else {
			buffer := []byte{}
			length, err := newReader.Read(buffer)
			if err != nil {
				resp.Header.Del("Content-Encoding")
			} else {
				newData = buffer[:length]
			}
		}
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(newData))
	resp.Header.Set("Content-Length", strconv.Itoa(len(newData)))
	resp.ContentLength = int64(len(newData))
	log.Println("orgHeader:", resp.Header, resp.Header.Get("Content-Length"))
	return resp, nil
}

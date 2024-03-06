package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/lomoalbert/socksmitm"
	proxy2 "golang.org/x/net/proxy"
	"golang.org/x/xerrors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

}

var pacPort = 4567
var socksPort = 5678

func main() {
	err := socksmitm.PacListenAndServe(context.TODO(), pacPort, socksPort)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	pkcs12Data, err := os.ReadFile("charles-ssl-proxying.p12")
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
	mux.SetDefaultHTTPRoundTrip(socksmitm.NormalRoundTrip)
	mux.Register("abc.com", ChangeReqRoundTrip)
	mux.Register("def.com", ChangeRespRoundTrip)
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

func ChangeReqRoundTrip(req *http.Request) (*http.Response, error) {
	log.Println("req:", req.Method, req.Proto, req.URL.Scheme, req.Host, req.URL.Path)
	if req.URL.Path != "/api/student" {
		return socksmitm.NormalRoundTrip(req)
	}
	if req.Body == nil {
		return nil, xerrors.New("not found req body")
	}
	err := req.ParseForm()
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	log.Println("Content-Length:", req.Header.Get("Content-Length"))
	formItem := req.Form.Get("key")
	formItem = "value"
	req.Form.Set("key", formItem)
	body := req.Form.Encode()
	newReq, err := http.NewRequest(req.Method, fmt.Sprintf("%s://%s%s", req.URL.Scheme, req.Host, req.URL.Path), strings.NewReader(body))
	newReq.Header = req.Header
	newReq.Header.Set("Content-Length", strconv.Itoa(len([]byte(body))))
	log.Println("Content-Length:", newReq.Header.Get("Content-Length"))
	resp, err := socksmitm.NormalRoundTrip(newReq)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	return resp, nil
}

func ChangeRespRoundTrip(req *http.Request) (*http.Response, error) {
	log.Println("req:", req.Method, req.Proto, req.URL.Scheme, req.Host, req.URL.Path)
	resp, err := socksmitm.NormalRoundTrip(req)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	orgBody := resp.Body
	defer orgBody.Close()
	_, err = io.ReadAll(orgBody)
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
	resp.Body = io.NopCloser(bytes.NewReader(newData))
	resp.Header.Set("Content-Length", strconv.Itoa(len(newData)))
	resp.ContentLength = int64(len(newData))
	log.Println("orgHeader:", resp.Header, resp.Header.Get("Content-Length"))
	return resp, nil
}

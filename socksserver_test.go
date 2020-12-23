package socksmitm_test

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lomoalbert/socksmitm"
	"golang.org/x/net/http2"
	proxy2 "golang.org/x/net/proxy"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestNewSocks5Server(t *testing.T) {
	var addr = "127.0.0.1:5678"
	pkcs12Data, err := ioutil.ReadFile("charles-ssl-proxying.p12")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	mux := socksmitm.NewMux(proxy2.FromEnvironment())
	server, err := socksmitm.NewSocks5Server(mux, pkcs12Data, "DwCpsCLsZc7c")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	server.RegisterRootCa()
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		err := server.Run(ctx, addr)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}()
	time.Sleep(time.Second)
	proxy, err := url.Parse("socks5://" + addr)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	tr := &http.Transport{
		Proxy: http.ProxyURL(proxy),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	http2.ConfigureTransport(tr)
	client := &http.Client{
		Transport:     tr,
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       time.Second * 10,
	}
	resp, err := client.Get("https://www.baidu.com")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	log.Println(string(data))
}

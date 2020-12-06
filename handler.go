package socksmitm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"log"
	"net"
	"net/http"
)

type Mux struct {
	DefaultHandler HandlerFunc
	Handler        HostHandler
}

type HostHandler map[string]HandlerFunc

type HandlerFunc func(conn net.Conn, isTls bool, host string, port int)

func NewMux(DefaultDialer proxy.Dialer) *Mux {
	return &Mux{DefaultHandler: NewDefaultHandlerFunc(DefaultDialer), Handler: make(HostHandler)}
}

func (mux *Mux) SetDefaultHandlerFunc(handler HandlerFunc) {
	mux.DefaultHandler = handler
}

func (mux *Mux) Register(host string, handler HandlerFunc) {
	mux.Handler[host] = handler
}

func (mux *Mux) Handle(conn net.Conn, isTls bool, host string, port int) {
	handler, ok := mux.Handler[host]
	if !ok {
		mux.DefaultHandler(conn, isTls, host, port)
		return
	}
	handler(conn, isTls, host, port)
}

func BlockHandlerFunc(conn net.Conn, isTls bool, host string, port int) {
	//log.Println("block request to:", host)
	return
}

func NewDefaultHandlerFunc(dialer proxy.Dialer) HandlerFunc {
	return func(clientConn net.Conn, isTls bool, host string, port int) {
		//log.Println("req:", isTls, host, port)
		serverConn, err := dialer.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		if isTls {
			serverConn = tls.Client(serverConn, &tls.Config{
				InsecureSkipVerify: true,
			})
		}
		defer serverConn.Close()
		defer clientConn.Close()
		reqBuff := bufio.NewReader(clientConn)
		respBuff := bufio.NewReader(serverConn)
		for {
			reqCopyBuff := bytes.NewBuffer(nil)
			clientReq, err := http.ReadRequest(bufio.NewReader(io.TeeReader(reqBuff, reqCopyBuff)))
			if err != nil {
				//log.Printf("%+v\n", err)
				// todo: tcp connect handler
				return
			}
			err = clientReq.Write(serverConn)
			if err != nil {
				//log.Printf("%+v\n", err)
				return
			}
			respCopyBuff := bytes.NewBuffer(nil)
			serverResp, err := http.ReadResponse(bufio.NewReader(io.TeeReader(respBuff, respCopyBuff)), clientReq)
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			err = serverResp.Write(clientConn)
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			if serverResp.StatusCode == http.StatusUpgradeRequired {
				break
			}

			//clientCopyReq, err := http.ReadRequest(bufio.NewReader(reqCopyBuff))
			//if err != nil {
			//	log.Printf("%+v\n", err)
			//	return
			//}
			//serverCopyResp, err := http.ReadResponse(bufio.NewReader(respCopyBuff), clientCopyReq)
			//if err != nil {
			//	log.Printf("%+v\n", err)
			//	return
			//}
			//log.Printf("copyreq: %#v\n", clientCopyReq)
			//log.Printf("copyresp: %#v\n", serverCopyResp)
		}
		//websocket
		go func() {
			_, err := io.Copy(serverConn, reqBuff)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			//log.Println(n)
		}()
		go func() {
			_, err := io.Copy(clientConn, respBuff)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			//log.Println(n)
		}()
	}
}

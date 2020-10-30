package socksmitm

import (
	"bufio"
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
	log.Println("block request to:", host)
	return
}

func NewDefaultHandlerFunc(dialer proxy.Dialer) HandlerFunc {
	return func(clientConn net.Conn, isTls bool, host string, port int) {
		log.Println("req:", isTls, host, port)
		serverConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
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
			clientReq, err := http.ReadRequest(reqBuff)
			if err != nil {
				log.Printf("%+v\n", err)
				// todo: tcp connect handler
				return
			}

			log.Printf(clientReq.URL.String())
			err = clientReq.Write(serverConn)
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			//}
			serverResp, err := http.ReadResponse(respBuff, clientReq)
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			//log.Println(serverResp)
			err = serverResp.Write(clientConn)
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			if serverResp.StatusCode == http.StatusUpgradeRequired {
				break
			}
		}
		//websocket
		go func() {
			n, err := io.Copy(serverConn, reqBuff)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			log.Println(n)
		}()
		go func() {
			n, err := io.Copy(clientConn, respBuff)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			log.Println(n)
		}()
	}
}

package socksmitm

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"time"

	"golang.org/x/net/proxy"
)

type Mux struct {
	DefaultHandler    HandlerFunc
	Handler           HostHandler
	DefaultUDPHandler UDPHandlerFunc
	UDPHandler        UDPHostHandler
}

type HostHandler map[string]HandlerFunc
type UDPHostHandler map[string]UDPHandlerFunc

type HandlerFunc func(clientConn net.Conn, isTls bool, host string, port int)
type UDPHandlerFunc func(clientConn net.Conn, host string, port int)

func NewMux(DefaultDialer proxy.Dialer) *Mux {
	return &Mux{
		DefaultHandler: NewNotRealReqHandlerFunc(DefaultDialer),
		//DefaultHandler:    NewDefaultHandlerFunc(DefaultDialer),
		Handler:           make(HostHandler),
		DefaultUDPHandler: NewDefaultUDPHandlerFunc(DefaultDialer),
		UDPHandler:        make(UDPHostHandler),
	}
}

func (mux *Mux) SetDefaultHandlerFunc(handler HandlerFunc) {
	mux.DefaultHandler = handler
}

func (mux *Mux) SetDefaultUDPHandlerFunc(UDPhandler UDPHandlerFunc) {
	mux.DefaultUDPHandler = UDPhandler
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

func (mux *Mux) UDPHandle(conn net.Conn, host string, port int) {
	udpHandler, ok := mux.UDPHandler[host]
	if !ok {
		mux.DefaultUDPHandler(conn, host, port)
		return
	}
	udpHandler(conn, host, port)
}

func BlockHandlerFunc(conn net.Conn, isTls bool, host string, port int) {
	//log.Println("block request to:", host)
	return
}

func BlockUDPHandlerFunc(conn net.Conn, host string, port int) {
	//log.Println("block request to:", host)
	return
}

func NewDefaultHandlerFunc(dialer proxy.Dialer) HandlerFunc {
	return func(clientConn net.Conn, isTls bool, host string, port int) {
		serverConn, err := dialer.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		if isTls {
			serverConn = tls.Client(serverConn, &tls.Config{
				InsecureSkipVerify: true,
				CipherSuites: []uint16{
					0xc02f,
					0xc030,
					0xc02b,
					0xc02c,
					0xc013,
					0xc009,
					0xc014,
					0xc00a,
					0x009c,
					0x009d,
					0x002f,
					0x0035,
					0xc012,
					0x000a,
				},
			})
		}
		defer serverConn.Close()
		defer clientConn.Close()
		go io.Copy(serverConn, clientConn)
		io.Copy(clientConn, serverConn)
	}
}

func NewNotRealReqHandlerFunc(dialer proxy.Dialer) HandlerFunc {
	return func(clientConn net.Conn, isTls bool, host string, port int) {
		defer clientConn.Close()
		clientConn.SetReadDeadline(time.Now().Add(time.Second * 5))
		body, err := ioutil.ReadAll(clientConn)
		if err != nil {
			log.Println(string(body))
			return
		}
	}
}

func NewDefaultUDPHandlerFunc(dialer proxy.Dialer) UDPHandlerFunc {
	return func(clientConn net.Conn, host string, port int) {
		//log.Println("req:", isTls, host, port)
		serverConn, err := dialer.Dial("udp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		defer serverConn.Close()
		defer clientConn.Close()
		//websocket
		go func() {
			_, err := io.Copy(serverConn, clientConn)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			//log.Println(n)
		}()
		go func() {
			_, err := io.Copy(clientConn, serverConn)
			if err != nil {
				log.Printf("%#v\n", err)
				return
			}
			//log.Println(n)
		}()
	}
}

package socksmitm

import (
	"bufio"
	"context"
	"fmt"
	"golang.org/x/net/proxy"
	"golang.org/x/xerrors"
	"io"
	"log"
	"net"
	"net/http"
)

type Mux struct {
	DefaultHTTPHandler HTTPRoundTrip
	HTTPHandlerMap     HTTPHandlerMap
	DefaultUDPHandler  UDPHandlerFunc
	UDPHandlerMap      UDPHandlerMap
}

type HTTPHandlerMap map[string]HTTPRoundTrip
type HTTPRoundTrip func(*http.Request) (*http.Response, error)
type UDPHandlerMap map[string]UDPHandlerFunc
type UDPHandlerFunc func(clientConn net.Conn, host string, port int)

func NewMux(DefaultDialer proxy.Dialer) *Mux {
	return &Mux{
		DefaultHTTPHandler: NormalRoundTrip,
		HTTPHandlerMap:     make(HTTPHandlerMap),
		DefaultUDPHandler:  NewDefaultUDPHandlerFunc(DefaultDialer),
		UDPHandlerMap:      make(UDPHandlerMap),
	}
}

func (mux *Mux) SetDefaultHTTPRoundTrip(handler HTTPRoundTrip) {
	mux.DefaultHTTPHandler = handler
}

func (mux *Mux) SetDefaultUDPHandlerFunc(UDPhandler UDPHandlerFunc) {
	mux.DefaultUDPHandler = UDPhandler
}

func (mux *Mux) Register(host string, handler HTTPRoundTrip) {
	mux.HTTPHandlerMap[host] = handler
}

func (mux *Mux) HandleHTTP(conn net.Conn, isTls bool, host string, port int) {
	defer func() {
		log.Println("conn closing...")
	}()
	for {
		handler, ok := mux.HTTPHandlerMap[host]
		if !ok {
			handler = mux.DefaultHTTPHandler
		}
		if handler == nil {
			return
		}
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		req = req.Clone(context.Background())
		if isTls {
			req.URL.Scheme = "https"
		} else {
			req.URL.Scheme = "http"
		}
		req.RequestURI = ""
		req.URL.Host = fmt.Sprintf("%s:%d", host, port)
		resp, err := handler(req)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		defer resp.Body.Close()
		err = resp.Write(conn)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}

}

func (mux *Mux) UDPHandle(conn net.Conn, host string, port int) {
	udpHandler, ok := mux.UDPHandlerMap[host]
	if !ok {
		mux.DefaultUDPHandler(conn, host, port)
		return
	}
	udpHandler(conn, host, port)
}

func BlockRoundTrip(req *http.Request) (*http.Response, error) {
	log.Println("block request to:", req.Host)
	return nil, xerrors.New("blocked")
}

func BlockUDPHandlerFunc(conn net.Conn, host string, port int) {
	//log.Println("block request to:", host)
	return
}

func NormalRoundTrip(req *http.Request) (*http.Response, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	return resp, nil
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

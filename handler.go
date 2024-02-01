package socksmitm

import (
	"bufio"
	"bytes"
	"crypto/tls"
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

func (mux *Mux) HandleHTTPS(conn net.Conn, clientHelloInfo *tls.ClientHelloInfo, targetIP string, port int) {
	for {
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			return
		}
		log.Println("req.Host:", req.Host, "req.URL.Host", req.URL.Host, "targetIP:", targetIP, "clientHelloInfo:", clientHelloInfo.ServerName)

		handler := mux.DefaultHTTPHandler
		handlerByHostName, ok := mux.HTTPHandlerMap[req.Host]
		if ok && handlerByHostName != nil {
			handler = handlerByHostName
		}
		req.URL.Scheme = "https"
		req.RequestURI = ""
		req.URL.Host = req.Host
		resp, err := handler(req)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		err = resp.Write(conn)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}
}
func (mux *Mux) HandleHTTP(conn net.Conn, targetIP string, port int) {
	for {
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			return
		}
		log.Println("req.Host:", req.Host, "req.URL.Host", req.URL.Host, "targetIP:", targetIP)

		handler := mux.DefaultHTTPHandler
		handlerByHostName, ok := mux.HTTPHandlerMap[req.Host]
		if ok && handlerByHostName != nil {
			handler = handlerByHostName
		}
		req.URL.Scheme = "http"
		req.RequestURI = ""
		req.URL.Host = req.Host
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
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}}
	resp, err := client.Do(req)
	if err != nil {
		//log.Println(req.Host, req.URL.Host)
		return nil, xerrors.Errorf("%w", err)
	}
	return resp, nil
}

func CopyRoundTrip(path string, handler func(req *http.Request, reqBody []byte, resp *http.Response, respBody []byte)) HTTPRoundTrip {
	return func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != path {
			return NormalRoundTrip(req)
		}
		reqBodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, xerrors.Errorf("%w", err)
		}
		req.ContentLength = int64(len(reqBodyBytes))
		req.Body = io.NopCloser(bytes.NewReader(reqBodyBytes))
		resp, err := NormalRoundTrip(req)
		if err != nil {
			return nil, xerrors.Errorf("%w\n", err)
		}
		respBodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, xerrors.Errorf("%w\n", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(reqBodyBytes))
		resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
		go handler(req, reqBodyBytes, resp, respBodyBytes)
		return resp, nil
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

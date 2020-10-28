package socksmitm

import (
	"crypto/tls"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"log"
	"net"
	"sync"
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

func NewDefaultHandlerFunc(dialer proxy.Dialer) HandlerFunc {
	return func(conn net.Conn, isTls bool, host string, port int) {
		log.Println("req:", isTls, host, port)
		c, err := dialer.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		if isTls {
			c = tls.Client(c, &tls.Config{
				InsecureSkipVerify:          true,
			})
		}
		defer c.Close()
		defer conn.Close()
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for{
				n,err := io.Copy(c, conn)
				if err != nil{
					log.Printf("%+v\n",err)
					return
				}
				if n==0{
					return
				}
				log.Println("from c to conn:",n)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for{
				n,err := io.Copy(conn,c)
				if err != nil{
					log.Printf("%+v\n",err)
					return
				}
				if n==0{
					return
				}
				log.Println("from conn to c:",n)
			}
		}()
		wg.Wait()
	}
}

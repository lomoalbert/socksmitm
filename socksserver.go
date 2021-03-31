package socksmitm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"golang.org/x/crypto/pkcs12"
	"golang.org/x/xerrors"
)

type Server struct {
	mux             *Mux
	rootCertificate *x509.Certificate
	rootPrivateKey  interface{}
	configs         map[string]*tls.Config
}

func NewSocks5Server(mux *Mux, pkcs12Data []byte, pkcs12Password string) (*Server, error) {
	privateKey, ca, err := pkcs12.Decode(pkcs12Data, pkcs12Password)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	return &Server{mux: mux, rootCertificate: ca, rootPrivateKey: privateKey, configs: make(map[string]*tls.Config)}, nil
}

func (server *Server) Run(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	log.Println("socks server listen:", addr)
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		//log.Println("got listener from:", conn.RemoteAddr())
		go func() {
			err = server.SocksHandle(conn)
			if err != nil {
				log.Printf("%+v\n", err)
			}
		}()
	}
}

func (server *Server) SocksHandle(conn net.Conn) error {
	defer conn.Close()
	req1Byes := make([]byte, 2)
	//+----+----------+----------+
	//|VER | NMETHODS | METHODS  |
	//+----+----------+----------+
	//| 1  |    1     | 1 to 255 |
	//+----+----------+----------+
	c, err := conn.Read(req1Byes)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	if c != 2 {
		return xerrors.Errorf("req header: %x", req1Byes)
	}
	//log.Println("client conn read:", req1Byes)
	reqMBytes := make([]byte, int(req1Byes[1]))
	c, err = conn.Read(reqMBytes)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	if c != int(req1Byes[1]) {
		return xerrors.Errorf("req header: %x", req1Byes)
	}
	//+----+--------+
	//|VER | METHOD |
	//+----+--------+
	//| 1  |   1    |
	//+----+--------+
	_, err = conn.Write([]byte{0x05, 0x00})
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	//+----+-----+-------+------+----------+----------+
	//|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	//+----+-----+-------+------+----------+----------+
	//| 1  |  1  | X'00' |  1   | Variable |    2     |
	//+----+-----+-------+------+----------+----------+
	reqMBytes = make([]byte, 2)
	c, err = conn.Read(reqMBytes)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	if c != 2 {
		return xerrors.Errorf("req header: %x", req1Byes)
	}
	cmd := reqMBytes[1]
	switch cmd {
	case 0x01: //CONNECT
		err = server.SocksTCPConnect(conn)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
	case 0x02: //BIND
		return xerrors.New("cmd unsupport") // todo:
	case 0x03: //UDP ASSOCIATE
		err = server.SocksUDPConnect(conn)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
	default:
		return xerrors.Errorf("cmd unsupport %x", cmd)
	}
	return nil
}

func (server *Server) SocksTCPConnect(conn net.Conn) error {
	reqMBytes := make([]byte, 2)
	//+-------+------+----------+----------+
	//|  RSV  | ATYP | DST.ADDR | DST.PORT |
	//+-------+------+----------+----------+
	//| X'00' |  1   | Variable |    2     |
	//+-------+------+----------+----------+
	c, err := conn.Read(reqMBytes)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	if c != 2 {
		return xerrors.Errorf("req header: %x", reqMBytes)
	}
	atyp := reqMBytes[1]
	switch atyp {
	case 0x01: // IPv4
		reqMBytes := make([]byte, 6)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != 6 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		dstAddr := reqMBytes[:4]
		port := reqMBytes[4:]
		server.SocksTCPConnectIPv4(conn, dstAddr, port)
	case 0x03: // 域名
		reqMBytes := make([]byte, 1)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != 1 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		domainLength := int(reqMBytes[0])
		reqMBytes = make([]byte, domainLength+2)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != domainLength+2 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		domain := reqMBytes[:domainLength]
		port := reqMBytes[domainLength:]
		server.SocksTCPConnectDomain(conn, domain, port)
	case 0x04: // IPv6
		return xerrors.New("atyp unsupport") // todo:
	default:
		return xerrors.New("atyp error")
	}
	return nil
}

func (server *Server) SocksTCPConnectIPv4(conn net.Conn, ip []byte, port []byte) {
	ipv4 := net.IPv4(ip[0], ip[1], ip[2], ip[3])
	portInt := int(port[0])*256 + int(port[1])
	domainStr := ipv4.String()
	conn.Write(append(append([]byte{0x05, 0x00, 0x00, 0x01}, ip...), port...))
	c1, c2 := net.Pipe()
	defer c2.Close()
	buff := make([]byte, 1)
	c, err := conn.Read(buff)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}

	isTls := buff[0] == byte(22)
	go func() {
		defer c1.Close()
		_, err := c1.Write(buff[:c])
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
		_, err = io.Copy(c1, conn)
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
	}()
	go func() {
		_, err := io.Copy(conn, c1)
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
	}()
	if isTls {
		c2 = tls.Server(c2, &tls.Config{GetConfigForClient: server.GenFuncGetConfigForClient(&domainStr)})
	}
	server.mux.HandleHTTP(c2, isTls, domainStr, portInt)
}

func (server *Server) SocksTCPConnectDomain(conn net.Conn, domain []byte, port []byte) {
	domainStr := string(domain)
	portInt := int(port[0])*256 + int(port[1])
	conn.Write(append(append([]byte{0x05, 0x00, 0x00, 0x03, byte(len(domain))}, domain...), port...))
	c1, c2 := net.Pipe()
	defer c2.Close()
	buff := make([]byte, 1)
	c, err := conn.Read(buff)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	//log.Println("content type:", buff[:c])

	isTls := buff[0] == byte(22)
	go func() {
		defer c1.Close()
		_, err := c1.Write(buff[:c])
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
		_, err = io.Copy(c1, conn)
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
	}()
	go func() {
		_, err := io.Copy(conn, c1)
		if err != nil {
			//log.Printf("%+v\n", err)
			return
		}
	}()
	if isTls {
		c2 = tls.Server(c2, &tls.Config{GetConfigForClient: server.GenFuncGetConfigForClient(&domainStr)})
	}
	server.mux.HandleHTTP(c2, isTls, domainStr, portInt)
}

func (server *Server) GenFuncGetConfigForClient(hostname *string) func(clientHelloInfo *tls.ClientHelloInfo) (*tls.Config, error) {
	return func(clientHelloInfo *tls.ClientHelloInfo) (*tls.Config, error) {
		*hostname = clientHelloInfo.ServerName
		config, ok := server.configs[MainDomain(clientHelloInfo.ServerName)]
		var err error
		if !ok {
			config, err = GenMITMTLSConfig(server.rootCertificate, server.rootPrivateKey, MainDomain(clientHelloInfo.ServerName))
			if err != nil {
				log.Printf("%+v\n", err)
				return nil, err
			}
			server.configs[MainDomain(clientHelloInfo.ServerName)] = config
		}
		return config, nil
	}
}

// RegisterRootCa 注册 root.ca 处理器, 用于浏览器获取ca证书
func (server *Server) RegisterRootCa() {
	server.mux.Register("root.ca", func(r *http.Request) (*http.Response, error) {
		rootCertData := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: server.rootCertificate.Raw,
		})
		defer r.Body.Close()
		header := "HTTP/1.1 200 OK\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=\"rootca.pem\"\nConnection: close\n\n"
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(append([]byte(header), rootCertData...))), r)
		if err != nil {
			return nil, xerrors.Errorf("%w", err)
		}
		resp.ContentLength = int64(len(rootCertData))
		return resp, nil
	})
}

func MainDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return domain
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func (server *Server) SocksUDPConnect(conn net.Conn) error {
	reqMBytes := make([]byte, 2)
	//+-------+------+----------+----------+
	//|  RSV  | ATYP | DST.ADDR | DST.PORT |
	//+-------+------+----------+----------+
	//| X'00' |  1   | Variable |    2     |
	//+-------+------+----------+----------+
	c, err := conn.Read(reqMBytes)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	if c != 2 {
		return xerrors.Errorf("req header: %x", reqMBytes)
	}
	atyp := reqMBytes[1]
	switch atyp {
	case 0x01: // IPv4
		reqMBytes := make([]byte, 6)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != 6 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		dstAddr := reqMBytes[:4]
		port := reqMBytes[4:]
		server.SocksUDPConnectIPv4(conn, dstAddr, port)
	case 0x03: // 域名
		reqMBytes := make([]byte, 1)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != 1 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		domainLength := int(reqMBytes[0])
		reqMBytes = make([]byte, domainLength+2)
		c, err = conn.Read(reqMBytes)
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		if c != domainLength+2 {
			return xerrors.Errorf("req header: %x", reqMBytes)
		}
		domain := reqMBytes[:domainLength]
		port := reqMBytes[domainLength:]
		server.SocksUDPConnectDomain(conn, domain, port)
	case 0x04: // IPv6
		return xerrors.New("atyp unsupport") // todo:
	default:
		return xerrors.New("atyp error")
	}
	return nil
}

func (server *Server) SocksUDPConnectIPv4(conn net.Conn, ip []byte, port []byte) {
	ipv4 := net.IPv4(ip[0], ip[1], ip[2], ip[3])
	portInt := int(port[0])*256 + int(port[1])
	//log.Println("ip:", ipv4)
	//log.Println("port:", portInt)
	domainStr := ipv4.String()
	conn.Write(append(append([]byte{0x05, 0x00, 0x00, 0x01}, ip...), port...))

	server.mux.UDPHandle(conn, domainStr, portInt)
}

func (server *Server) SocksUDPConnectDomain(conn net.Conn, domain []byte, port []byte) {
	domainStr := string(domain)
	portInt := int(port[0])*256 + int(port[1])
	//log.Println("domainStr:", domainStr)
	//log.Println("port:", portInt)
	conn.Write(append(append([]byte{0x05, 0x00, 0x00, 0x03, byte(len(domain))}, domain...), port...))
	server.mux.UDPHandle(conn, domainStr, portInt)
}

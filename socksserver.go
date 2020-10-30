package socksmitm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/pkcs12"
	"golang.org/x/xerrors"
	"io"
	"log"
	"net"
	"strings"
)

type Server struct {
	mux         *Mux
	certificate *x509.Certificate
	configs     map[string]*tls.Config
}

func NewSocks5Server(mux *Mux, pkcs12Data []byte, pkcs12Password string) (*Server, error) {
	_, ca, err := pkcs12.Decode(pkcs12Data, pkcs12Password)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	return &Server{mux: mux, certificate: ca, configs: make(map[string]*tls.Config)}, nil
}

func (server *Server) Run(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	log.Println("listen:", addr)
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return xerrors.Errorf("%w", err)
		}
		log.Println("got listener from:", conn.RemoteAddr())
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
	log.Println("client conn read:", req1Byes)
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
		return xerrors.New("cmd unsupport") // todo:
	default:
		return xerrors.New("cmd error")
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
		server.SocksConnectIPv4(conn, dstAddr, port)
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
		server.SocksConnectDomain(conn, domain, port)
	case 0x04: // IPv6
		return xerrors.New("atyp unsupport") // todo:
	default:
		return xerrors.New("atyp error")
	}
	return nil
}

func (server *Server) SocksConnectIPv4(conn net.Conn, ip []byte, port []byte) {
	ipv4 := net.IPv4(ip[0], ip[1], ip[2], ip[3])
	portInt := int(port[0])*256 + int(port[1])
	log.Println("ip:", ipv4)
	log.Println("port:", portInt)
	// todo:
}

func (server *Server) SocksConnectDomain(conn net.Conn, domain []byte, port []byte) {
	domainStr := string(domain)
	portInt := int(port[0])*256 + int(port[1])
	log.Println("domainStr:", domainStr)
	log.Println("port:", portInt)
	conn.Write(append(append([]byte{0x05, 0x00, 0x00, 0x03, byte(len(domain))}, domain...), port...))
	c1, c2 := net.Pipe()
	defer c2.Close()
	buff := make([]byte, 1)
	c, err := conn.Read(buff)
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	log.Println("content type:", buff[:c])

	isTls := buff[0] == byte(22)
	go func() {
		defer c1.Close()
		_, err := c1.Write(buff[:c])
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		_, err = io.Copy(c1, conn)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}()
	go func() {
		_, err := io.Copy(conn, c1)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}()
	if isTls {
		config, ok := server.configs[MainDomain(domainStr)]
		var err error
		if !ok {
			config, err = GenMITMTLSConfig(server.certificate, MainDomain(domainStr))
			if err != nil {
				log.Printf("%+v\n", err)
				return
			}
			server.configs[MainDomain(domainStr)] = config
		}
		c2 = tls.Server(c2, config)
	}
	server.mux.Handle(c2, isTls, domainStr, portInt)
}

// RegisterRootCa 注册 root.ca 处理器, 用于浏览器获取ca证书
func (server *Server) RegisterRootCa() {
	server.mux.Register("root.ca", func(conn net.Conn, isTls bool, host string, port int) {
		rootCertData := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: server.certificate.Raw,
		})
		buff := make([]byte, 1024)
		_, err := conn.Read(buff)
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
		log.Println(string(buff))
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=\"rootca.pem\"\nContent-Length: %d\nConnection: close\n\n", len(rootCertData))))
		conn.Write(rootCertData)
		return
	})
}

func MainDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return domain
	}
	return "*." + strings.Join(parts[len(parts)-2:], ".")
}

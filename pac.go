package socksmitm

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/xerrors"
	"log"
	"net"
	"net/http"
)

var pacContent = `
function FindProxyForURL(url, host)
{
    var direct = 'DIRECT';
    var tunnel = 'SOCKS %s:%d';
    if (isPlainHostName(host) ||
        host.indexOf('127.') == 0 ||
        host.indexOf('192.168.') == 0 ||
        host.indexOf('10.') == 0 ||
        shExpMatch(host, 'localhost.*'))
    {
        return direct;
    }

    return tunnel;
}
`

func PacListenAndServe(ctx context.Context, pacPort, socksPort int) error {
	ip, err := externalIP()
	if err != nil {
		return xerrors.Errorf("%w", err)
	}
	log.Println("pac url: " + fmt.Sprintf("http://%s:%d/", ip, pacPort))
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", ip, pacPort), Handler: &PacHandler{Host: ip, Port: pacPort, SocksPort: socksPort}}
	go func() {
		<-ctx.Done()
		srv.Shutdown(ctx)
	}()
	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.Printf("%+v\n", err)
			return
		}
	}()
	return nil
}

type PacHandler struct {
	Host      string
	Port      int
	SocksPort int
}

func (handler *PacHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	_, err := w.Write([]byte(fmt.Sprintf(pacContent, handler.Host, handler.SocksPort)))
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
}

func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("local ip not found")
}

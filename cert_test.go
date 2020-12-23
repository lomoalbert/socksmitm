package socksmitm_test

import (
	"github.com/lomoalbert/socksmitm"
	"golang.org/x/crypto/pkcs12"
	"io/ioutil"
	"log"
	"testing"
)

func TestServer_Run(t *testing.T) {
	caP12, err := ioutil.ReadFile("charles-ssl-proxying.p12")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	rootPrivateKey, ca, err := pkcs12.Decode(caP12, "DwCpsCLsZc7c")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	config, err := socksmitm.GenMITMTLSConfig(ca, rootPrivateKey, "baidu.com")
	if err != nil {
		log.Printf("%+v\n", err)
		return
	}
	for _, cert := range config.Certificates {
		log.Println(cert.PrivateKey)
	}
}

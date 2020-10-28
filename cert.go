package socksmitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"golang.org/x/xerrors"
	"log"
	"math/big"
	mathrand "math/rand"
	"time"
)

func GenMITMTLSConfig(rootCa *x509.Certificate,dnsName string)(config *tls.Config,err error){

	equiCer := &x509.Certificate{
		SerialNumber: big.NewInt(mathrand.Int63()), //证书序列号
		Subject: pkix.Name{
			Country:            []string{"CN"},
			Organization:       []string{"Easy"},
			OrganizationalUnit: []string{"Easy"},
			Province:           []string{"ShenZhen"},
			CommonName:         dnsName,
			Locality:           []string{"ShenZhen"},
		},
		NotBefore:             time.Now(),                                                                 //证书有效期开始时间
		NotAfter:              time.Now().AddDate(1, 0, 0),                                                //证书有效期结束时间
		BasicConstraintsValid: true,                                                                       //基本的有效性约束
		IsCA:                  false,                                                                      //是否是根证书
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, //证书用途(客户端认证，数据加密)
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageDataEncipherment,
		//IPAddresses: nil,
		DNSNames: []string{dnsName},
	}
	//生成公钥私钥对
	priKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil,xerrors.Errorf("%w",err)
	}
	subcert, err := x509.CreateCertificate(rand.Reader, equiCer, rootCa, &priKey.PublicKey, priKey)
	if err != nil {
		return nil,xerrors.Errorf("%w",err)
	}

	//编码证书文件和私钥文件
	caPem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: subcert,
	}
	cert := pem.EncodeToMemory(caPem)

	buf := x509.MarshalPKCS1PrivateKey(priKey)
	keyPem := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: buf,
	}
	key := pem.EncodeToMemory(keyPem)

	cer, err := tls.X509KeyPair(cert, key)
	if err != nil {
		log.Println(err)
		return
	}
	config = &tls.Config{Certificates: []tls.Certificate{cer}}
	return config,nil
}
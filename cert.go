package socksmitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"

	"golang.org/x/xerrors"
)

func GenMITMTLSConfig(rootCa *x509.Certificate, rootPrivateKey interface{}, dnsName string) (config *tls.Config, err error) {
	now := time.Now().Add(-1 * time.Hour).UTC()
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}
	equiCer := &x509.Certificate{
		SerialNumber: serialNumber, //证书序列号
		Subject: pkix.Name{
			Country:            []string{"CN"},
			Organization:       []string{"Easy"},
			OrganizationalUnit: []string{"Easy"},
			Province:           []string{"ShenZhen"},
			Locality:           []string{"ShenZhen"},
			CommonName:         dnsName,
		},
		NotBefore:             now.Add(-time.Hour),  //证书有效期开始时间
		NotAfter:              now.AddDate(1, 0, 0), //证书有效期结束时间
		BasicConstraintsValid: true,                 //基本的有效性约束
		MaxPathLen:            -1,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, //证书用途(客户端认证，数据加密)
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		//SubjectKeyId: rootCa.SubjectKeyId,
	}
	if ip := net.ParseIP(dnsName); ip != nil {
		equiCer.IPAddresses = []net.IP{ip}
	} else {
		equiCer.DNSNames = []string{dnsName, "*." + dnsName}
	}
	//生成公钥私钥对
	priKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}

	x, err := x509.CreateCertificate(rand.Reader, equiCer, rootCa, &priKey.PublicKey, rootPrivateKey)
	if err != nil {
		return nil, xerrors.Errorf("%w", err)
	}

	////编码证书文件和私钥文件
	//caPem := &pem.Block{
	//	Type:  "CERTIFICATE",
	//	Bytes: subCert,
	//}
	//cert := pem.EncodeToMemory(caPem)
	//
	//buf := x509.MarshalPKCS1PrivateKey(priKey)
	//keyPem := &pem.Block{
	//	Type:  "PRIVATE KEY",
	//	Bytes: buf,
	//}
	//key := pem.EncodeToMemory(keyPem)
	//
	//cer, err := tls.X509KeyPair(cert, key)
	//if err != nil {
	//	log.Println(err)
	//	return
	//}
	//cer.Certificate = append(cer.Certificate, )
	cert := tls.Certificate{}
	cert.Certificate = append(cert.Certificate, x, rootCa.Raw)
	cert.PrivateKey = priKey
	cert.Leaf = rootCa
	config = &tls.Config{
		Certificates: []tls.Certificate{cert},
		//NextProtos:   []string{"h2", "http/1.1"},
	}
	return config, nil
}

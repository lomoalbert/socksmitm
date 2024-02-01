Q: pkcs12: unknown digest algorithm 2.16.840.1.101.3.4.2.1
A: pkcs12 -> pem -> pkcs12
> openssl pkcs12 -info -in charles-ssl-proxying.p12 -nodes -out charles-ssl-proxying.pem
> openssl pkcs12 -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -export -macalg sha1 -in charles-ssl-proxying.pem -inkey charles-ssl-proxying.pem -CAfile charles-ssl-proxying.pem -out charles-ssl-proxying.p12 -passout pass:yourpassword

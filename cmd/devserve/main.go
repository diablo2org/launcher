// Command devserve serves a folder over HTTPS on 127.0.0.1 for local testing,
// since the launcher only fetches over HTTPS. It writes its self-signed
// certificate to a file; start the launcher with LAUNCHER_DEV_CA set to that
// file to trust it. Never used in a release.
//
//	devserve -dir testenv/site -cert testenv/.dev/devserve.pem -addr 127.0.0.1:8667
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("dir", ".", "folder to serve")
	certFile := flag.String("cert", "devserve.pem", "where to write the certificate for the launcher to trust")
	addr := flag.String("addr", "127.0.0.1:8667", "address to listen on")
	flag.Parse()

	cert, pemBytes, err := selfSigned()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(*certFile), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*certFile, pemBytes, 0o644); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:      *addr,
		Handler:   logged(http.FileServer(http.Dir(*dir))),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	log.Printf("serving %s at https://%s (certificate: %s)", *dir, *addr, *certFile)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}

// selfSigned makes a short-lived certificate for 127.0.0.1 and localhost.
func selfSigned() (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "launcher devserve"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(7 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pemBytes, nil
}

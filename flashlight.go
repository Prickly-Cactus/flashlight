// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	//"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/oxtoacart/go-mitm/mitm"
	"github.com/oxtoacart/keyman"
)

const (
	CONNECT          = "CONNECT"
	ONE_WEEK         = 7 * 24 * time.Hour
	TWO_WEEKS        = ONE_WEEK * 2
	X_LANTERN_HOST   = "X-Lantern-Host"
	X_LANTERN_SCHEME = "X-Lantern-Scheme"

	PK_FILE   = "proxypk.pem"
	CERT_FILE = "proxycert.pem"
)

var (
	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	mitmAddr     = flag.String("mitmAddr", "localhost:10093", "ip:port on which the client mitm proxy should listen for requets, defaults to localhost:10093")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverPort", 80, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")

	isDownstream bool

	pk             *keyman.PrivateKey
	pkPem          []byte
	issuingCert    *keyman.Certificate
	issuingCertPem []byte

	wg sync.WaitGroup

	mitmProxy *mitm.Proxy
)

func init() {
	flag.Parse()
	if *help || *addr == "" {
		flag.Usage()
		os.Exit(1)
	}

	isDownstream = *upstreamHost != ""
}

func main() {
	if err := initCerts(); err != nil {
		log.Fatalf("Unable to initialize certs: %s", err)
	}
	if isDownstream {
		runClient()
		runMitmProxy()
	} else {
		runServer()
	}
	wg.Wait()
}

// runClient runs the client HTTP proxy server
func runClient() {
	// On the client, use a bunch of CPUs if necessary
	runtime.GOMAXPROCS(4)
	wg.Add(1)

	server := &http.Server{
		Addr:         *addr,
		Handler:      http.HandlerFunc(handleClient),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("About to start client (http) proxy at %s", *addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Unable to start client proxy: %s", err)
		}
		wg.Done()
	}()
}

// runMitmProxy runs the MITM proxy that the client uses for proxying HTTPS requests
// we have to MITM these because we can't CONNECT tunnel through CloudFlare
func runMitmProxy() {
	wg.Add(1)

	var err error
	mitmProxy, err = mitm.NewProxy(PK_FILE, CERT_FILE, *mitmAddr)
	if err != nil {
		log.Fatalf("Unable to initialize mitm proxy: %s", err)
	}

	mitmProxy.HandlerFunc = handleClientMITM
	go func() {
		log.Printf("About to start mitm proxy at %s", *mitmAddr)
		errCh := mitmProxy.Start()
		if err := <-errCh; err != nil {
			log.Fatalf("Unable to start mitm proxy: %s", err)
		}
		wg.Done()
	}()
}

// runServer runs the server HTTPS proxy
func runServer() {
	wg.Add(1)

	server := &http.Server{
		Addr:         *addr,
		Handler:      http.HandlerFunc(handleServer),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("About to start server (https) proxy at %s", *addr)
		// if err := server.ListenAndServeTLS(CERT_FILE, PK_FILE); err != nil {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Unable to start server proxy: %s", err)
		}
		wg.Done()
	}()
}

// handleClient handles requests from a local client (e.g. the browser)
func handleClient(resp http.ResponseWriter, req *http.Request) {
	if req.Method == "CONNECT" {
		mitmProxy.Intercept(resp, req)
	} else {
		req.URL.Scheme = "http"
		doHandleClient(resp, req)
	}
}

// doHandleClient does the work of handling client HTTP requests and injecting
// special Lantern headers to work correctly with the upstream server proxy.
func doHandleClient(resp http.ResponseWriter, req *http.Request) {
	host := *upstreamHost
	if *masqueradeAs != "" {
		host = *masqueradeAs
	}
	upstreamAddr := fmt.Sprintf("%s:%d", host, *upstreamPort)

	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Remember the host and scheme that was actually requested
			req.Header.Set(X_LANTERN_HOST, req.Host)
			req.Header.Set(X_LANTERN_SCHEME, req.URL.Scheme)
			req.URL.Scheme = "http"
			// Set our upstream proxy as the host for this request
			req.Host = *upstreamHost
			req.URL.Host = "bubba"

			log.Println(spew.Sdump(req))
		},
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(fmt.Sprintf("http://www.google.com"))
			},
			Dial: func(network, addr string) (net.Conn, error) {
				// log.Printf("Using %s to handle request for: %s", upstreamAddr, req.URL.String())
				// tlsConfig := &tls.Config{
				// 	ServerName:         host,
				// 	InsecureSkipVerify: true,
				// }
				// return tls.Dial(network, upstreamAddr, tlsConfig)
				return net.Dial(network, upstreamAddr)
			},
		},
	}
	rp.ServeHTTP(resp, req)
}

// handleClientMITM handles requests to the client-side MITM proxy, making some
// small modifications and then delegating to doHandleClient.
func handleClientMITM(resp http.ResponseWriter, req *http.Request) {
	req.URL.Scheme = "https"
	req.Host = hostIncludingPort(req)
	doHandleClient(resp, req)
}

func hostIncludingPort(req *http.Request) (host string) {
	host = req.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}
	return
}

// handleServer handles requests from a downstream flashlight client
func handleServer(resp http.ResponseWriter, req *http.Request) {
	log.Println(spew.Sdump(req))

	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// TODO - need to add support for tunneling HTTPS traffic using CONNECT
			req.URL.Scheme = req.Header.Get(X_LANTERN_SCHEME)
			// Grab the actual host from the original client and use that for the outbound request
			req.URL.Host = req.Header.Get(X_LANTERN_HOST)
			req.Host = req.URL.Host
			log.Printf("Handling request for: %s", req.URL.String())
		},
	}
	rp.ServeHTTP(resp, req)
}

// initCerts initializes server certificates, used both for the server HTTPS
// proxy and the client MITM proxy
func initCerts() (err error) {
	if pk, err = keyman.LoadPKFromFile(PK_FILE); err != nil {
		if pk, err = keyman.GeneratePK(2048); err != nil {
			return
		}
		if err = pk.WriteToFile(PK_FILE); err != nil {
			return
		}
	}
	pkPem = pk.PEMEncoded()
	if issuingCert, err = keyman.LoadCertificateFromFile(CERT_FILE); err != nil {
		// TODO: don't hardcode the common name
		if issuingCert, err = certificateFor("lantern.io", nil); err != nil {
			return
		}
		if err = issuingCert.WriteToFile(CERT_FILE); err != nil {
			return
		}
	}
	issuingCertPem = issuingCert.PEMEncoded()
	return
}

// certificateFor generates a certificate for a given name, signed by the given
// issuer.  If no issuer is specified, the generated certificate is
// self-signed.
func certificateFor(name string, issuer *keyman.Certificate) (cert *keyman.Certificate, err error) {
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(int64(time.Now().Nanosecond())),
		Subject: pkix.Name{
			Organization: []string{"Lantern"},
			CommonName:   name,
		},
		NotBefore: now.Add(-1 * ONE_WEEK),
		NotAfter:  now.Add(TWO_WEEKS),

		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	if issuer == nil {
		template.KeyUsage = template.KeyUsage | x509.KeyUsageCertSign
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IsCA = true
	}
	cert, err = pk.Certificate(template, issuer)
	return
}

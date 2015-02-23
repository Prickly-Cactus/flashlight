package statserver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/waitforserver"

	"github.com/getlantern/flashlight/ui"
)

var (
	log = golog.LoggerFor("flashlight.statserver")

	service    *ui.Service
	cfgMutex   sync.RWMutex
	geoClient  *http.Client
	peers      map[string]*Peer
	peersMutex sync.RWMutex
)

func Configure(newClient *http.Client) {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()
	geoClient = newClient
	if service == nil {
		err := registerService()
		if err != nil {
			log.Errorf("Unable to start statserver: %v", err)
			return
		}
		go read()
		log.Debug("Started")
	}
}

func configureGeoClient(proxyAddr string) error {
	proxyUrl, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		return err
	}
	err = waitforserver.WaitForServer("tcp", proxyAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("Proxy never came online at %v: %v", proxyAddr, err)
	}
	geoClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
			TLSClientConfig: &tls.Config{
				ClientSessionCache: tls.NewLRUClientSessionCache(10),
			},
		},
	}
	log.Debugf("Using proxy at: %v", proxyUrl)
	return nil
}

func registerService() error {
	peers = make(map[string]*Peer)
	helloFn := func(write func(interface{}) error) error {
		log.Tracef("Writing all peers to new client")
		peersMutex.RLock()
		defer peersMutex.RUnlock()
		for _, peer := range peers {
			err := write(peerUpdate(peer))
			if err != nil {
				return err
			}
		}
		return nil
	}

	var err error
	service, err = ui.Register("Stats", nil, helloFn)
	return err
}

func OnBytesReceived(ip string, bytes int64) {
	getOrCreatePeer(ip).onBytesReceived(bytes)
}

func OnBytesSent(ip string, bytes int64) {
	getOrCreatePeer(ip).onBytesSent(bytes)
}

func getOrCreatePeer(ip string) *Peer {
	peersMutex.Lock()
	defer peersMutex.Unlock()
	peer, found := peers[ip]
	if found {
		return peer
	}
	peer = newPeer(ip, onPeerUpdate)
	peers[ip] = peer
	return peer
}

type update struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func onPeerUpdate(peer *Peer) {
	service.Out <- peerUpdate(peer)
}

func peerUpdate(peer *Peer) *update {
	return &update{
		Type: "peer",
		Data: peer,
	}
}

func read() {
	for _ = range service.In {
		// Discard message, just in case any message is sent to this service.
	}
}

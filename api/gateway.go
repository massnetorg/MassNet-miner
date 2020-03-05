package api

import (
	"flag"
	"fmt"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	gw "massnet.org/mass/api/proto"
	"massnet.org/mass/config"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
)

const (
	DefaultHTTPLimit = 128 // DefaultHTTPLimit default max http connections
)

func Run(cfg *config.Config) error {
	portHttp := cfg.Network.API.APIPortHttp
	portGRPC := cfg.Network.API.APIPortGRPC

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard,
		&runtime.JSONPb{OrigName: true, EmitDefaults: true}))
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize))}
	echoEndpoint := flag.String("echo_endpoint", ":"+portGRPC, "endpoint of Service")
	err := gw.RegisterApiServiceHandlerFromEndpoint(ctx, mux, *echoEndpoint, opts)
	if err != nil {
		return err
	}

	// TODO: CORS
	isAllowedAddress, err := getIPAccessControlFunc(cfg.Network.API.APIWhitelist, cfg.Network.API.APIAllowedLan)
	if err != nil {
		return err
	}

	handle := accessControlHandler(concurrentRequestHandler(maxBytesHandler(mux)), isAllowedAddress)
	port := fmt.Sprintf("%s%s", ":", portHttp)
	return http.ListenAndServe(port, handle)
}

var (
	rfc1918_10  = net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)}
	rfc1918_192 = net.IPNet{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)}
	rfc1918_172 = net.IPNet{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)}
	lanRules    = map[string]net.IPNet{"10": rfc1918_10, "192": rfc1918_192, "172": rfc1918_172}
)

func getIPAccessControlFunc(whitelist []string, lanPrefix []string) (func(addr string) bool, error) {
	allowedIPs := make([]net.IP, 0, len(whitelist))
	var allowAllIP bool
	for i, addr := range whitelist {
		if addr == "*" {
			allowAllIP = true
			continue
		}
		tcpIP := net.ParseIP(addr)
		if tcpIP == nil {
			return nil, errors.New(fmt.Sprintln("invalid whitelist", i, addr))
		}
		allowedIPs = append(allowedIPs, tcpIP)
	}

	allowedLANs := make([]net.IPNet, 0, len(lanRules))
	for i, lan := range lanPrefix {
		netRule, ok := lanRules[lan]
		if !ok {
			logging.CPrint(logging.ERROR, "invalid lan prefix", logging.LogFormat{"index": i, "prefix": lan})
			continue
		}
		// TODO: check duplicate lan
		allowedLANs = append(allowedLANs, netRule)
	}

	var fn = func(addr string) bool {
		if allowAllIP {
			return true
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			logging.CPrint(logging.WARN, "api fail to resolve http request.RemoteAddr", logging.LogFormat{"request_addr": addr, "err": err})
			return false
		}

		// allow localhost
		if tcpAddr.IP.String() == "127.0.0.1" || tcpAddr.IP.String() == "::1" {
			return true
		}

		// allow whitelist
		for _, ip := range allowedIPs {
			if ip.String() == tcpAddr.IP.String() {
				return true
			}
		}

		// allow LAN
		for _, rule := range allowedLANs {
			if rule.Contains(tcpAddr.IP) {
				return true
			}
		}

		return false
	}

	return fn, nil
}

func accessControlHandler(h http.Handler, isAllowedAddress func(addr string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !isAllowedAddress(req.RemoteAddr) {
			logging.CPrint(logging.WARN, "api received request from forbidden address", logging.LogFormat{"remote_addr": req.RemoteAddr, "url_path": req.URL.Path})
			runtime.OtherErrorHandler(w, req, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, req)
	})
}

func statusUnavailableHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("{\"err:\",\"Sorry, we received too many simultaneous requests.\nPlease try again later.\"}"))
}

func concurrentRequestHandler(h http.Handler) http.Handler {
	httpCh := make(chan struct{}, DefaultHTTPLimit)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case httpCh <- struct{}{}:
			defer func() { <-httpCh }()
			h.ServeHTTP(w, req)

		default:
			logging.CPrint(logging.WARN, "too many concurrent requests", logging.LogFormat{"address": req.RemoteAddr, "path": req.URL.Path})
			statusUnavailableHandler(w, req)
		}
	})
}

func maxBytesHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// A block can easily be bigger than maxReqSize, but everything
		// else should be pretty small.
		req.Body = http.MaxBytesReader(w, req.Body, maxMsgSize)
		h.ServeHTTP(w, req)
	})
}

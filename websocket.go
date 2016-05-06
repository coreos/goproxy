package goproxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func isWebSocketRequest(r *http.Request) bool {
	contains := func(key, val string) bool {
		vv := strings.Split(r.Header.Get(key), ",")
		for _, v := range vv {
			if val == strings.ToLower(strings.TrimSpace(v)) {
				return true
			}
		}
		return false
	}
	if !contains("Connection", "upgrade") {
		return false
	}
	if !contains("Upgrade", "websocket") {
		return false
	}
	return true
}

func (proxy *ProxyHttpServer) handleWebsocket(ctx *ProxyCtx, tlsConfig *tls.Config, w http.ResponseWriter, req *http.Request) {
	// Assuming wss since we got here from a CONNECT
	targetURL := url.URL{Scheme: "wss", Host: req.URL.Host, Path: req.URL.Path}

	// Run request through handlers
	req, resp := proxy.filterRequest(req, ctx)
	if resp != nil {
		//TODO handle this
	}

	targetSiteCon, err := tls.Dial("tcp", targetURL.Host, defaultTLSConfig)
	if err != nil {
		ctx.Logf("Error dialing target site: %v")
		return
	}
	defer targetSiteCon.Close()

	// write handshake request to target
	err = req.Write(targetSiteCon)
	if err != nil {
		ctx.Logf("Error writing request: %v", err)
		return
	}

	targetTlsReader := bufio.NewReader(targetSiteCon)

	// Read handshake response from target
	resp, err = http.ReadResponse(targetTlsReader, req)
	if err != nil && err != io.EOF {
		ctx.Warnf("Cannot read TLS resp from mitm'd client %v %v", targetURL.Host, err)
		return
	}

	// Run response through handlers
	resp = proxy.filterResponse(resp, ctx)

	// Can safely ignore ok, connection already hijacked
	hij, _ := w.(http.Hijacker)
	clientCon, _, _ := hij.Hijack()

	// Proxy handshake back to client
	err = resp.Write(clientCon)
	if err != nil {
		ctx.Logf("Error writing response: %v", err)
		return
	}

	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		ctx.Logf("err: %v", err)
		errChan <- err
	}

	// Start proxying websocket data
	go cp(targetSiteCon, clientCon)
	go cp(clientCon, targetSiteCon)
	<-errChan
}

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

func (proxy *ProxyHttpServer) handleWebsocket(ctx *ProxyCtx, tlsConfig *tls.Config, w http.ResponseWriter, req *http.Request, clientCon *tls.Conn) {
	// Assuming wss since we got here from a CONNECT
	targetURL := url.URL{Scheme: "wss", Host: req.URL.Host, Path: req.URL.Path}

	// Run request through handlers
	req, resp := proxy.filterRequest(req, ctx)
	if resp != nil {
		//TODO handle this
	}

	targetSiteCon, err := tls.Dial("tcp", targetURL.Host, tlsConfig)
	if err != nil {
		ctx.Warnf("Error dialing target site: %v")
		return
	}
	defer targetSiteCon.Close()

	// write handshake request to target
	err = req.Write(targetSiteCon)
	if err != nil {
		ctx.Warnf("Error writing upgrade request: %v", err)
		return
	}

	targetTlsReader := bufio.NewReader(targetSiteCon)

	// Read handshake response from target
	resp, err = http.ReadResponse(targetTlsReader, req)
	if err != nil {
		ctx.Warnf("Error reading handhsake response  %v", err)
		return
	}

	// Run response through handlers
	resp = proxy.filterResponse(resp, ctx)

	// Proxy handshake back to client
	err = resp.Write(clientCon)
	if err != nil {
		ctx.Warnf("Error writing handshake response: %v", err)
		return
	}

	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		ctx.Warnf("Websocket error: %v", err)
		errChan <- err
	}

	// Start proxying websocket data
	go cp(targetSiteCon, clientCon)
	go cp(clientCon, targetSiteCon)
	<-errChan
}

package goproxy

import (
	"crypto/tls"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/url"
)

func (proxy *ProxyHttpServer) handleWebsocket(ctx *ProxyCtx, tlsConfig *tls.Config, w http.ResponseWriter, req *http.Request) {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	//TODO: pull protocols etc from headers into dialer?
	dialer := websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	requestHeader := http.Header{}
	if origin := req.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin", origin)
	}
	for _, prot := range req.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range req.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}

	targetURL := url.URL{Scheme: "wss", Host: req.URL.Host, Path: req.URL.Path}
	targetCon, resp, err := dialer.Dial(targetURL.String(), requestHeader)

	if err != nil {
		ctx.Logf("Unable to dial target websocket: %s\n", err)
		return
	}
	defer targetCon.Close()

	upgradeHeader := http.Header{}
	if header := resp.Header.Get("Sec-Websocket-Protocol"); header != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", header)
	}
	if header := resp.Header.Get("Set-Cookie"); header != "" {
		upgradeHeader.Set("Set-Cookie", header)
	}

	clientCon, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		ctx.Warnf("Couldn't upgrade client connection:  %s\n", err)
		return
	}
	defer clientCon.Close()

	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		ctx.Logf("err: %v", err)
		errChan <- err
	}

	go cp(targetCon.UnderlyingConn(), clientCon.UnderlyingConn())
	go cp(clientCon.UnderlyingConn(), targetCon.UnderlyingConn())
	<-errChan
}

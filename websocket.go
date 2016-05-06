package goproxy

import (
	"bufio"
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

	targetURL := url.URL{Scheme: "wss", Host: req.URL.Host, Path: req.URL.Path}

	req, resp := proxy.filterRequest(req, ctx)
	if resp != nil {
		//TODO handle this
	}
	ctx.Logf("req: %v", req)

	targetSiteCon, err := tls.Dial("tcp", targetURL.Host, defaultTLSConfig)
	if err != nil {
		ctx.Logf("Error dialing target site: %v")
		return
	}
	defer targetSiteCon.Close()

	err = req.Write(targetSiteCon)
	if err != nil {
		ctx.Logf("Error writing request: %v", err)
		return
	}

	targetTlsReader := bufio.NewReader(targetSiteCon)
	resp, err = http.ReadResponse(targetTlsReader, req)
	if err != nil && err != io.EOF {
		ctx.Warnf("Cannot read TLS resp from mitm'd client %v %v", targetURL.Host, err)
		return
	}

	ctx.Logf("resp: %v", resp)
	clientCon, err := upgrader.Upgrade(w, req, resp.Header)
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

	go cp(targetSiteCon, clientCon.UnderlyingConn())
	go cp(clientCon.UnderlyingConn(), targetSiteCon)
	<-errChan
}

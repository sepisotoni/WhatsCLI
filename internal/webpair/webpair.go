// Package webpair serves the pairing QR on a temporary LAN-accessible page for
// terminals that cannot display it. The QR is a credential: the server binds
// to all interfaces, uses an unguessable path, sends no-store headers, and
// stops itself after a short TTL.
package webpair

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

type Server struct {
	URL  string
	Port int
	stop func()
}

func (s *Server) Stop() {
	if s != nil && s.stop != nil {
		s.stop()
	}
}

// Start listens on a random port on all interfaces. qr returns the current
// pairing payload ("" while none is available).
func Start(ctx context.Context, qr func() string, ttl time.Duration) (*Server, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	lanIP, err := lanIPv4()
	if err != nil {
		return nil, err
	}
	prefix := "/" + hex.EncodeToString(token) + "/"
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, errors.New("could not open a local port")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		fmt.Fprint(w, page(qr()))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
	var once sync.Once
	stop := func() { once.Do(func() { srv.Close() }) }
	go srv.Serve(listener)
	go func() {
		timer := time.NewTimer(ttl)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		stop()
	}()
	return &Server{URL: fmt.Sprintf("http://%s:%d%s", lanIP, port, prefix), Port: port, stop: stop}, nil
}

func lanIPv4() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.New("could not find a local network address for web pairing")
	}
	var selected net.IP
	selectedPreferred := false
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			preferred := !virtualInterface(iface.Name)
			if selected == nil || preferred && !selectedPreferred || preferred == selectedPreferred && ip.IsPrivate() && !selected.IsPrivate() {
				selected = ip
				selectedPreferred = preferred
			}
		}
	}
	if selected == nil {
		return nil, errors.New("web pairing needs an active IPv4 network address")
	}
	return selected, nil
}

func virtualInterface(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "podman", "cni", "flannel"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func svg(payload string) string {
	q, err := qrcode.New(payload, qrcode.Low)
	if err != nil {
		return ""
	}
	bits := q.Bitmap() // includes the encoder's 4-module quiet zone
	n := len(bits)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="440" height="440" shape-rendering="crispEdges" role="img" aria-label="WhatsApp pairing code"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="`, n, n)
	for y, row := range bits {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

func page(payload string) string {
	body := `<p class="wait">Waiting for a pairing code&hellip; this page refreshes automatically.</p>`
	if payload != "" {
		if s := svg(payload); s != "" {
			body = `<div class="qr">` + s + `</div>`
		}
	}
	return `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="3"><title>WhatsCLI · Link a device</title>
<style>
:root{color-scheme:dark}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#020d0a;color:#eaf6ef;font:16px ui-monospace,Menlo,Consolas,monospace}
main{width:min(92vw,880px);border:1px solid #1fe590;border-radius:20px;background:#04150f;padding:36px;display:grid;gap:28px;grid-template-columns:1fr auto}
@media(max-width:760px){main{grid-template-columns:1fr}}
h1{margin:0 0 4px;letter-spacing:.08em}h1 b{color:#1fe590}p{color:#8ba89a;margin:6px 0}
ol{padding:0;margin:20px 0;list-style:none;counter-reset:s}li{counter-increment:s;margin:12px 0;display:flex;gap:14px;align-items:center}
li::before{content:counter(s);display:grid;place-items:center;width:34px;height:34px;border:1px solid #0f5238;border-radius:8px;color:#1fe590}
.qr{padding:14px;border-radius:18px;background:#fff;box-shadow:0 0 0 2px #1fe590,0 0 40px #1fe59055}.qr svg{display:block;max-width:100%;height:auto}
.note{border:1px solid #0f5238;border-radius:10px;padding:10px 14px}.wait{align-self:center}
</style><main><section><h1>LINK <b>A DEVICE</b></h1><p>Log in to WhatsApp</p>
<ol><li>On your phone, open WhatsApp.</li><li>Go to Settings or &#8942; &rarr; Linked devices.</li><li>Select Link a device and scan the QR.</li><li>Keep your terminal open until linked.</li></ol>
<p class="note">The code refreshes automatically. Anyone who can access this page can link a device. Share only with people you trust.</p></section>` + body + `</main></html>`
}

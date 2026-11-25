package nucleus

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bf4/logger"
)

const (
	Host = ""
	Port = 80
)

var (
	psnTicket         string
	authorizationCode string
	ticketMu          sync.RWMutex
)

type NucleusServer struct {
	server *http.Server

	finalOnce sync.Once
	final     http.Handler
}

const LogBodyMax = 4096

func New() *NucleusServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/ps3.php", handlePS3)
	mux.HandleFunc("/success", handleSuccess)

	return &NucleusServer{
		server: &http.Server{
			Addr:     ":80",
			Handler:  mux,
			ErrorLog: log.New(logWriter{"HTTP"}, "", 0),
		},
	}
}

func (s *NucleusServer) handler() http.Handler {
	s.finalOnce.Do(func() {
		s.final = logAll(s.server.Handler)
	})
	return s.final
}

func (s *NucleusServer) Mount(wrap func(http.Handler) http.Handler) {
	if s == nil || s.server == nil || wrap == nil {
		return
	}
	s.server.Handler = wrap(s.server.Handler)
}

func (s *NucleusServer) Start() error {
	s.server.Handler = s.handler()
	logger.Info("[NUCLEUS] HTTP listener active on %s", s.server.Addr)

	err := s.server.ListenAndServe()

	if err == http.ErrServerClosed {
		return nil
	}

	return err
}

type logWriter struct{ name string }

func (w logWriter) Write(p []byte) (int, error) {
	logger.Warn("[NUCLEUS %s] %s", w.name, strings.TrimSpace(string(p)))
	return len(p), nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func logAll(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("[NUCLEUS] %s %s Host=%s from %s", r.Method, r.RequestURI, r.Host, r.RemoteAddr)
		for name, values := range r.Header {
			for _, v := range values {
				logger.Debug("[NUCLEUS]   %s: %s", name, v)
			}
		}

		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil {
				logger.Warn("[NUCLEUS]   body read error: %v", err)
			}
			if len(body) > 0 {
				shown := body
				suffix := ""
				if len(shown) > LogBodyMax {
					shown = shown[:LogBodyMax]
					suffix = fmt.Sprintf(" ...(+%d bytes)", len(body)-LogBodyMax)
				}
				logger.Debug("[NUCLEUS]   body (%d bytes): %q%s", len(body), shown, suffix)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		rec := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		if loc := rec.Header().Get("Location"); loc != "" {
			logger.Info("[NUCLEUS] -> %d (%d bytes) Location=%s in %s", rec.status, rec.bytes, loc, time.Since(start).Round(time.Microsecond))
		} else {
			logger.Info("[NUCLEUS] -> %d (%d bytes) in %s", rec.status, rec.bytes, time.Since(start).Round(time.Microsecond))
		}
	})
}

func logRequest(handler string, r *http.Request) {
	logger.Debug("[NUCLEUS]   handled by %s", handler)
}

func setHTMLHeaders(w http.ResponseWriter) {
	w.Header().Set("Server", "nginx")
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Header().Set("Connection", "close")
}

func setPsnTicket(ticket string) {
	ticketMu.Lock()
	psnTicket = ticket
	ticketMu.Unlock()
}

func getPsnTicket() string {
	ticketMu.RLock()
	defer ticketMu.RUnlock()
	return psnTicket
}

func setAuthorizationCode(code string) {
	ticketMu.Lock()
	authorizationCode = code
	ticketMu.Unlock()
}

func redirect(response http.ResponseWriter, location string) {
	setHTMLHeaders(response)
	response.Header().Set("Location", location)
	response.WriteHeader(http.StatusFound)

	_, _ = response.Write([]byte(
		"<html><head><title>302 Found</title></head>" +
			"<body><h1>302 Found</h1></body></html>",
	))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	logRequest("root", r)

	ticket := getPsnTicket()

	if ticket == "" {
		writeText(w, http.StatusBadRequest, "Missing PSN ticket")
		return
	}

	location := "http://clientconfig.ea.com:80/success?code=" + url.QueryEscape(ticket)
	redirect(w, location)
}

func handlePS3(w http.ResponseWriter, r *http.Request) {
	logRequest("ps3", r)

	connect := r.URL.Query().Get("connect")
	proxy := r.URL.Query().Get("proxy")

	ticket := r.Header.Get("psn_ticket")

	if ticket != "" {
		setPsnTicket(ticket)
	}

	if r.Body != nil {
		_, _ = io.ReadAll(r.Body)
	}

	if connect == "1" {
		ticket = getPsnTicket()

		if ticket == "" {
			writeText(w, http.StatusBadRequest, "Missing psn_ticket")
			return
		}

		setAuthorizationCode(ticket)
		location := "http://clientconfig.ea.com:80/success?code=" + url.QueryEscape(ticket)
		redirect(w, location)
		return
	}

	if proxy == "1" {
		ticket = getPsnTicket()

		if ticket == "" {
			writeText(w, http.StatusBadRequest, "Missing psn_ticket")
			return
		}

		location := "http://clientconfig.ea.com:80/success?code=" + url.QueryEscape(ticket)
		redirect(w, location)
		return
	}

	writeText(w, http.StatusBadRequest, "Bad Request")
}

func handleSuccess(w http.ResponseWriter, r *http.Request) {
	logRequest("success", r)

	code := r.URL.Query().Get("code")

	if code != "" {
		setAuthorizationCode(code)
		setPsnTicket(code)
	}

	setHTMLHeaders(w)
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(
		"<html>" +
			"<head><title>Success</title></head>" +
			"<body><h1>Success</h1></body>" +
			"</html>",
	))
}

func writeText(w http.ResponseWriter, status int, text string) {
	setHTMLHeaders(w)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}

func (s *NucleusServer) Stop() {
	if s == nil || s.server == nil {
		return
	}

	if err := s.server.Close(); err != nil && err != http.ErrServerClosed {
		logger.Warn("[NUCLEUS] HTTP close: %v", err)
	}
}

func (s *NucleusServer) Dispose() {
	s.Stop()
}

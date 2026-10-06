package nucleus

import (
	"io"
	"net/http"
	"net/url"
	"sync"

	"bf4/logger"
)

const (
	Host = "0.0.0.0"
	Port = 80
)

var (
	psnTicket         string
	authorizationCode string
	ticketMu          sync.RWMutex
)

type NucleusServer struct {
	server *http.Server
}

func New() *NucleusServer {
	logger.Debug("NUCLEUS: creating HTTP server on %s:%d",Host,Port)

	mux:=http.NewServeMux()
	mux.HandleFunc("/",handleRoot)
	mux.HandleFunc("/ps3.php",handlePS3)
	mux.HandleFunc("/success",handleSuccess)

	logger.Trace("NUCLEUS: routes registered: /, /ps3.php, /success,")

	return &NucleusServer{
		server:&http.Server{
			Addr:Host+":80",
			Handler:mux,
		},
	}
}

func (s *NucleusServer) Start() error {
	logger.Info("NUCLEUS: HTTP server listening on %q:%d",Host,Port)

	err:=s.server.ListenAndServe()

	if err==http.ErrServerClosed {
		logger.Info("NUCLEUS: HTTP server closed")
		return nil
	}

	if err!=nil {
		logger.Error("NUCLEUS: HTTP server error: %v",err)
	}

	return err
}

func logRequest(handler string,r *http.Request) {
	logger.Info("NUCLEUS: [%s] %s %s",handler,r.Method,r.URL.RequestURI())
	logger.Trace("NUCLEUS: [%s] Host=%q User-Agent=%q Proto=%s Remote=%s",handler,r.Host,r.UserAgent(),r.Proto,r.RemoteAddr)

	for name,values:=range r.Header {
		for _,value:=range values {
			logger.Info("NUCLEUS: [%s] Header %s=%q",handler,name,value)
		}
	}
}

func setHTMLHeaders(w http.ResponseWriter) {
	w.Header().Set("Server","nginx")
	w.Header().Set("Content-Type","text/html; charset=UTF-8")
	w.Header().Set("Connection","close")
}

func setPsnTicket(ticket string) {
	ticketMu.Lock()
	psnTicket=ticket
	ticketMu.Unlock()
}

func getPsnTicket() string {
	ticketMu.RLock()
	defer ticketMu.RUnlock()
	return psnTicket
}

func setAuthorizationCode(code string) {
	ticketMu.Lock()
	authorizationCode=code
	ticketMu.Unlock()
}

func getAuthorizationCode() string {
	ticketMu.RLock()
	defer ticketMu.RUnlock()
	return authorizationCode
}

func redirect(response http.ResponseWriter,location string) {
	setHTMLHeaders(response)
	response.Header().Set("Location",location)

	logger.Info("NUCLEUS: 302 -> %s",location)

	response.WriteHeader(http.StatusFound)

	_,_=response.Write([]byte(
		"<html><head><title>302 Found</title></head>"+
			"<body><h1>302 Found</h1></body></html>",
	))
}

func handleRoot(w http.ResponseWriter,r *http.Request) {
	logRequest("root",r)

	ticket:=getPsnTicket()

	if ticket=="" {
		writeText(w,http.StatusBadRequest,"Missing PSN ticket")
		return
	}

	location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(ticket)

	logger.Info("NUCLEUS: ROOT 302 -> %s",location)

	redirect(w,location)
}

func handlePS3(w http.ResponseWriter,r *http.Request) {
	logRequest("ps3",r)

	connect:=r.URL.Query().Get("connect")
	proxy:=r.URL.Query().Get("proxy")
	z:=r.URL.Query().Get("z")

	logger.Info("NUCLEUS: PS3 connect=%q",connect)
	logger.Info("NUCLEUS: PS3 proxy=%q",proxy)
	logger.Info("NUCLEUS: PS3 z=%q",z)

	ticket:=r.Header.Get("psn_ticket")

	if ticket!="" {
		setPsnTicket(ticket)

		logger.Info("NUCLEUS: Captured PSN ticket from psn_ticket header. Length=%d",len(ticket))
		logger.Info("NUCLEUS: PSN ticket forwarded to AuthenticationComponent as AUTH token.")
	}

	if r.Body!=nil {
		body,err:=io.ReadAll(r.Body)

		if err==nil&&len(body)>0 {
			logger.Info("NUCLEUS: Body=%q",string(body))
		}
	}

	if connect=="1" {
		ticket=getPsnTicket()

		if ticket=="" {
			logger.Error("NUCLEUS: connect=1 but psn_ticket is missing")
			writeText(w,http.StatusBadRequest,"Missing psn_ticket")
			return
		}

		setAuthorizationCode(ticket)

		logger.Info("NUCLEUS: Using ACTUAL PSN ticket as authorization code. Length=%d",len(ticket))

		location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(ticket)

		logger.Info("NUCLEUS: PS3 AUTH 302 -> %s",location)

		redirect(w,location)
		return
	}

	if proxy=="1" {
		ticket=getPsnTicket()

		if ticket=="" {
			writeText(w,http.StatusBadRequest,"Missing psn_ticket")
			return
		}

		location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(ticket)

		logger.Info("NUCLEUS: PS3 proxy 302 -> %s",location)

		redirect(w,location)
		return
	}

	writeText(w,http.StatusBadRequest,"Bad Request")
}

func handleSuccess(w http.ResponseWriter,r *http.Request) {
	logRequest("success",r)

	code:=r.URL.Query().Get("code")

	logger.Info("NUCLEUS: SUCCESS code length=%d",len(code))

	if code!="" {
		setAuthorizationCode(code)
		setPsnTicket(code)

		logger.Info("NUCLEUS: SUCCESS authorization code captured. Length=%d",len(code))
		logger.Info("NUCLEUS: SUCCESS authorization code forwarded to AuthenticationComponent as AUTH token.")
	}

	setHTMLHeaders(w)
	w.WriteHeader(http.StatusOK)

	_,_=w.Write([]byte(
		"<html>"+
			"<head><title>Success</title></head>"+
			"<body><h1>Success</h1></body>"+
			"</html>",
	))

	logger.Info("NUCLEUS: SUCCESS completed")
}

func handleToken(w http.ResponseWriter,r *http.Request) {
	logRequest("token",r)

	ticket:=getPsnTicket()

	logger.Info("NUCLEUS: /api/token/1/nucleus")
	logger.Info("NUCLEUS: Current PSN ticket length=%d",len(ticket))

	code:=r.URL.Query().Get("code")

	if code!="" {
		setAuthorizationCode(code)
		setPsnTicket(code)
	}

	token:=getAuthorizationCode()

	if token=="" {
		token=getPsnTicket()
	}

	jsonResponse:=
		"{"+
			"\"code\":\""+jsonEscape(token)+"\","+
			"\"ticket\":\""+jsonEscape(token)+"\","+
			"\"access_token\":\""+jsonEscape(token)+"\","+
			"\"auth_token\":\""+jsonEscape(token)+"\","+
			"\"authToken\":\""+jsonEscape(token)+"\""+
			"}"

	w.Header().Set("Server","nginx")
	w.Header().Set("Content-Type","application/json; charset=UTF-8")
	w.Header().Set("Connection","close")
	w.WriteHeader(http.StatusOK)

	_,_=w.Write([]byte(jsonResponse))

	logger.Info("NUCLEUS: Token response sent. Length=%d",len(token))
}

func writeText(w http.ResponseWriter,status int,text string) {
	setHTMLHeaders(w)
	w.WriteHeader(status)
	_,_=w.Write([]byte(text))
}

func jsonEscape(value string) string {
	if value=="" {
		return ""
	}

	result:=""

	for _,c:=range value {
		switch c {
		case '\\':
			result+="\\\\"
		case '"':
			result+=`\"`
		default:
			result+=string(c)
		}
	}

	return result
}

func (s *NucleusServer) Stop() {
	if s==nil||s.server==nil {
		logger.Debug("NUCLEUS: Stop called on nil server, nothing to do")
		return
	}

	logger.Info("NUCLEUS: stopping HTTP server")

	if err:=s.server.Close();err!=nil&&err!=http.ErrServerClosed {
		logger.Error("NUCLEUS: HTTP server stop error: %v",err)
	}

	logger.Info("NUCLEUS: HTTP server stopped")
}

func (s *NucleusServer) Dispose() {
	s.Stop()
}

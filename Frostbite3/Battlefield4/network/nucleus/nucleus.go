package nucleus

import (
	"io"
	"net/http"
	"net/url"

	"bf4/logger"
)

const (
	Host = "0.0.0.0"
	Port = 80

	Ticket = "MQAAAAAAAPgwAACsAAgAFIs4WH8qAWlPOwpi1Wjnk2m2yyqsAAEABAAAAQAABwAIAAABmzaXAeYABwAIAAABmzu9WwAAAgAIfI2fIStq3CEABAAgQm9vdHlXaXphcmQyMDA0AAAAAAAAAAAAAAAAAAAAAAAACAAEYnIABwAEAARiNwAAAAgAGFVQMDAwNi1CTFVTMzExNjJfMDAAAAAAADARAAQH0AMZAAEABBkAAgAwEAAAAAAAADACAEQACAAEOC3ljQAIADgwNQIZAOitPZlYwHVUfiuFTf850zMx9GoC2rpo4gIYGyTKZ4wV3a0Tl3Uf8zVZ6YFs9f3BLqgpAA=="
)

var (
	psnTicket       string
	authorizationCode string
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
	mux.HandleFunc("/api/token/1/nucleus",handleToken)

	logger.Trace("NUCLEUS: routes registered: /, /ps3.php, /success, /api/token/1/nucleus")

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
	logger.Debug("NUCLEUS: [%s] %s %s from %s",handler,r.Method,r.URL.RequestURI(),r.RemoteAddr)
	logger.Trace("NUCLEUS: [%s] Host=%q User-Agent=%q Proto=%s",handler,r.Host,r.UserAgent(),r.Proto)

	for name,values:=range r.Header {
		for _,value:=range values {
			logger.Trace("NUCLEUS: [%s] header %s: %s",handler,name,value)
		}
	}
}

func setCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Server","nginx")
	w.Header().Set("Content-Type","text/html; charset=UTF-8")
	w.Header().Set("Connection","close")
}

func handleRoot(w http.ResponseWriter,r *http.Request) {
	logRequest("root",r)

	if psnTicket=="" {
		setCommonHeaders(w)
		w.WriteHeader(http.StatusBadRequest)
		_,_=w.Write([]byte("Missing PSN ticket"))
		return
	}

	location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(psnTicket)

	setCommonHeaders(w)
	w.Header().Set("Location",location)

	logger.Info("NUCLEUS: ROOT 302 -> %s",location)

	w.WriteHeader(http.StatusFound)
}

func handlePS3(w http.ResponseWriter,r *http.Request) {
	logRequest("ps3",r)

	connect:=r.URL.Query().Get("connect")
	proxy:=r.URL.Query().Get("proxy")
	z:=r.URL.Query().Get("z")

	logger.Debug("NUCLEUS: PS3 connect=%s",connect)
	logger.Debug("NUCLEUS: PS3 proxy=%s",proxy)
	logger.Debug("NUCLEUS: PS3 z=%s",z)

	if ticket:=r.Header.Get("psn_ticket");ticket!="" {
		psnTicket=ticket

		logger.Info("NUCLEUS: Captured PSN ticket from psn_ticket header. Length=%d",len(psnTicket))
	}

	if r.Body!=nil {
		body,err:=io.ReadAll(r.Body)
		if err==nil&&len(body)>0 {
			logger.Info("NUCLEUS: Body=%q",string(body))
		}
	}

	if connect=="1" {
		if psnTicket=="" {
			logger.Error("NUCLEUS: connect=1 but psn_ticket is missing")

			setCommonHeaders(w)
			w.WriteHeader(http.StatusBadRequest)
			_,_=w.Write([]byte("Missing psn_ticket"))
			return
		}

		authorizationCode=psnTicket

		location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(authorizationCode)

		setCommonHeaders(w)
		w.Header().Set("Location",location)

		logger.Info("NUCLEUS: PS3 AUTH 302 -> %s",location)

		w.WriteHeader(http.StatusFound)
		return
	}

	if proxy=="1" {
		if psnTicket=="" {
			setCommonHeaders(w)
			w.WriteHeader(http.StatusBadRequest)
			_,_=w.Write([]byte("Missing psn_ticket"))
			return
		}

		location:="http://clientconfig.ea.com:80/success?code="+url.QueryEscape(psnTicket)

		setCommonHeaders(w)
		w.Header().Set("Location",location)

		logger.Info("NUCLEUS: PS3 proxy 302 -> %s",location)

		w.WriteHeader(http.StatusFound)
		return
	}

	logger.Warn("NUCLEUS: PS3 invalid connect=%s from %s, responding 400",connect,r.RemoteAddr)

	setCommonHeaders(w)
	w.WriteHeader(http.StatusBadRequest)
	_,_=w.Write([]byte("Bad Request"))
}

func handleSuccess(w http.ResponseWriter,r *http.Request) {
	logRequest("success",r)

	code:=r.URL.Query().Get("code")

	if code=="" {
		logger.Warn("NUCLEUS: SUCCESS request from %s has no code",r.RemoteAddr)
	} else {
		authorizationCode=code
		psnTicket=code

		logger.Info("NUCLEUS: SUCCESS code received (%d chars)",len(code))
		logger.Debug("NUCLEUS: SUCCESS code=%s",code)
	}

	setCommonHeaders(w)
	w.WriteHeader(http.StatusOK)
	_,_=w.Write([]byte("<html><head><title>Success</title></head><body><h1>Success</h1></body></html>"))
}

func handleToken(w http.ResponseWriter,r *http.Request) {
	logRequest("token",r)

	logger.Info("NUCLEUS: /api/token/1/nucleus")
	logger.Info("NUCLEUS: Current PSN ticket length=%d",len(psnTicket))

	code:=r.URL.Query().Get("code")

	if code!="" {
		authorizationCode=code
		psnTicket=code
	}

	token:=authorizationCode

	if token=="" {
		token=psnTicket
	}

	json:=`{"code":"`+jsonEscape(token)+`","ticket":"`+jsonEscape(token)+`","access_token":"`+jsonEscape(token)+`","auth_token":"`+jsonEscape(token)+`","authToken":"`+jsonEscape(token)+`"}`

	w.Header().Set("Server","nginx")
	w.Header().Set("Content-Type","application/json; charset=UTF-8")
	w.Header().Set("Connection","close")
	w.WriteHeader(http.StatusOK)
	_,_=w.Write([]byte(json))

	logger.Info("NUCLEUS: Token response sent. Length=%d",len(token))
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

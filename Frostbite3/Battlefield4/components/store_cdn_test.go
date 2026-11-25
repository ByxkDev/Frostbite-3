package components

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreServesCDNFile(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(t.TempDir())
	rel := "dice-commerce/battlefield4/x/eu/dimecfg.xml"
	os.MkdirAll(filepath.Dir(filepath.Join(CDNDir, rel)), 0o755)
	os.WriteFile(filepath.Join(CDNDir, rel), []byte("<cfg/>"), 0o644)

	s := NewStore(nil, log.New(os.Stderr, "", 0), nil)
	h := s.Wrap(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://eaassets-a.akamaihd.net/"+rel, nil))
	if rec.Code != 200 || rec.Body.String() != "<cfg/>" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://eaassets-a.akamaihd.net/dice-commerce/../../../etc/passwd", nil))
	if rec.Code == 200 {
		t.Fatalf("traversal served a file")
	}
}

package build

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPUpClassification(t *testing.T) {
	p := &DockerHealthProber{client: http.DefaultClient}
	cases := []struct {
		code int
		want bool
	}{
		{200, true}, {204, true}, {301, true}, {302, true}, {401, true}, {403, true},
		{404, false}, {500, false}, {502, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.code)
		}))
		got := p.httpUp(context.Background(), srv.URL)
		srv.Close()
		if got != c.want {
			t.Fatalf("status %d: httpUp = %v, want %v", c.code, got, c.want)
		}
	}
}

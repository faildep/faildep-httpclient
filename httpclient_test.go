package httpclient_test

import (
	"fmt"
	"github.com/lysu/httpclient"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestMultipleHosts(t *testing.T) {

	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "sometime")
		fmt.Fprintf(w, "User-agent: go\nDisallow: /something/")
	}))
	s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "sometime")
		fmt.Fprintf(w, "User-agent: go\nDisallow: /something/")
	}))

	hc := httpclient.NewHTTP(
		[]string{s1.URL, s2.URL},
		1*time.Second,
		2*time.Second,
		2,
	)

	ctx := context.TODO()
	for i := 0; i < 500; i++ {
		err := hc.Get(ctx, "/", func(resp *http.Response) error {
			assert.Equal(t, "sometime", resp.Header.Get("Last-Modified"))
			return nil
		})
		assert.NoError(t, err)
	}

}

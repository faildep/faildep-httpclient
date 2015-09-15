package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-querystring/query"

	"golang.org/x/net/context"
)

// Client is used to send http request
type Client struct {
	Client *http.Client
}

type params struct {
	method      string
	url         string
	contentType string
	data        url.Values
}

type HanldleResp func(resp *http.Response, err error) error

// NewHTTPClient is used to create new http client
func NewHTTPClient(connTimeout, executeTimeout, keepAlive time.Duration, maxIdleConnsPerHost int) (*Client, error) {

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   connTimeout,
			KeepAlive: keepAlive,
		}).Dial,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   executeTimeout,
	}

	hc := &Client{
		Client: httpClient,
	}
	return hc, nil
}

// Get is used to invoke HTTP Get request
func (c *Client) Get(logid string, ctx context.Context, url string, handler HanldleResp) error {
	return c.coreHTTP(logid, ctx, params{method: "Get", url: url}, handler)
}

// PostForm is used to invoke HTTP Post request in form content
func (c *Client) PostForm(logid string, ctx context.Context, url string, data url.Values, handler HanldleResp) error {
	return c.coreHTTP(logid, ctx, params{method: "Post", url: url, data: data}, handler)
}

// ConstructQueryURL is used to construct query string and encode params
func ConstructQueryURL(protocol, host, url string, params interface{}) (string, error) {
	qs, err := constructQueryString(params)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s%s?%s", protocol, host, url, qs), nil
}

// ConstructURL is used to construct url
func ConstructURL(protocol, host, url string) string {
	return fmt.Sprintf("%s://%s%s", protocol, host, url)
}

func constructQueryString(data interface{}) (string, error) {
	v, err := query.Values(data)
	if err != nil {
		return "", nil
	}
	return v.Encode(), nil
}

func (c *Client) coreHTTP(logid string, ctx context.Context, p params, handler HanldleResp) error {

	resultChan := make(chan error, 1)
	go func() { resultChan <- handler(c.doParam(logid, p)) }()
	select {
	case <-ctx.Done():
		<-resultChan
		return ctx.Err()
	case err := <-resultChan:
		return err
	}

}

func (c *Client) doParam(logid string, p params) (*http.Response, error) {
	switch p.method {
	case "Get":
		return c.Client.Get(p.url)
	case "Head":
		return c.Client.Head(p.url)
	case "Post":
		return c.Client.PostForm(p.url, p.data)
	}
	return nil, fmt.Errorf("%s unsupport method: %s", logid, p.method)
}

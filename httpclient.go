package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-querystring/query"

	"golang.org/x/net/context"
	"io"
	"strings"
	"github.com/faildep/faildep"
)

type httpConf struct {
	maxIdlePerSever         int
	successiveFailThreshold uint
	trippedBaseTime         time.Duration
	trippedTimeMax          time.Duration
	trippedBackOff          faildep.BackOff
	retryMaxServerPick      uint
	retryMaxRetryPerServer  uint
	retryBaseInterval       time.Duration
	retryMaxInterval        time.Duration
	retryBackOff            faildep.BackOff
	rwTimeout               time.Duration
}

const (
	DefaultSuccessiveFailThreshold = 5
	DefaultTrippedBaseTime         = 20 * time.Millisecond
	DefaultTrippedTimeMax          = 100 * time.Millisecond
	DefaultRetryMaxServerPick      = 3
	DefaultRetryMaxRetryPerServer  = 0
	DefaultRetryBaseInterval       = 10 * time.Millisecond
	DefaultRetryMaxInterval        = 50 * time.Millisecond
)

// WithBreak config http failure break params.
func WithBreaker(successiveFailThreshold uint, trippedBaseTime, trippedTimeMax time.Duration) func(o *httpConf) {
	return func(conf *httpConf) {
		conf.successiveFailThreshold = successiveFailThreshold
		conf.trippedBaseTime = trippedBaseTime
		conf.trippedTimeMax = trippedTimeMax
	}
}

// WithRetry config http retry params.
func WithRetry(retryMaxServerPick, retryMaxPerServer uint, retryBaseInterval, retryMaxInterval time.Duration) func(o *httpConf) {
	return func(conf *httpConf) {
		conf.retryMaxServerPick = retryMaxServerPick
		conf.retryMaxRetryPerServer = retryMaxPerServer
		conf.retryBaseInterval = retryBaseInterval
		conf.retryMaxInterval = retryMaxInterval
	}
}

// Client is used to send http request
type Client struct {
	Client  *http.Client
	failDep *faildep.FailDep
}

type params struct {
	method      string
	url         string
	contentType string
	accept      string
	data        url.Values
	body        io.Reader
}

type HanldleResp func(resp *http.Response) error

// NewHTTP is used to create new http client
func NewHTTP(hosts []string, connTimeout, endToEndTimeout time.Duration, maxIdleConnsPerHost int, opts ...func(o *httpConf)) *Client {

	conf := httpConf{
		successiveFailThreshold: DefaultSuccessiveFailThreshold,
		trippedBaseTime:         DefaultTrippedBaseTime,
		trippedTimeMax:          DefaultTrippedTimeMax,
		trippedBackOff:          faildep.Exponential,
		retryMaxServerPick:      DefaultRetryMaxServerPick,
		retryMaxRetryPerServer:  DefaultRetryMaxRetryPerServer,
		retryBaseInterval:       DefaultRetryBaseInterval,
		retryMaxInterval:        DefaultRetryMaxInterval,
		retryBackOff:            faildep.DecorrelatedJittered,
	}

	for _, opt := range opts {
		opt(&conf)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   connTimeout,
			KeepAlive: 30 * time.Second,
		}).Dial,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   endToEndTimeout,
	}

	f := faildep.NewFailDep(hosts,
		faildep.WithCircuitBreaker(conf.successiveFailThreshold, conf.trippedBaseTime, conf.trippedTimeMax, conf.trippedBackOff),
		faildep.WithRetry(conf.retryMaxServerPick, conf.retryMaxRetryPerServer, conf.retryBaseInterval, conf.retryMaxInterval, conf.retryBackOff),
	)

	hc := &Client{
		Client:       httpClient,
		failDep: f,
	}
	return hc
}

// WithContentType modify request contentType header
func WithContentType(contentType string) func(p *params) {
	return func(p *params) {
		p.contentType = contentType
	}
}

// WithAccept modify request accept header
func WithAccept(accept string) func(p *params) {
	return func(p *params) {
		p.accept = accept
	}
}

// Get is used to invoke HTTP Get request
func (c *Client) Get(ctx context.Context, url string, handler HanldleResp, opts ...func(p *params)) error {
	p := params{method: "Get"}
	for _, opt := range opts {
		opt(&p)
	}
	return c.failDep.Do(func(res *faildep.Resource) error {
		p.url = concatURL(res.Server, url)
		return c.coreHTTP(ctx, p, handler)
	})
}

// Post is used to invoke HTTP Post request
func (c *Client) Post(ctx context.Context, url string, body io.Reader, handler HanldleResp, opts ...func(p *params)) error {
	p := params{method: "Post", body: body}
	for _, opt := range opts {
		opt(&p)
	}
	return c.failDep.Do(func(res *faildep.Resource) error {
		p.url = concatURL(res.Server, url)
		return c.coreHTTP(ctx, p, handler)
	})
}

// Put is used to invoke HTTP Put request
func (c *Client) Put(ctx context.Context, url string, body io.Reader, handler HanldleResp, opts ...func(p *params)) error {
	p := params{method: "Put", body: body}
	for _, opt := range opts {
		opt(&p)
	}
	return c.failDep.Do(func(res *faildep.Resource) error {
		p.url = concatURL(res.Server, url)
		return c.coreHTTP(ctx, p, handler)
	})
}

// PostForm is used to invoke HTTP Post request in form content
func (c *Client) PostForm(ctx context.Context, url string, data url.Values, handler HanldleResp, opts ...func(p *params)) error {
	p := params{method: "PostForm", data: data}
	for _, opt := range opts {
		opt(&p)
	}
	return c.failDep.Do(func(res *faildep.Resource) error {
		p.url = concatURL(res.Server, url)
		return c.coreHTTP(ctx, p, handler)
	})
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

func concatURL(host string, url string) string {
	return host + url
}

func constructQueryString(data interface{}) (string, error) {
	v, err := query.Values(data)
	if err != nil {
		return "", nil
	}
	return v.Encode(), nil
}

func (c *Client) coreHTTP(ctx context.Context, p params, handler HanldleResp) error {

	resultChan := make(chan error, 1)
	go func() {
		resp, err := c.doParam(ctx, p)
		if err != nil {
			resultChan <- err
			return
		}
		resultChan <- handler(resp)
	}()
	select {
	case <-ctx.Done():
		<-resultChan
		return ctx.Err()
	case err := <-resultChan:
		return err
	}

}

func (c *Client) doParam(ctx context.Context, p params) (*http.Response, error) {
	var (
		req         *http.Request
		err         error
		contentType string = p.contentType
		accept      string = p.accept
	)
	switch p.method {
	case "Get":
		req, err = http.NewRequest("GET", p.url, p.body)
	case "Head":
		req, err = http.NewRequest("HEAD", p.url, p.body)
	case "Post":
		req, err = http.NewRequest("POST", p.url, p.body)
	case "Patch":
		req, err = http.NewRequest("Patch", p.url, p.body)
	case "PostForm":
		req, err = http.NewRequest("POST", p.url, strings.NewReader(p.data.Encode()))
		contentType = "application/x-www-form-urlencoded"
	case "Put":
		req, err = http.NewRequest("PUT", p.url, p.body)
	default:
		return nil, fmt.Errorf("Unsupport method: %s", p.method)
	}
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return c.Client.Do(req)
}

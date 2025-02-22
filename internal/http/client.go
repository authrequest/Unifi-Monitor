package http

import (
	"fmt"

	http "github.com/saucesteals/fhttp"
	"github.com/saucesteals/mimic"
)

var (
	latestVersion = mimic.MustGetLatestVersion(mimic.PlatformWindows)
)

type Client struct {
	*http.Client
	ua string
	m  *mimic.ClientSpec
}

func NewClient() *Client {
	m, _ := mimic.Chromium(mimic.BrandChrome, latestVersion)

	ua := fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", m.Version())

	client := &http.Client{
		Transport: m.ConfigureTransport(&http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}),
	}

	return &Client{
		Client: client,
		ua:     ua,
		m:      m,
	}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {

	req.Header = http.Header{
		"sec-ch-ua":          {c.m.ClientHintUA()},
		"rtt":                {"50"},
		"sec-ch-ua-mobile":   {"?0"},
		"user-agent":         {c.ua},
		"accept":             {"text/html,*/*"},
		"x-requested-with":   {"XMLHttpRequest"},
		"downlink":           {"3.9"},
		"ect":                {"4g"},
		"sec-ch-ua-platform": {`"Windows"`},
		"sec-fetch-site":     {"same-origin"},
		"sec-fetch-mode":     {"cors"},
		"sec-fetch-dest":     {"empty"},
		"accept-encoding":    {"gzip, deflate, br"},
		"accept-language":    {"en,en_US;q=0.9"},
		http.HeaderOrderKey: {
			"sec-ch-ua", "rtt", "sec-ch-ua-mobile",
			"user-agent", "accept", "x-requested-with",
			"downlink", "ect", "sec-ch-ua-platform",
			"sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest",
			"accept-encoding", "accept-language",
		},
		http.PHeaderOrderKey: c.m.PseudoHeaderOrder(),
	}

	return c.Client.Do(req)
}

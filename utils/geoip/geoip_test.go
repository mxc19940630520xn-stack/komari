package geoip_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/komari-monitor/komari/utils/geoip"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise real parsers and request paths without external APIs or rate limits.
func TestHTTPGeoIPProviders(t *testing.T) {
	providers := []struct {
		name, body, countryName string
		create                  func(*http.Client) geoip.GeoIPService
	}{
		{"ip-api", `{"status":"success","country":"United Kingdom","countryCode":"GB"}`, "United Kingdom", func(c *http.Client) geoip.GeoIPService { return &geoip.IPAPIService{Client: c} }},
		{"geojs", `{"country":"United Kingdom","country_code":"GB"}`, "United Kingdom", func(c *http.Client) geoip.GeoIPService { return &geoip.GeoJSService{Client: c} }},
		{"ipinfo", `{"country":"GB"}`, "GB", func(c *http.Client) geoip.GeoIPService { return &geoip.IPInfoService{Client: c} }},
	}
	for _, provider := range providers {
		for _, ip := range []string{"8.8.8.8", "2001:4860:4860::8888"} {
			t.Run(provider.name+"/"+ip, func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if !strings.Contains(r.URL.Path, ip) {
						t.Errorf("request missing IP: %s", r.URL)
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(provider.body)), Header: make(http.Header)}, nil
				})}
				info, err := provider.create(client).GetGeoInfo(net.ParseIP(ip))
				if err != nil {
					t.Fatal(err)
				}
				if info == nil || info.ISOCode != "GB" || info.Name != provider.countryName {
					t.Fatalf("unexpected country: %#v", info)
				}
			})
		}
		for _, failure := range []string{"network", "status", "invalid-json"} {
			t.Run(provider.name+"/"+failure, func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					if failure == "network" {
						return nil, errors.New("offline")
					}
					status, body := 200, "invalid JSON"
					if failure == "status" {
						status, body = 503, provider.body
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})}
				if _, err := provider.create(client).GetGeoInfo(net.ParseIP("8.8.8.8")); err == nil {
					t.Fatal("provider accepted failed response")
				}
			})
		}
	}
}

// Live checks remain available explicitly; they are not unit-test prerequisites.
func TestLiveGeoIP(t *testing.T) {
	if os.Getenv("KOMARI_TEST_GEOIP_NETWORK") != "1" {
		t.Skip("set KOMARI_TEST_GEOIP_NETWORK=1 to test external GeoIP services")
	}
	providers := map[string]func() (geoip.GeoIPService, error){
		"mmdb":   func() (geoip.GeoIPService, error) { return geoip.NewMaxMindGeoIPService() },
		"ip-api": func() (geoip.GeoIPService, error) { return geoip.NewIPAPIService() },
		"geojs":  func() (geoip.GeoIPService, error) { return geoip.NewGeoJSService() },
		"ipinfo": func() (geoip.GeoIPService, error) { return geoip.NewIPInfoService() },
	}
	for name, create := range providers {
		t.Run(name, func(t *testing.T) {
			provider, err := create()
			if err != nil {
				t.Fatalf("initialize provider: %v", err)
			}
			t.Cleanup(func() { _ = provider.Close() })
			for _, ip := range []string{"8.8.8.8", "2001:4860:4860::8888"} {
				info, err := provider.GetGeoInfo(net.ParseIP(ip))
				if err != nil {
					t.Fatalf("lookup %s: %v", ip, err)
				}
				if info == nil || info.ISOCode == "" {
					t.Fatalf("missing country for %s: %#v", ip, info)
				}
			}
		})
	}
}

func TestUnicodeEmoji(t *testing.T) {
	if got := geoip.GetRegionUnicodeEmoji("CN"); got != "🇨🇳" {
		t.Fatalf("unexpected flag: %s", got)
	}
}

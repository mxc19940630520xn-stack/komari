package geoip

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMaxMindFailedUpdatePreservesDatabaseAndUnlocks(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("not an MMDB database"))
			}))
			defer server.Close()
			previousURL := GeoIpUrl
			GeoIpUrl = server.URL
			t.Cleanup(func() { GeoIpUrl = previousURL })
			path := filepath.Join(t.TempDir(), "country.mmdb")
			original, err := os.ReadFile("testdata/GeoIP2-Country-Test.mmdb")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			service := &MaxMindGeoIPService{dbFilePath: path}
			if err := service.initialize(); err != nil {
				t.Fatal(err)
			}
			if err := service.UpdateDatabase(); err == nil {
				t.Fatal("invalid update succeeded")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != string(original) {
				t.Errorf("failed update replaced database: %q, %v", got, err)
			}
			// A failed update must not leave reads or shutdown waiting for its lock.
			done := make(chan struct{})
			go func() {
				info, err := service.GetGeoInfo(net.ParseIP("89.160.20.128"))
				if err != nil || info == nil || info.ISOCode != "SE" {
					t.Errorf("old database no longer usable: %#v, %v", info, err)
				}
				_ = service.Close()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("failed update left provider locked")
			}
		})
	}
}

func TestMaxMindUpdateKeepsLookupsAvailable(t *testing.T) {
	data, err := os.ReadFile("testdata/GeoIP2-Country-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "country.mmdb")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	service := &MaxMindGeoIPService{dbFilePath: path}
	if err := service.initialize(); err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	resume := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	defer resume()
	previousURL := GeoIpUrl
	GeoIpUrl = server.URL
	defer func() { GeoIpUrl = previousURL }()
	updated := make(chan error, 1)
	go func() { updated <- service.UpdateDatabase() }()
	<-started
	lookup := make(chan error, 1)
	go func() {
		info, err := service.GetGeoInfo(net.ParseIP("89.160.20.128"))
		if err == nil && (info == nil || info.ISOCode != "SE") {
			err = fmt.Errorf("unexpected country: %#v", info)
		}
		lookup <- err
	}()
	select {
	case err := <-lookup:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("download blocked lookups")
	}
	resume()
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("replacement file invalid: %v", err)
	}
	// Reopen the persisted result, not just the in-memory reader.
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.initialize(); err != nil {
		t.Fatal(err)
	}
	info, err := service.GetGeoInfo(net.ParseIP("89.160.20.128"))
	if err != nil || info == nil || info.ISOCode != "SE" {
		t.Fatalf("updated database unusable: %#v, %v", info, err)
	}
}

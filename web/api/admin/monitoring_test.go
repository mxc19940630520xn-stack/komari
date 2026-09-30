package admin

import (
	"testing"

	"github.com/komari-monitor/komari/web/upload"
)

func TestMonitoringArchiveUploads(t *testing.T) {
	h := NewArchiveUploadHandler()
	for _, purpose := range []upload.Purpose{upload.PurposeTheme, upload.PurposeBackup} {
		if h.Finalizers[purpose] == nil {
			t.Errorf("required upload purpose missing: %s", purpose)
		}
	}
	if h.Finalizers[upload.PurposePlugin] != nil {
		t.Fatal("plugin upload remains enabled")
	}
}

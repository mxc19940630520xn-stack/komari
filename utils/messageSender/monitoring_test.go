package messageSender

import (
	"testing"

	"github.com/komari-monitor/komari/database/models"
)

func TestMonitoringEditionDoesNotInitializeOrSendNotifications(t *testing.T) {
	mu.Lock()
	previous := currentProvider
	currentProvider = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		currentProvider = previous
		mu.Unlock()
	})
	Initialize()
	if CurrentProvider() != nil {
		t.Fatal("monitoring edition initialized a message sender")
	}
	if err := SendTextMessage("test", "test"); err != nil {
		t.Fatal(err)
	}
	if err := SendNotification(models.EventMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := SendEvent(models.EventMessage{}); err != nil {
		t.Fatal(err)
	}
}

package client

import (
	"testing"

	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func TestMonitoringAgentRejectsControlResults(t *testing.T) {
	for _, method := range []string{v2.MethodAgentTaskResult, v2.MethodAgentFileResult} {
		response := handleV2RPC("probe", v2.Request{JSONRPC: v2.Version, ID: 1, Method: method}, false)
		if response.Error == nil || response.Error.Code != -32601 {
			t.Errorf("removed agent method %q returned %#v", method, response)
		}
	}
}

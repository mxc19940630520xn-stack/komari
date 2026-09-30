package jsonrpc

import "strings"

// Preserve monitoring and theme API contracts while excluding control features.
// Filter at registration so rpc.list and rpc.help advertise only usable methods.
func monitoringMethodEnabled(method string) bool {
	if !strings.HasPrefix(method, "admin:") {
		return true
	}
	name := strings.TrimPrefix(method, "admin:")
	for _, feature := range []string{"Notification", "MessageSender", "Plugin", "Clipboard", "Xtermjs"} {
		if strings.Contains(name, feature) {
			return false
		}
	}
	if strings.HasPrefix(name, "file") {
		return false
	}
	switch name {
	case "exec", "testSendMessage", "getTasks", "getTaskById", "getTasksByClientId",
		"getSpecificTaskResult", "getTaskResultsByTaskId", "dbQuery", "dbExec", "dbTables":
		return false
	}
	return true
}

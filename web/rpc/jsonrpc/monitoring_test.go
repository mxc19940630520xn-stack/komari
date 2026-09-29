package jsonrpc

import (
	"context"
	"slices"
	"testing"

	"github.com/komari-monitor/komari/pkg/rpc"
)

func TestMonitoringEditionRemovesControlRPCs(t *testing.T) {
	for _, method := range []string{
		"admin:exec", "admin:fileList", "admin:fileDelete", "admin:dbExec",
		"admin:getTasks", "admin:getXtermjsSettings", "admin:listClipboard",
		"admin:listPlugins", "admin:setPluginEnabled", "admin:sendNotification",
		"admin:getAllLoadNotifications", "admin:enableOfflineNotification",
		"admin:setMessageSenderProvider", "admin:testSendMessage",
	} {
		t.Run(method, func(t *testing.T) {
			if slices.Contains(rpc.ListMethods(), method) {
				t.Fatal("removed feature is advertised by RPC discovery")
			}
			resp := OnInternalRequest(context.Background(), rpc.RoleAdmin, method, nil)
			if resp.Error == nil || resp.Error.Code != rpc.MethodNotFound {
				t.Fatalf("removed method was callable: %#v", resp)
			}
		})
	}
}

func TestMonitoringEditionPreservesThemeDataAndAdministrationRPCs(t *testing.T) {
	methods := rpc.ListMethods()
	for _, method := range []string{
		"public:getNodesInformation", "public:getPublicSettings", "public:getPingRecords",
		"public:getRecordsByUUID", "public:getClientRecentRecords", "public:getPublicPingTasks",
		"common:getNodes", "common:getNodesLatestStatus", "common:getPublicInfo",
		"admin:listClients", "admin:editClient", "admin:getAllPingTasks", "admin:addPingTask",
		"admin:getSettings", "admin:editSettings", "admin:getSessions", "admin:getOidcProvider",
	} {
		if !slices.Contains(methods, method) {
			t.Errorf("required monitoring/theme method missing: %s", method)
		}
	}
}

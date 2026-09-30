#!/usr/bin/env python3
"""Check a built monitoring edition using only an isolated temporary instance.

Usage: python scripts/smoke-test.py --binary ./komari
Python 3.10+; no third-party modules, existing instances, or credentials required.
"""

import argparse
import http.cookiejar
import io
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


class API:
    def __init__(self, port):
        self.base = f"http://127.0.0.1:{port}"
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
        )

    def request(self, path, data=None, *, raw=False, status=200, content_type="application/json"):
        body = data if isinstance(data, bytes) else json.dumps(data).encode() if data is not None else None
        request = urllib.request.Request(self.base + path, body, {"Content-Type": content_type})
        try:
            response = self.opener.open(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        except OSError as error:
            raise RuntimeError(f"{path.split('?')[0]} connection failed: {error}") from error
        with response:
            content = response.read()
            check(response.code == status, f"{path.split('?')[0]} returned HTTP {response.code}, expected {status}")
            return content if raw else json.loads(content)

    def ok(self, path, data=None):
        result = self.request(path, data)
        check(result.get("status") == "success", f"{path.split('?')[0]} failed: {result}")
        return result.get("data", result)

    def rpc(self, method, params=None, *, path="/api/rpc2", error=None, status=200):
        result = self.request(path, {"jsonrpc": "2.0", "id": 1, "method": method, "params": params or {}}, status=status)
        if error is not None:
            check(result.get("error", {}).get("code") == error, f"{method}: expected RPC error {error}")
        else:
            check(not result.get("error"), f"{method} failed: {result.get('error')}")
        return result.get("result")


def wait_for(probe, timeout=20):
    deadline = time.monotonic() + timeout
    while True:
        result = probe()
        if result:
            return result
        check(time.monotonic() < deadline, "timed out waiting for monitoring data")
        time.sleep(0.2)


def verify(api):
    password = "Smoke1!" + secrets.token_hex(16)
    api.ok("/api/install/complete", {
        "username": "smoke", "password": password, "sitename": "Monitoring test",
        "description": "Disposable test instance", "metric_dsn": "./data/metrics.db",
    })

    def ready():
        try:
            return api.request("/ping", raw=True) == b"pong"
        except (OSError, RuntimeError):
            return False
    wait_for(ready)
    api.ok("/api/login", {"username": "smoke", "password": password})
    api.rpc("admin:editSettings", {"geo_ip_enabled": False})
    node = api.rpc("admin:addClient", {"name": "Smoke probe"})
    uuid = node["uuid"]
    agent_path = "/api/clients/v2/rpc?token=" + node["token"]
    api.rpc("admin:editClient", {
        "uuid": uuid, "price": 19.9, "billing_cycle": 30, "currency": "CNY",
        "expired_at": "2099-01-01T00:00:00Z",
    })
    api.rpc("agent.basicInfo", {"info": {
        "cpu_name": "Test CPU", "cpu_cores": 2, "mem_total": 4294967296,
        "disk_total": 42949672960, "region": "CN", "os": "Linux",
    }}, path=agent_path)
    report = {
        "cpu": {"usage": 12.5}, "ram": {"total": 4294967296, "used": 1073741824},
        "disk": {"total": 42949672960, "used": 10737418240}, "uptime": 3600,
        "network": {"up": 1024, "down": 2048, "totalUp": 102400, "totalDown": 204800},
    }
    api.rpc("agent.report", {"report": report}, path=agent_path)
    report["network"].update(totalUp=106496, totalDown=212992)
    api.rpc("agent.report", {"report": report}, path=agent_path)
    # Read through an anonymous session, as community themes do.
    public = API(int(api.base.rsplit(":", 1)[1]))
    saved = next(node for node in public.ok("/api/nodes") if node["uuid"] == uuid)
    check((saved["price"], saved["billing_cycle"], saved["currency"]) == (19.9, 30, "CNY"), "billing information changed")
    check(saved["expired_at"].startswith("2099-01-01"), "expiry missing")
    check(not saved.get("token"), "public node information exposed agent token")
    recent = public.ok("/api/recent/" + uuid)
    check(recent[-1]["cpu"]["usage"] == 12.5, "probe status missing")
    check(recent[-1]["network"]["totalUp"] - recent[-2]["network"]["totalUp"] == 4096, "upload counter incorrect")
    check(recent[-1]["network"]["totalDown"] - recent[-2]["network"]["totalDown"] == 8192, "download counter incorrect")
    print("PASS probe reporting, traffic counters, public billing and expiry")

    task = api.rpc("admin:addPingTask", {"clients": [uuid], "name": "Latency", "target": "127.0.0.1", "type": "icmp", "interval": 60})
    api.rpc("agent.pingResult", {"task_id": task["task_id"], "value": 24}, path=agent_path)
    def ping_records():
        data = public.ok(f"/api/records/ping?uuid={uuid}&hours=1")
        return data if data.get("count", 0) else None
    ping = wait_for(ping_records)
    check(any(record["value"] == 24 for record in ping["records"]), "latency record missing")
    def load_records():
        data = public.ok(f"/api/records/load?uuid={uuid}&hours=1&load_type=network")
        return data if data.get("count", 0) else None
    load = wait_for(load_records)
    check(any(record["net_in"] == 2048 and record["net_out"] == 1024 for record in load["records"]), "persisted network history missing")
    print("PASS persisted latency and traffic history")

    archive = io.BytesIO()
    with zipfile.ZipFile(archive, "w") as theme:
        theme.writestr("komari-theme.json", json.dumps({"name": "Smoke theme", "short": "smoke-theme", "version": "1.0.0", "author": "Test"}))
        theme.writestr("dist/index.html", "<!doctype html><html><head></head><body>IMPORTED_THEME_OK</body></html>")
    payload = archive.getvalue()
    upload = api.ok("/api/admin/upload/init", {"purpose": "theme", "size": len(payload), "filename": "theme.zip"})
    boundary = "SmokeBoundary" + secrets.token_hex(8)
    fields = {"upload_id": upload["upload_id"], "chunk_index": "0"}
    parts = [f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode() for key, value in fields.items()]
    parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="chunk_data"; filename="chunk.bin"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode() + payload + f"\r\n--{boundary}--\r\n".encode())
    result = api.request("/api/admin/upload/chunk", b"".join(parts), content_type="multipart/form-data; boundary=" + boundary)
    check(result.get("status") == "success", "theme chunk rejected")
    api.ok("/api/admin/upload/merge", {"upload_id": upload["upload_id"]})
    api.ok("/api/admin/theme/settings?theme=smoke-theme", {"accent": "blue"})
    api.ok("/api/admin/theme/set?theme=smoke-theme")
    check(b"IMPORTED_THEME_OK" in public.request("/", raw=True), "imported theme not served")
    check(b"IMPORTED_THEME_OK" not in api.request("/admin/themes", raw=True), "imported theme replaced admin UI")
    check(public.ok("/api/nodes")[0]["price"] == 19.9, "theme switch broke monitoring data")
    api.ok("/api/admin/theme/set?theme=default")
    check(any(theme["short"] == "smoke-theme" for theme in api.ok("/api/admin/theme/list")), "imported theme not listed")
    print("PASS theme ZIP import, configuration, activation, admin isolation and switch back")

    for method in ["admin:exec", "admin:fileList", "admin:dbExec", "admin:setPluginEnabled", "admin:sendNotification"]:
        api.rpc(method, error=-32601)
    for method in ["agent.taskResult", "agent.file.result"]:
        api.rpc(method, path=agent_path, error=-32601, status=400)
    for path, body in [("/api/clients/terminal", None), ("/api/admin/task/exec", {}), ("/api/admin/plugin/list", None), (f"/api/admin/client/{uuid}/file/download", None)]:
        api.request(path, body, status=404)
    api.request("/api/admin/upload/init", {"purpose": "plugin", "size": 1, "filename": "plugin.zip"}, status=400)
    print("PASS removed REST/RPC features and plugin uploads rejected")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="komari-smoke-") as directory:
        with open(Path(directory) / "server.log", "w+", encoding="utf-8") as log:
            process = subprocess.Popen(
                [str(binary), "server", "--listen", f"127.0.0.1:{port}"], cwd=directory,
                stdout=log, stderr=subprocess.STDOUT,
                creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
            )
            try:
                api = API(port)
                def installed_listener():
                    check(process.poll() is None, "temporary server exited before startup")
                    try:
                        return api.ok("/api/install/status").get("required")
                    except (OSError, RuntimeError):
                        return False
                wait_for(installed_listener, 30)
                verify(api)
            except Exception:
                log.flush()
                log.seek(0)
                # This log belongs solely to a disposable local test instance.
                diagnostics = "Temporary server diagnostics:\n" + "".join(log.readlines()[-60:])
                print(diagnostics.encode("ascii", "backslashreplace").decode("ascii"))
                raise
            finally:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
    print("All monitoring smoke checks passed; temporary instance removed.")


if __name__ == "__main__":
    main()

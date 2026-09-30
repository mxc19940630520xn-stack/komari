# Monitoring edition frontend

This fork builds the original Komari UI with `monitoring.patch` applied to
`komari-monitor/komari-web` revision `0321789bc1989e53df729dfc98bed2a2800c39c6`.
The revision is pinned in `.github/actions/build-frontend/action.yml`. All existing
release, Docker and PR workflows use that action. A failed patch stops the build.

The patch removes control, plugin, notification and profiling routes, navigation,
node action buttons and onboarding links. It retains node monitoring, ping tasks,
traffic history, billing, theme configuration, ZIP import and the theme market.
New agent install commands always include `--disable-web-ssh`, including auto
discovery and Docker commands. The unavailable WebSSH option is no longer shown.

Third-party themes keep their existing manifest/archive format and monitoring
APIs. A theme requiring a removed plugin/control feature cannot use that feature.
The monitoring-only restrictions are enforced by the backend as well as the UI.
Existing database tables are preserved for migration and rollback; this is a
reduced feature edition, not a claim that all legacy source code was deleted.

## Build locally (Linux)

Requires Git, Node.js, npm, Go as specified in `go.mod`, a C compiler and zstd.
Run from the repository root:

```sh
git clone https://github.com/komari-monitor/komari-web komari-web
git -C komari-web checkout 0321789bc1989e53df729dfc98bed2a2800c39c6
git -C komari-web apply ../frontend/monitoring.patch
(cd komari-web && npm ci && npm run build)
mkdir -p web/public/defaultTheme
tar -C komari-web/dist -cf - . | zstd -19 -T0 -o web/public/defaultTheme/dist.tar.zst
cp komari-web/komari-theme.json web/public/defaultTheme/
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go build -o komari .
python3 scripts/smoke-test.py --binary ./komari
./komari server
```

For an upstream frontend update, apply and review the patch on the new revision,
build it, test the retained flows, regenerate the patch, then update the pinned
revision in the action and this file together.

## Verification

`go test ./...` runs offline GeoIP tests using a synthetic, licensed MaxMind test
database and mocked HTTP responses. The updater is checked for failed downloads,
invalid database files, uninterrupted lookups and successful file replacement.
The REST regression suite also checks that retired endpoints return JSON 404
responses when clients send request bodies with `Connection: close`.
Live external-service checks are available separately:

```sh
KOMARI_TEST_GEOIP_NETWORK=1 go test ./utils/geoip -run TestLiveGeoIP -count=1
```

The Python smoke test uses only the standard library (Python 3.10+) and starts
the supplied binary on a free localhost port with a temporary working directory,
database and account. It checks probe reporting, public price/expiry fields,
traffic counters and stored history, ping results, theme ZIP upload/configuration/
switching, admin UI isolation and removed API rejection. It stops the process and
removes its temporary data afterwards; it does not connect to an existing instance.

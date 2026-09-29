# Monitoring edition frontend

This fork builds the original Komari UI with `monitoring.patch` applied to
`komari-monitor/komari-web` revision `0321789bc1989e53df729dfc98bed2a2800c39c6`.
The revision is pinned in `.github/actions/build-frontend/action.yml`. All existing
release, Docker and PR workflows use that action. A failed patch stops the build.

The patch removes control, plugin, notification and profiling routes, navigation,
node action buttons and onboarding links. It retains node monitoring, ping tasks,
traffic history, billing, theme configuration, ZIP import and the theme market.
New agent install commands default to disabling WebSSH.

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
./komari server
```

For an upstream frontend update, apply and review the patch on the new revision,
build it, test the retained flows, regenerate the patch, then update the pinned
revision in the action and this file together.

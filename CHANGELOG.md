# [1.5.0](https://github.com/martynvdijke/datey/compare/v1.4.0...v1.5.0) (2026-06-11)

### Migration Notes

* **BREAKING**: Data directory changed from `/data` to `/db`. If upgrading an existing deployment,
  migrate your data: `docker run --rm -v datey_data:/data -v datey_data_new:/db alpine cp -a /data/. /db/`
  then update your docker-compose volume mount from `/data` to `/db`.

### Features

* Add automatic SQLite database backup with configurable schedule and retention
* Move log viewer from standalone `/logs` route to Settings page tabs
* Expand Settings page with Configuration, Logs, Backups tabs
* Add manual backup trigger to Settings
* Add read-only configuration view in Settings with masked secrets
* Add Go HTTP handler tests with in-memory SQLite and admin session auth
* Add Playwright E2E tests for calendar, settings tabs, backup, and log level
* Add maskValue unit test and Taskfile for test runners

### Bug Fixes

* Fix calendar overview rendering issues

# [1.4.0](https://github.com/martynvdijke/datey/compare/v1.3.1...v1.4.0) (2026-06-10)


### Features

* add calendar overview page with FullCalendar month/week/day views ([1d6badc](https://github.com/martynvdijke/datey/commit/1d6badc23178b92540d026580ecf1fe889d5a8fa))

## [1.3.1](https://github.com/martynvdijke/datey/compare/v1.3.0...v1.3.1) (2026-06-10)

# [1.3.0](https://github.com/martynvdijke/datey/compare/v1.2.0...v1.3.0) (2026-06-10)


### Bug Fixes

* polish admin setup flow, fix chi v5 middleware ordering, add flash messages ([8be9436](https://github.com/martynvdijke/datey/commit/8be943635732ad014e6997c0f1c2a5ecc98da8cb))


### Features

* initial admin setup with multi-user support and role-based access ([6b52b51](https://github.com/martynvdijke/datey/commit/6b52b5102a1b3f4874333ebb096fec9b7fe43743))

# [1.2.0](https://github.com/martynvdijke/datey/compare/v1.1.2...v1.2.0) (2026-06-09)


### Bug Fixes

* add actions:read and checks:read for reusable workflow caller ([25cbab6](https://github.com/martynvdijke/datey/commit/25cbab645575dac8fc08cab65eea23f11dd936ec))
* add continue-on-error to otel-cicd-action in remaining workflows ([a62a70e](https://github.com/martynvdijke/datey/commit/a62a70e10f0c648da1ada08b1cd0e2a9bdf748df))
* add continue-on-error to otel-cicd-action step (correct indentation) ([b0926fb](https://github.com/martynvdijke/datey/commit/b0926fbaf155dbda062bf06d1a697e21ee760195))
* **deps:** update all non-major dependencies ([09357e5](https://github.com/martynvdijke/datey/commit/09357e500f3f4ca1734b09a3d4db15cd73650de5))
* rename githubToken to otelToken for otel-cicd-action@v4 ([52f5d9f](https://github.com/martynvdijke/datey/commit/52f5d9f34c1017acf407ac82942a1552783c993f))
* revert otelToken to githubToken for otel-cicd-action@v4 ([234707f](https://github.com/martynvdijke/datey/commit/234707f5a062eddcc9e860ea3a42c1213ea3d927))
* use githubToken instead of otelToken for otel-cicd-action@v4 ([cbc5024](https://github.com/martynvdijke/datey/commit/cbc5024a27c22414cc8c8ed51ef3d212a84bde15))


### Features

* add otlpAuthorization input for Bearer auth ([1f1dc33](https://github.com/martynvdijke/datey/commit/1f1dc333fefbb0c86ac460300ee653f55d8db490))

## [1.1.2](https://github.com/martynvdijke/datey/compare/v1.1.1...v1.1.2) (2026-06-07)


### Bug Fixes

* **docker:** slim build context by excluding unnecessary files from .dockerignore ([42b36f8](https://github.com/martynvdijke/datey/commit/42b36f8b051fddda715fabdbf9f27020e485dc18))

## [1.1.1](https://github.com/martynvdijke/datey/compare/v1.1.0...v1.1.1) (2026-06-06)


### Bug Fixes

* ensure /data directory exists and is writable by datey user in Docker image ([5fc3093](https://github.com/martynvdijke/datey/commit/5fc309330f087946da04d15cda5c990126c379da))

# [1.1.0](https://github.com/martynvdijke/datey/compare/v1.0.0...v1.1.0) (2026-06-06)


### Bug Fixes

* align playwright test port with app default and wire version into health handler ([50f627f](https://github.com/martynvdijke/datey/commit/50f627faef3497ca03823d45be16f412eb0d304f))
* **deps:** update all non-major dependencies ([5f8c5e5](https://github.com/martynvdijke/datey/commit/5f8c5e5df5d75e6b8b13ac4f9ccd44de068bc71d))
* remove duplicate /health route from RegisterRoutes ([6d20dd7](https://github.com/martynvdijke/datey/commit/6d20dd7bc7a33a1a3203700fb4b66655dfa5c254))


### Features

* add central logging tab with ring buffer, OTEL export, and log level control ([29f3b5a](https://github.com/martynvdijke/datey/commit/29f3b5a18b81c1d26b946402a6d795d1e468b2a3))
* add Umami self-hosted analytics support with admin settings and script injection ([0309c06](https://github.com/martynvdijke/datey/commit/0309c061ded38165cc65a38087aa3ca872843d06))

## [1.0.1](https://github.com/martynvdijke/datey/compare/v1.0.0...v1.0.1) (2026-06-03)


### Bug Fixes

* **deps:** update all non-major dependencies ([5f8c5e5](https://github.com/martynvdijke/datey/commit/5f8c5e5df5d75e6b8b13ac4f9ccd44de068bc71d))

# 1.0.0 (2026-06-03)


### Bug Fixes

* **ci:** trigger ci for release ([ae54546](https://github.com/martynvdijke/datey/commit/ae5454655b7ef5bb0fa80025f81940d081a319b7))
* release process ([fe9277f](https://github.com/martynvdijke/datey/commit/fe9277f0c4e8745a55be0d4e1ab941cf007f9184))


### Features

* initial release ([421b0ee](https://github.com/martynvdijke/datey/commit/421b0ee20e75281a58fca84c2341d5020f676dae))

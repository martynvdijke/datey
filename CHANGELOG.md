# [1.15.0](https://github.com/martynvdijke/datey/compare/v1.14.4...v1.15.0) (2026-06-22)


### Bug Fixes

* **ci:** bump golangci-lint-action from v6 to v9 ([b03a616](https://github.com/martynvdijke/datey/commit/b03a616e7dcd62529356664e8bc77afbcda6da68))


### Features

* UI/UX design system, accessibility, security hardening, and tech debt cleanup ([1c6644c](https://github.com/martynvdijke/datey/commit/1c6644c8e335bf053b694ef51540bc147e06e233))

## [1.14.4](https://github.com/martynvdijke/datey/compare/v1.14.3...v1.14.4) (2026-06-22)


### Bug Fixes

* **deps:** update all non-major dependencies ([#12](https://github.com/martynvdijke/datey/issues/12)) ([28f8e35](https://github.com/martynvdijke/datey/commit/28f8e350ca72bb0253d3e71c102f48817423e645))

## [1.14.3](https://github.com/martynvdijke/datey/compare/v1.14.2...v1.14.3) (2026-06-20)


### Bug Fixes

* **notifications:** add tests for per-person notifications and test-send ([3ecbaf6](https://github.com/martynvdijke/datey/commit/3ecbaf6d3250b096d363169c4531f30b72a668e4))

## [1.14.2](https://github.com/martynvdijke/datey/compare/v1.14.1...v1.14.2) (2026-06-20)

## [1.14.1](https://github.com/martynvdijke/datey/compare/v1.14.0...v1.14.1) (2026-06-19)


### Bug Fixes

* **deps:** update github.com/emersion/go-vcard digest to d854b7e ([#11](https://github.com/martynvdijke/datey/issues/11)) ([d873d08](https://github.com/martynvdijke/datey/commit/d873d08fee6ee30b00c3d03654bc7f99d980d231))

# [1.14.0](https://github.com/martynvdijke/datey/compare/v1.13.1...v1.14.0) (2026-06-19)


### Features

* **ui:** modernize to standard Bootstrap 5.3 with 3-way Light/Dark/E-Ink theme toggle ([bae633b](https://github.com/martynvdijke/datey/commit/bae633babf26a41bd93f369b763ce4ad1f07a020))

## [1.13.1](https://github.com/martynvdijke/datey/compare/v1.13.0...v1.13.1) (2026-06-18)


### Bug Fixes

* **deps:** update all non-major dependencies to v1.14.46 ([#9](https://github.com/martynvdijke/datey/issues/9)) ([e4dff7a](https://github.com/martynvdijke/datey/commit/e4dff7aa4240698cb7ece357b02bd2018122c24e))

# [1.13.0](https://github.com/martynvdijke/datey/compare/v1.12.0...v1.13.0) (2026-06-18)


### Features

* **ui:** redesign navbar in Bootstrap+Material style, remove dice roller ([53c83e8](https://github.com/martynvdijke/datey/commit/53c83e87673f652e309ca76caaefe094648c1c82))

# [1.12.0](https://github.com/martynvdijke/datey/compare/v1.11.2...v1.12.0) (2026-06-17)


### Features

* **ui:** redesign navbar, add light/dark theme toggle, polish dashboard ([c6d5880](https://github.com/martynvdijke/datey/commit/c6d5880404a282dc6bfc3e3e91f15d88d0112526))

## [1.11.2](https://github.com/martynvdijke/datey/compare/v1.11.1...v1.11.2) (2026-06-17)


### Bug Fixes

* **navbar:** add regression tests for e-ink toggle contrast fix ([5bb5524](https://github.com/martynvdijke/datey/commit/5bb55240c05d758ab9f6c8ce047ddef4f3351089))

## [1.11.1](https://github.com/martynvdijke/datey/compare/v1.11.0...v1.11.1) (2026-06-17)


### Bug Fixes

* **navbar:** correct e-ink toggle button contrast and mobile toggler visibility ([799ec74](https://github.com/martynvdijke/datey/commit/799ec748725675b48e3bd10ea7230f191ccd4900)), closes [#6c757d](https://github.com/martynvdijke/datey/issues/6c757d) [#2d3a5c](https://github.com/martynvdijke/datey/issues/2d3a5c)

# [1.11.0](https://github.com/martynvdijke/datey/compare/v1.10.0...v1.11.0) (2026-06-16)


### Features

* add e-ink display mode with per-user toggle and config force ([d050413](https://github.com/martynvdijke/datey/commit/d05041315cbca6c4701c2fabb342bcba6084f3fa))

# [1.10.0](https://github.com/martynvdijke/datey/compare/v1.9.7...v1.10.0) (2026-06-15)


### Features

* people/groups rename, dice roller, dashboard date finder, email notifications, polish UI ([4eface2](https://github.com/martynvdijke/datey/commit/4eface202ef111e07a283beb87f0d8c54a06f625))

## [1.9.7](https://github.com/martynvdijke/datey/compare/v1.9.6...v1.9.7) (2026-06-15)

## [1.9.6](https://github.com/martynvdijke/datey/compare/v1.9.5...v1.9.6) (2026-06-14)


### Bug Fixes

* consolidate main.go, add DB health check, improve dashboard logging ([91a1f0c](https://github.com/martynvdijke/datey/commit/91a1f0c9ab6bea324ba84d028c37700d68d2bae5))

## [1.9.5](https://github.com/martynvdijke/datey/compare/v1.9.4...v1.9.5) (2026-06-14)


### Bug Fixes

* ensure database dir is writable at runtime with entrypoint script ([af512c2](https://github.com/martynvdijke/datey/commit/af512c2b74e46fbbdb05b8f6b08aea663809029b))

## [1.9.4](https://github.com/martynvdijke/datey/compare/v1.9.3...v1.9.4) (2026-06-14)


### Bug Fixes

* explicitly set DATA_DIR=/db in Dockerfile for data persistence ([ff8cd6e](https://github.com/martynvdijke/datey/commit/ff8cd6e6c458805cf5a6608338198588f2c02d48))

## [1.9.3](https://github.com/martynvdijke/datey/compare/v1.9.2...v1.9.3) (2026-06-14)


### Bug Fixes

* change default LOG_LEVEL from warn to info so startup logs are visible ([e0d348c](https://github.com/martynvdijke/datey/commit/e0d348c78e6281681f5b77506823872fea05eb53))

## [1.9.2](https://github.com/martynvdijke/datey/compare/v1.9.1...v1.9.2) (2026-06-13)

## [1.9.1](https://github.com/martynvdijke/datey/compare/v1.9.0...v1.9.1) (2026-06-12)


### Bug Fixes

* handle empty/invalid channel_targets in scheduler with fallback ([c30ac5b](https://github.com/martynvdijke/datey/commit/c30ac5b75a3fa103c36d6b092d35ca31185419c0))

# [1.9.0](https://github.com/martynvdijke/datey/compare/v1.8.0...v1.9.0) (2026-06-12)


### Features

* add email notifications with per-channel delivery tracking ([dacc8f2](https://github.com/martynvdijke/datey/commit/dacc8f229cb490fc91bfbd61cda0e709da855a3f))

# [1.8.0](https://github.com/martynvdijke/datey/compare/v1.7.0...v1.8.0) (2026-06-11)


### Features

* add one-time notification support with scheduler, web UI, and tests ([112a1bf](https://github.com/martynvdijke/datey/commit/112a1bf3f82f13820d0b19fc4acce595c27c1162))

# [1.7.0](https://github.com/martynvdijke/datey/compare/v1.6.1...v1.7.0) (2026-06-11)


### Features

* add vCard import/export support ([bb7e0d6](https://github.com/martynvdijke/datey/commit/bb7e0d68e3fb93c5046ca8ddc0e2a9725416fa75))

## [1.6.1](https://github.com/martynvdijke/datey/compare/v1.6.0...v1.6.1) (2026-06-11)


### Bug Fixes

* docker-compose DATA_DIR=/db to match volume mount, add config tests ([2cebb6e](https://github.com/martynvdijke/datey/commit/2cebb6e47265a3fc0429d10bfa97d6d670891be3))

# [1.6.0](https://github.com/martynvdijke/datey/compare/v1.5.0...v1.6.0) (2026-06-11)


### Features

* container /db mount, settings overhaul, logs in settings, and test infrastructure ([3a6a76e](https://github.com/martynvdijke/datey/commit/3a6a76e646e6bfa50e679c547421bbb4c0f47fd7))

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

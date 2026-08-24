# [1.44.0](https://github.com/martynvdijke/datey/compare/v1.43.0...v1.44.0) (2026-08-24)


### Features

* **notifications:** per-user channel targets, event scoping, and global fallback ([02e402d](https://github.com/martynvdijke/datey/commit/02e402d7cabbb922394b54d5a8ac181b1195a470))

# [1.43.0](https://github.com/martynvdijke/datey/compare/v1.42.0...v1.43.0) (2026-08-24)


### Features

* **notifier:** Discord, Slack, and Matrix notification channels ([7f115e1](https://github.com/martynvdijke/datey/commit/7f115e11d41d06e34ea7f395f26ad06b17df6ec9))

# [1.42.0](https://github.com/martynvdijke/datey/compare/v1.41.0...v1.42.0) (2026-08-24)


### Features

* **i18n:** embedded en/de catalogs, locale middleware, per-user language setting ([b5501ec](https://github.com/martynvdijke/datey/commit/b5501ec73c4fd6de69bf39ad7947c84e6eb7c0b9))

# [1.41.0](https://github.com/martynvdijke/datey/compare/v1.40.0...v1.41.0) (2026-08-24)


### Features

* **pwa:** installable manifest, versioned offline service worker, offline banner ([10d5f7e](https://github.com/martynvdijke/datey/commit/10d5f7e72100cd4ada9984e70dbdd4faafac7921))

# [1.40.0](https://github.com/martynvdijke/datey/compare/v1.39.0...v1.40.0) (2026-08-24)


### Features

* **events:** lunar calendar birthdays with leap-month handling ([4aeeb9b](https://github.com/martynvdijke/datey/commit/4aeeb9bd87343c269c28df9913227abcd41b171f))
* **homeassistant:** calendar entity endpoint for HA calendar discovery ([0d93c52](https://github.com/martynvdijke/datey/commit/0d93c52d15c2e57ce9db74ba163986be1431da7d))
* **homeassistant:** finish calendar entity — reminder-window default, settings URL, binary sensors ([b2378ad](https://github.com/martynvdijke/datey/commit/b2378adf215666e6f619cd0e1b70b916c9e1c736))

# [1.39.0](https://github.com/martynvdijke/datey/compare/v1.38.1...v1.39.0) (2026-08-23)


### Bug Fixes

* **events:** use tagged switch for pattern type validation ([ad52752](https://github.com/martynvdijke/datey/commit/ad5275212a53bb580dc0a117459f676e7ea178e1))


### Features

* **ent:** add tag and gift idea schemas with generated code ([dd292e4](https://github.com/martynvdijke/datey/commit/dd292e47670f0ac84683db3d3b3b37b12688ddf6))
* **events:** custom nth-weekday recurrence rules ([19a1790](https://github.com/martynvdijke/datey/commit/19a1790a6991176786d0b5fa7d37d8af6b1fa5d5))
* **people:** edit person details and per-person photo settings modal ([565a267](https://github.com/martynvdijke/datey/commit/565a267fa6e0fb631d523bab257a30a68c978ec1))
* **people:** free-form tags with autocomplete and list filtering ([3630cf3](https://github.com/martynvdijke/datey/commit/3630cf350a6490037d2a893bd9e87389f471a4b7))
* **people:** milestone detection for event dates ([9c196c4](https://github.com/martynvdijke/datey/commit/9c196c4653257201f73192998980164b210dd228))
* **people:** per-person gift ideas with status lifecycle ([b04b5eb](https://github.com/martynvdijke/datey/commit/b04b5eb06f56963c8645f5b55c35b105fb74f0f8))
* **stats:** read-only stats dashboard with css-only charts ([ba74be8](https://github.com/martynvdijke/datey/commit/ba74be8f7675f7ab64bb3785586129712f96dca3))

## [1.38.1](https://github.com/martynvdijke/datey/compare/v1.38.0...v1.38.1) (2026-08-23)


### Bug Fixes

* **immich:** handle paginated /api/people response and add client tests ([38c9a8a](https://github.com/martynvdijke/datey/commit/38c9a8ab833b298c7ac6fd2fabfad493c16c14d0))

# [1.38.0](https://github.com/martynvdijke/datey/compare/v1.37.3...v1.38.0) (2026-08-21)


### Bug Fixes

* **lint:** check Close error returns, drop unused testAll result type ([daf4327](https://github.com/martynvdijke/datey/commit/daf43270b727ba66ffdb20a692f4a6082022d29e))


### Features

* **people:** group categories, Immich bulk photo sync, config test buttons ([43a6d97](https://github.com/martynvdijke/datey/commit/43a6d9735d699578714baf7391f76867c3857ccd))

## [1.37.3](https://github.com/martynvdijke/datey/compare/v1.37.2...v1.37.3) (2026-08-20)


### Bug Fixes

* **deps:** update all non-major dependencies ([#40](https://github.com/martynvdijke/datey/issues/40)) ([3cffb1a](https://github.com/martynvdijke/datey/commit/3cffb1ae7264e5f637963934291a81a583635121))

## [1.37.2](https://github.com/martynvdijke/datey/compare/v1.37.1...v1.37.2) (2026-08-19)


### Bug Fixes

* **scheduler:** notify today's events and make all events annual ([c105fcb](https://github.com/martynvdijke/datey/commit/c105fcb75a4d390d82d2daf738eb6f7015914b38))
* **scheduler:** use tagged switch for day phrasing (golangci QF1003) ([7c9b023](https://github.com/martynvdijke/datey/commit/7c9b023b1930958c5a9d6e5bf767ba638ad94ab8))

## [1.37.1](https://github.com/martynvdijke/datey/compare/v1.37.0...v1.37.1) (2026-08-18)


### Bug Fixes

* **deps:** update all non-major dependencies ([aca8ce9](https://github.com/martynvdijke/datey/commit/aca8ce9a3682d4af7c961a9a75d0c3e18699236d))

# [1.37.0](https://github.com/martynvdijke/datey/compare/v1.36.1...v1.37.0) (2026-08-18)


### Bug Fixes

* **trmnl:** remove unused channelInfo code after dropping channels UI ([1798a97](https://github.com/martynvdijke/datey/commit/1798a973ac144a67af29a8d22c924a7a5052d16f))


### Features

* **trmnl:** always show next upcoming birthday, drop channels UI ([1d0e85b](https://github.com/martynvdijke/datey/commit/1d0e85bbe2ff5cf2bbb30e82237f6cdd698db792))

## [1.36.1](https://github.com/martynvdijke/datey/compare/v1.36.0...v1.36.1) (2026-08-16)


### Bug Fixes

* **trmnl:** fix dashboard data bindings and layout width ([a070b4f](https://github.com/martynvdijke/datey/commit/a070b4f552d70effd79b2eb31362761b54984ea1))

# [1.36.0](https://github.com/martynvdijke/datey/compare/v1.35.5...v1.36.0) (2026-08-16)


### Features

* **auth:** add email password reset flow ([d87b48a](https://github.com/martynvdijke/datey/commit/d87b48ad4516dd924d1f6fa66d9d42059b794480))

## [1.35.5](https://github.com/martynvdijke/datey/compare/v1.35.4...v1.35.5) (2026-08-16)

## [1.35.4](https://github.com/martynvdijke/datey/compare/v1.35.3...v1.35.4) (2026-08-15)


### Bug Fixes

* **trmnl:** pin plugin id 443906 ([f8dffc9](https://github.com/martynvdijke/datey/commit/f8dffc9fea6232b4f05b408f7c33ac6a8ea48865))

## [1.35.3](https://github.com/martynvdijke/datey/compare/v1.35.2...v1.35.3) (2026-08-15)


### Bug Fixes

* **trmnl:** pin plugin id 443905 ([1757f74](https://github.com/martynvdijke/datey/commit/1757f7494de3af9633c3dbca9cfd19d7cee6d9af))

## [1.35.2](https://github.com/martynvdijke/datey/compare/v1.35.1...v1.35.2) (2026-08-15)


### Bug Fixes

* **trmnl:** recreate deleted plugins ([9ad640a](https://github.com/martynvdijke/datey/commit/9ad640abf555656c6590c91372942fc738780fa4))

## [1.35.1](https://github.com/martynvdijke/datey/compare/v1.35.0...v1.35.1) (2026-08-15)


### Bug Fixes

* **trmnl:** pin plugin IDs and gate trmnlp push on release ([85f3433](https://github.com/martynvdijke/datey/commit/85f3433eeca6417a1948def89c9c22326592b4e2))

# [1.35.0](https://github.com/martynvdijke/datey/compare/v1.34.0...v1.35.0) (2026-08-15)


### Bug Fixes

* **trmnl:** shorten plugin descriptions to satisfy trmnlp lint ([f99b55c](https://github.com/martynvdijke/datey/commit/f99b55ca01ec904cfff76509a471612880d2764a))


### Features

* **trmnl:** add birthdays plugin and restructure plugin layout ([9797624](https://github.com/martynvdijke/datey/commit/979762476b9f1426c6d16fddbe77994953a15ec3))

# [1.34.0](https://github.com/martynvdijke/datey/compare/v1.33.3...v1.34.0) (2026-08-15)


### Features

* **backup:** add automatic weekly backups with configurable retention ([f07c2b2](https://github.com/martynvdijke/datey/commit/f07c2b2196e04b0907f0a7deafce7ed658d30713))

## [1.33.3](https://github.com/martynvdijke/datey/compare/v1.33.2...v1.33.3) (2026-08-14)

## [1.33.2](https://github.com/martynvdijke/datey/compare/v1.33.1...v1.33.2) (2026-08-14)


### Bug Fixes

* **deps:** update module github.com/playwright-community/playwright-go to v0.6201.0 ([2aed585](https://github.com/martynvdijke/datey/commit/2aed585219e95bd6aa9caf49be57ea446647a0e5))

## [1.33.1](https://github.com/martynvdijke/datey/compare/v1.33.0...v1.33.1) (2026-08-14)


### Bug Fixes

* **calendar:** honor FullCalendar RFC3339 range params; fix eink weekend headers ([64d0c91](https://github.com/martynvdijke/datey/commit/64d0c91b617ecaebe13b8f2fce7e1b1f890841de))

# [1.33.0](https://github.com/martynvdijke/datey/compare/v1.32.0...v1.33.0) (2026-08-13)


### Features

* **calendar:** theme FullCalendar across all modes and expand annual events ([687ce5d](https://github.com/martynvdijke/datey/commit/687ce5dfbe25a0d99938e22e3a6e54c0cd554c3b)), closes [hi#contrast](https://github.com/hi/issues/contrast)

# [1.32.0](https://github.com/martynvdijke/datey/compare/v1.31.0...v1.32.0) (2026-08-13)


### Bug Fixes

* satisfy golangci-lint in carddav client and settings ([1c50e88](https://github.com/martynvdijke/datey/commit/1c50e882270e0696281073ab67f1dc5796940f04))


### Features

* catch up missed reminders and two-way CardDAV sync ([d9be804](https://github.com/martynvdijke/datey/commit/d9be804f52540722846636b605c9086e9f4e7089))

# [1.31.0](https://github.com/martynvdijke/datey/compare/v1.30.1...v1.31.0) (2026-08-12)


### Bug Fixes

* **immich:** check response body close errors ([bd831c0](https://github.com/martynvdijke/datey/commit/bd831c03a0638478099931009e2a57b277c2aead))


### Features

* **people:** polish people pages with structured vCard data and Immich photos ([38ed756](https://github.com/martynvdijke/datey/commit/38ed756482c6b733d308675b68d59e5a0646aaa8))

## [1.30.1](https://github.com/martynvdijke/datey/compare/v1.30.0...v1.30.1) (2026-08-12)


### Bug Fixes

* **deps:** update module golang.org/x/crypto to v0.55.0 ([#35](https://github.com/martynvdijke/datey/issues/35)) ([043b8af](https://github.com/martynvdijke/datey/commit/043b8af4b66dbad7a8d9d862dfe0a2f3f9b3cc75))

# [1.30.0](https://github.com/martynvdijke/datey/compare/v1.29.0...v1.30.0) (2026-08-12)


### Features

* **notifications:** remove one-time notification feature ([0aa881b](https://github.com/martynvdijke/datey/commit/0aa881bc4060f8eb2e868733b7bbf230f04ce2e1))
* **scheduler:** annual birthday notifications ([0e86d7b](https://github.com/martynvdijke/datey/commit/0e86d7bfa3763beb27544a9f9ea06302cc249de0))

# [1.29.0](https://github.com/martynvdijke/datey/compare/v1.28.0...v1.29.0) (2026-08-11)


### Features

* **ics:** import calendar events from .ics files ([bc02511](https://github.com/martynvdijke/datey/commit/bc0251163e30fdb318806d5e9151217709554df5))

# [1.28.0](https://github.com/martynvdijke/datey/compare/v1.27.1...v1.28.0) (2026-08-11)


### Features

* **vcard:** multi-file import, overwrite option, and yearless BDAY support ([02b1eaa](https://github.com/martynvdijke/datey/commit/02b1eaa668d8f8387dfb52abf107f6368fc3abe1))

## [1.27.1](https://github.com/martynvdijke/datey/compare/v1.27.0...v1.27.1) (2026-08-10)

# [1.27.0](https://github.com/martynvdijke/datey/compare/v1.26.0...v1.27.0) (2026-08-09)


### Features

* **push:** add web push notifications via VAPID ([886eb80](https://github.com/martynvdijke/datey/commit/886eb80942f23e0b1bdf482b24354d20f556144f))

# [1.26.0](https://github.com/martynvdijke/datey/compare/v1.25.0...v1.26.0) (2026-08-09)


### Features

* **ha:** add Home Assistant stats feed and plugin folder ([fb02293](https://github.com/martynvdijke/datey/commit/fb022931543f35b9f5c847ae207547c45b6c75c4))

# [1.25.0](https://github.com/martynvdijke/datey/compare/v1.24.0...v1.25.0) (2026-08-09)


### Features

* **api:** add public upcoming events JSON endpoint ([1dbd590](https://github.com/martynvdijke/datey/commit/1dbd59022a88ce1b043aae4a52d04c1b5a1a4b59))

# [1.24.0](https://github.com/martynvdijke/datey/compare/v1.23.0...v1.24.0) (2026-08-09)


### Bug Fixes

* **lint:** resolve golangci-lint issues blocking CI ([6ffbf28](https://github.com/martynvdijke/datey/commit/6ffbf28a6ba1598e8df3170c7474905dacf5025d))


### Features

* **feed:** add public RSS feed of upcoming events ([836bfec](https://github.com/martynvdijke/datey/commit/836bfec34a9f6158f0409c2e33b6979d149568d3))
* **notify:** add generic JSON webhook notification channel ([68f3d75](https://github.com/martynvdijke/datey/commit/68f3d75bb83f333d3ecd0f3fb983e975eafa3738))
* **notify:** add ntfy.sh push notification channel ([82a8293](https://github.com/martynvdijke/datey/commit/82a82938bdb55dfa638e2a5ba1d7f24e148c526f))
* **people:** show calculated ages from birthday events ([613a7c5](https://github.com/martynvdijke/datey/commit/613a7c5c2d6907ccbb758c9219f75474f4a01a32))
* **rules:** add Easter-based recurring rules computed via Gregorian computus ([6a95a7c](https://github.com/martynvdijke/datey/commit/6a95a7ce4bd55ce5c732dc9e583ba8b7520b1fc5))

# [1.23.0](https://github.com/martynvdijke/datey/compare/v1.22.0...v1.23.0) (2026-08-08)


### Features

* **trmnl:** add TRMNL e-ink stats plugin with public stats feed ([0162ef5](https://github.com/martynvdijke/datey/commit/0162ef5a6c0b1fef51482c174d10da950c3924f9))

# [1.22.0](https://github.com/martynvdijke/datey/compare/v1.21.14...v1.22.0) (2026-08-08)


### Features

* **ical:** add public iCal feed with configurable event timing ([a8d1a58](https://github.com/martynvdijke/datey/commit/a8d1a58d6f36a1d76abe4472bf4f804dfc1f5894))

## [1.21.14](https://github.com/martynvdijke/datey/compare/v1.21.13...v1.21.14) (2026-08-07)


### Bug Fixes

* **ui:** show flash messages as toasts on redirect-based flows ([afe5963](https://github.com/martynvdijke/datey/commit/afe596316a49fdc3557a3425223ccfaf651417d7))

## [1.21.13](https://github.com/martynvdijke/datey/compare/v1.21.12...v1.21.13) (2026-08-05)


### Bug Fixes

* **deps:** update all non-major dependencies ([#32](https://github.com/martynvdijke/datey/issues/32)) ([912dcc0](https://github.com/martynvdijke/datey/commit/912dcc0828f1fe423d61b0e8a326a8b67dc28ecf))

## [1.21.12](https://github.com/martynvdijke/datey/compare/v1.21.11...v1.21.12) (2026-08-03)

## [1.21.11](https://github.com/martynvdijke/datey/compare/v1.21.10...v1.21.11) (2026-07-31)

## [1.21.10](https://github.com/martynvdijke/datey/compare/v1.21.9...v1.21.10) (2026-07-30)

## [1.21.9](https://github.com/martynvdijke/datey/compare/v1.21.8...v1.21.9) (2026-07-29)


### Bug Fixes

* **deps:** update module github.com/mattn/go-sqlite3 to v1.14.49 ([#28](https://github.com/martynvdijke/datey/issues/28)) ([dd8107f](https://github.com/martynvdijke/datey/commit/dd8107f4504b228d33e92bc7e4e2ba41c24ea98e))

## [1.21.8](https://github.com/martynvdijke/datey/compare/v1.21.7...v1.21.8) (2026-07-28)

## [1.21.7](https://github.com/martynvdijke/datey/compare/v1.21.6...v1.21.7) (2026-07-27)

## [1.21.6](https://github.com/martynvdijke/datey/compare/v1.21.5...v1.21.6) (2026-07-26)

## [1.21.5](https://github.com/martynvdijke/datey/compare/v1.21.4...v1.21.5) (2026-07-25)

## [1.21.4](https://github.com/martynvdijke/datey/compare/v1.21.3...v1.21.4) (2026-07-20)

## [1.21.3](https://github.com/martynvdijke/datey/compare/v1.21.2...v1.21.3) (2026-07-16)

## [1.21.2](https://github.com/martynvdijke/datey/compare/v1.21.1...v1.21.2) (2026-07-14)

## [1.21.1](https://github.com/martynvdijke/datey/compare/v1.21.0...v1.21.1) (2026-07-13)


### Bug Fixes

* **deps:** update all non-major dependencies ([#20](https://github.com/martynvdijke/datey/issues/20)) ([19a14cf](https://github.com/martynvdijke/datey/commit/19a14cf656b6ffab9c639b07465fb84502671685))

# [1.21.0](https://github.com/martynvdijke/datey/compare/v1.20.0...v1.21.0) (2026-07-12)


### Bug Fixes

* suppress unused errcheck warning on os.Setenv in otel.go ([9bcc724](https://github.com/martynvdijke/datey/commit/9bcc7242cc25156d7784f8f8731808fd07ce88ef))


### Features

* add OpenTelemetry observability with traces, metrics, and logs ([982c919](https://github.com/martynvdijke/datey/commit/982c9193aeabdf3a50a931643f17dab478bb93e1))

# [1.20.0](https://github.com/martynvdijke/datey/compare/v1.19.4...v1.20.0) (2026-07-11)


### Bug Fixes

* remove unnecessary nil check on errs map in test ([4fc6536](https://github.com/martynvdijke/datey/commit/4fc6536a1763cd31dcd01cd3c7f734529eac342b))


### Features

* allow all settings to be overridden from the database via admin UI ([cc8c53f](https://github.com/martynvdijke/datey/commit/cc8c53fa17d4b25fdd3b57e4ea3515b955f3f14a))

## [1.19.4](https://github.com/martynvdijke/datey/compare/v1.19.3...v1.19.4) (2026-07-09)


### Bug Fixes

* **deps:** update module golang.org/x/crypto to v0.54.0 ([#19](https://github.com/martynvdijke/datey/issues/19)) ([d919761](https://github.com/martynvdijke/datey/commit/d919761a96b66c267c0ae1caccbbd218a2fd8a59))

## [1.19.3](https://github.com/martynvdijke/datey/compare/v1.19.2...v1.19.3) (2026-07-08)

## [1.19.2](https://github.com/martynvdijke/datey/compare/v1.19.1...v1.19.2) (2026-07-07)


### Bug Fixes

* **deps:** update module github.com/go-chi/chi/v5 to v5.3.1 ([#17](https://github.com/martynvdijke/datey/issues/17)) ([b5ac3c5](https://github.com/martynvdijke/datey/commit/b5ac3c56d544aa926ebc2972b853c16b76b09dd5))

## [1.19.1](https://github.com/martynvdijke/datey/compare/v1.19.0...v1.19.1) (2026-07-06)

# [1.19.0](https://github.com/martynvdijke/datey/compare/v1.18.2...v1.19.0) (2026-07-05)


### Features

* store raw vCard data on import and display on person detail ([fce0d75](https://github.com/martynvdijke/datey/commit/fce0d7529871aa64cfba991b0aa9e9c573e134a4))

## [1.18.2](https://github.com/martynvdijke/datey/compare/v1.18.1...v1.18.2) (2026-07-04)


### Bug Fixes

* update e2e selectors for people-grid, add vCard test fixtures ([1ec0f22](https://github.com/martynvdijke/datey/commit/1ec0f22a2c3584040133f00671c368d88459ae0a))

## [1.18.1](https://github.com/martynvdijke/datey/compare/v1.18.0...v1.18.1) (2026-07-03)


### Bug Fixes

* **deps:** update all non-major dependencies ([b69b679](https://github.com/martynvdijke/datey/commit/b69b679f6bce20b4c11befa35f6bb4befa97c7fc))

# [1.18.0](https://github.com/martynvdijke/datey/compare/v1.17.0...v1.18.0) (2026-07-03)


### Features

* gender support (F→Female/M→Male in Notes) + fix scheduler Person edge for birthday reminders ([ef2c51c](https://github.com/martynvdijke/datey/commit/ef2c51c6d2b64ea20b94c5c5cd93d334dadc8207))

# [1.17.0](https://github.com/martynvdijke/datey/compare/v1.16.0...v1.17.0) (2026-07-03)


### Features

* improved vCard import — preserve unknown fields in Notes, HTMX inline results workflow ([3059c56](https://github.com/martynvdijke/datey/commit/3059c56e037f9fe548e0252299f92b59ab0ad6c2))

# [1.16.0](https://github.com/martynvdijke/datey/compare/v1.15.5...v1.16.0) (2026-07-01)


### Features

* enrich vCard import with structured BDAY/GENDER/N parsing + auto birthday event ([1e84dd9](https://github.com/martynvdijke/datey/commit/1e84dd92b3ba6d79a4652e5a635f8556500cf099))

## [1.15.5](https://github.com/martynvdijke/datey/compare/v1.15.4...v1.15.5) (2026-06-26)


### Bug Fixes

* **deps:** update module github.com/playwright-community/playwright-go to v0.6000.0 ([#14](https://github.com/martynvdijke/datey/issues/14)) ([166dd12](https://github.com/martynvdijke/datey/commit/166dd125a0f04d77e7fe822909482ef15dec5512))

## [1.15.4](https://github.com/martynvdijke/datey/compare/v1.15.3...v1.15.4) (2026-06-24)


### Bug Fixes

* static file handler path bug causing CSS 404, improve login page and slider UI ([91a11af](https://github.com/martynvdijke/datey/commit/91a11af008c467e5d0cba64a35c738da23eb4648))

## [1.15.3](https://github.com/martynvdijke/datey/compare/v1.15.2...v1.15.3) (2026-06-24)

## [1.15.2](https://github.com/martynvdijke/datey/compare/v1.15.1...v1.15.2) (2026-06-24)

## [1.15.1](https://github.com/martynvdijke/datey/compare/v1.15.0...v1.15.1) (2026-06-23)


### Bug Fixes

* dashboard template nested define, CSS event-card refactor, eink cleanup ([ddb4514](https://github.com/martynvdijke/datey/commit/ddb45145f54c7579c12a6596a36c7c2dc9445750))

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

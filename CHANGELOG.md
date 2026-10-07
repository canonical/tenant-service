# Changelog

## [0.4.0](https://github.com/canonical/tenant-service/compare/v0.3.1...v0.4.0) (2026-10-07)


### Features

* **ci:** shift-left vulnerability check & release quarantine gate ([c15d4f0](https://github.com/canonical/tenant-service/commit/c15d4f0d85bd53f8496084ced0e060b1dabc72fc))
* **ci:** shift-left vulnerability check & release quarantine gate ([#112](https://github.com/canonical/tenant-service/issues/112)) ([70fec42](https://github.com/canonical/tenant-service/commit/70fec4299ac78284bbb247ed0f84f181cc718b2a))
* federate with authorization service ([f125c01](https://github.com/canonical/tenant-service/commit/f125c0172cf6287e00b2ee6101f2e2695a72ad5b))


### Bug Fixes

* create invited identities with Kratos's default schema ([edb3925](https://github.com/canonical/tenant-service/commit/edb3925b054b9d52aaad6d41a4542e0e25cd0219))
* create invited identities with Kratos's default schema ([#124](https://github.com/canonical/tenant-service/issues/124)) ([91fb175](https://github.com/canonical/tenant-service/commit/91fb175e558b87a75e6d59b1478987a17f79a870))
* **deps:** update go deps ([d4ec300](https://github.com/canonical/tenant-service/commit/d4ec3007e73aaec3664bf2ec13b0687da730edfc))
* **deps:** update go deps ([c4f7451](https://github.com/canonical/tenant-service/commit/c4f7451d5d82207beafeb947273a1372c13e1946))
* **deps:** update go deps (excluding identity-platform-api) ([c8bc58e](https://github.com/canonical/tenant-service/commit/c8bc58e08159bdeccc0206d759e0588505532b2d))
* **deps:** update go deps (minor) ([#25](https://github.com/canonical/tenant-service/issues/25)) ([7662cbe](https://github.com/canonical/tenant-service/commit/7662cbe2a302f54d21bb4290002f2172468e95ee))
* **deps:** update go deps (patch) ([#33](https://github.com/canonical/tenant-service/issues/33)) ([de4ada0](https://github.com/canonical/tenant-service/commit/de4ada0952f358fe0c8ff513f32010f5b627920b))
* **deps:** update go deps (supersedes [#25](https://github.com/canonical/tenant-service/issues/25)) ([#115](https://github.com/canonical/tenant-service/issues/115)) ([9327660](https://github.com/canonical/tenant-service/commit/93276600fe989f3c108f26516a597b1aca965ef3))
* **deps:** update go deps to fad4113 ([7778743](https://github.com/canonical/tenant-service/commit/7778743f70c353c17cd93d0b976fe4f21712bad2))
* **deps:** update go deps to fad4113 ([#134](https://github.com/canonical/tenant-service/issues/134)) ([e865ec3](https://github.com/canonical/tenant-service/commit/e865ec3ee48266ab17ea4752b061f421631f3185))
* **deps:** update module github.com/exaring/otelpgx to v0.12.1 ([213acc2](https://github.com/canonical/tenant-service/commit/213acc2be5a514c9c0361da0090244794e5d63d0))
* **deps:** update module github.com/exaring/otelpgx to v0.12.1 ([#136](https://github.com/canonical/tenant-service/issues/136)) ([652c265](https://github.com/canonical/tenant-service/commit/652c265059044a99a7a89a9fb9f9ff5375073dac))
* **deps:** update module github.com/ory/client-go to v1.22.79 ([9794088](https://github.com/canonical/tenant-service/commit/9794088251d70c8df22825729b2604a0373fccc3))
* **deps:** update module github.com/ory/client-go to v1.22.79 ([#127](https://github.com/canonical/tenant-service/issues/127)) ([11d16aa](https://github.com/canonical/tenant-service/commit/11d16aac457a740e85b777730d2b847e4b262fe1))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace to v1.45.0 [security] ([6dcdf51](https://github.com/canonical/tenant-service/commit/6dcdf51d24c9742a20775ea14b94f050cc7c921e))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc to v1.45.0 [security] ([13cdc79](https://github.com/canonical/tenant-service/commit/13cdc795977be2c3616ce669a767daef77bdb910))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp to v1.45.0 [security] ([77b3f38](https://github.com/canonical/tenant-service/commit/77b3f38b0b2138045187d88a0392fdfb8a35c879))

## [0.3.1](https://github.com/canonical/tenant-service/compare/v0.3.0...v0.3.1) (2026-08-13)


### Bug Fixes

* fix vulnerabilities ([22bd605](https://github.com/canonical/tenant-service/commit/22bd6059e7822154201e12c1e2901172aa407035))

## [0.3.0](https://github.com/canonical/tenant-service/compare/v0.2.0...v0.3.0) (2026-07-01)


### Features

* consume identity-platform-api and restore local openapi http client generation ([a0ea794](https://github.com/canonical/tenant-service/commit/a0ea7942755e44bd0042203736ea9bb02fcdb800))


### Bug Fixes

* add include_email param ([93d2828](https://github.com/canonical/tenant-service/commit/93d282819df602c653944e31908d571b0a3655e6))
* add interceptors ([e2cb03b](https://github.com/canonical/tenant-service/commit/e2cb03b1eb5b8ac9d38e15ca01d3c386424a1b45))
* consolidate tenant APIs ([a1f5f19](https://github.com/canonical/tenant-service/commit/a1f5f190590cc3ed66d42e70c50531947eb26441))
* fix vulnerabilities ([bff85f5](https://github.com/canonical/tenant-service/commit/bff85f54cc766c28538476fd14bbceeb8ec6ac03))
* use identity-platform-api ([2e30a58](https://github.com/canonical/tenant-service/commit/2e30a58a86784207185c4abc3f3c3ae046955a31)), closes [#13](https://github.com/canonical/tenant-service/issues/13)

## [0.2.0](https://github.com/canonical/tenant-service/compare/v0.1.0...v0.2.0) (2026-04-28)


### Features

* **api:** update tenant proto definition and generated files ([1ee5b95](https://github.com/canonical/tenant-service/commit/1ee5b95e31bc4583d0f33d5a6c9b921208c0507f))
* **auth:** implement identity and kratos integration ([eab182f](https://github.com/canonical/tenant-service/commit/eab182fc65dffc15a97c7abf6d01c8ed09d953b0))
* **cli:** add client commands and tools ([8d78d0a](https://github.com/canonical/tenant-service/commit/8d78d0ae7054fc504ae841d160193b592b3357c7))
* implement tenant-aware login integration and docs ([032a0c6](https://github.com/canonical/tenant-service/commit/032a0c6df4abac2eff6a2c517ca338c30547d7a2))
* **internal:** update storage, types, and openfga client ([5a04e67](https://github.com/canonical/tenant-service/commit/5a04e67ded55e1aa387eba7d83bf58562af72ae3))
* **tenant:** implement tenant service and server wiring ([9b114b5](https://github.com/canonical/tenant-service/commit/9b114b5f1aaba7c32185b5310324e2238d30468f))
* **webhooks,tenant:** implement Kratos login/registration hooks and email-based tenant lookup ([6f3f156](https://github.com/canonical/tenant-service/commit/6f3f1569e6d3b099efbe2f2a6499b2cf63523ab3))


### Bug Fixes

* add api token authn to webhooks API ([9e09f58](https://github.com/canonical/tenant-service/commit/9e09f58d7a6828f4a8a9d229700f8ff81f8984e9))
* add ca-certificates and ssl cert permissions to rock ([9f7fbdc](https://github.com/canonical/tenant-service/commit/9f7fbdc62fd49a1e49b11b59e0ce1b427e9913d6))
* add common /api/v0 prefix ([448f4e6](https://github.com/canonical/tenant-service/commit/448f4e641a6141a33ff92b38fb4527062758adf1))
* add database client ([a7c864a](https://github.com/canonical/tenant-service/commit/a7c864a993069d3ad9fa4a55f25eccd3241da2ee))
* add database monitoring ([6469eea](https://github.com/canonical/tenant-service/commit/6469eea9b82690965def4f8ffa6caca685bfae8d))
* add hydra hook ([760848e](https://github.com/canonical/tenant-service/commit/760848e471d3ec09ea15f9025d3d7c48640fae32))
* add jwt authentication ([3d13c98](https://github.com/canonical/tenant-service/commit/3d13c981b1419b6a653b950520ad9d9882b5f6f9))
* add jwt authentication ([13f4556](https://github.com/canonical/tenant-service/commit/13f4556b526ad801133915ba4765a906be92e806))
* add kratos webhook handler ([e0db1c4](https://github.com/canonical/tenant-service/commit/e0db1c4c7c0f023062c944f540ac727a4352f8d2))
* add more prometheus metrics ([5b49011](https://github.com/canonical/tenant-service/commit/5b49011451485b1d9b525048c75d644cac047051))
* add openfga client ([f7aa825](https://github.com/canonical/tenant-service/commit/f7aa825296413ea35ddb0caf0c8833fe16c7c9b2))
* add pagination to list endpoints ([500d84b](https://github.com/canonical/tenant-service/commit/500d84b84f9502f21e5567bc8656f1fd0b8499bd))
* allow lookup by id ([f520840](https://github.com/canonical/tenant-service/commit/f520840c4d9a66d72b6a2dcfb3288185e325cbe1))
* **deps:** update go deps ([05f945f](https://github.com/canonical/tenant-service/commit/05f945fc99d04e2eca72bfb63008d622eb117cb0))
* **deps:** update go deps ([3ef9ed5](https://github.com/canonical/tenant-service/commit/3ef9ed5af7b32f989bed16e1431c0a40ac88b919))
* **deps:** update go deps (minor) ([#21](https://github.com/canonical/tenant-service/issues/21)) ([9da80d9](https://github.com/canonical/tenant-service/commit/9da80d9547be9d05f29c51955801bb873a0101d1))
* **deps:** update go deps (patch) ([#19](https://github.com/canonical/tenant-service/issues/19)) ([1d9681c](https://github.com/canonical/tenant-service/commit/1d9681c3e7f6253d36d35baa8aa7f17c360dcca0))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp to v1.43.0 [security] ([4578d56](https://github.com/canonical/tenant-service/commit/4578d56ff031ed16df77b5df4d0d0290c625e779))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp to v1.43.0 [security] ([#58](https://github.com/canonical/tenant-service/issues/58)) ([8faefed](https://github.com/canonical/tenant-service/commit/8faefeda06661e13cc7f7453984ef7e9ebea6693))
* **deps:** update module go.opentelemetry.io/otel/sdk to v1.43.0 [security] ([0ee9716](https://github.com/canonical/tenant-service/commit/0ee9716c106ea8cb16c1c16474de05cc732b9fb6))
* downgrade docker/docker and docker/cli to v28.0.1 to fix e2e build ([103a8f5](https://github.com/canonical/tenant-service/commit/103a8f59bf8bca40c9fcdc0f2c838674581a000a))
* enhance logs with more info ([05bde05](https://github.com/canonical/tenant-service/commit/05bde0594a397416fcdc011a04a27e34b227ec7a))
* enhance traces ([0de9444](https://github.com/canonical/tenant-service/commit/0de94442d11d9488d8e6155e51c9fe76e9c60f43))
* fix rockcraft.yaml ([b2c8d70](https://github.com/canonical/tenant-service/commit/b2c8d701f8c54ad9b0059c162e93a1fd4919b363))
* remove invite logic ([aedf304](https://github.com/canonical/tenant-service/commit/aedf3043e56264cdb9c4e3fb55ba596eadd97bb3))
* remove membership check ([6246bed](https://github.com/canonical/tenant-service/commit/6246bede085cac75ad65850634c8c0a64d00114e))
* update API ([59bdfd7](https://github.com/canonical/tenant-service/commit/59bdfd742a8d96d04bd8218adb9c664911a788d3))
* update kratos config ([7b4c839](https://github.com/canonical/tenant-service/commit/7b4c8390a273182d009df94555ff2f90d92e2100))
* validate request params ([bd5d8ce](https://github.com/canonical/tenant-service/commit/bd5d8ceb0562aac52a60713a524ad25d97643595))

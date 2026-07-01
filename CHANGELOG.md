# Changelog

## [1.5.0](https://github.com/pkuehne/dots/compare/v1.4.0...v1.5.0) (2026-07-01)


### Features

* install full archive tree with install_dir ([#44](https://github.com/pkuehne/dots/issues/44)) ([bad0fd6](https://github.com/pkuehne/dots/commit/bad0fd6e81964104f49659023a029060b47e78ab))


### Bug Fixes

* remove stale shell snippets on apply ([#47](https://github.com/pkuehne/dots/issues/47)) ([668f2e1](https://github.com/pkuehne/dots/commit/668f2e10f5fcfd7edd15eb3cbb47130d2f4adcbb))

## [1.4.0](https://github.com/pkuehne/dots/compare/v1.3.0...v1.4.0) (2026-06-25)


### Features

* docker-style live progress for tools/repos update & install ([#39](https://github.com/pkuehne/dots/issues/39)) ([6cf9f30](https://github.com/pkuehne/dots/commit/6cf9f301a010bca4e6703ef267bb3c18232ea050))
* live concurrent progress for apply file deploy and repo clone ([#41](https://github.com/pkuehne/dots/issues/41)) ([7824d0b](https://github.com/pkuehne/dots/commit/7824d0b6ac60e069b1c3d7a3c0045510135fb353))


### Bug Fixes

* render shell, git, ssh, presets sections like files/tools ([#40](https://github.com/pkuehne/dots/issues/40)) ([0c9d079](https://github.com/pkuehne/dots/commit/0c9d0799b1260f8751fca8ffd1e50f84300a3cec))

## [1.3.0](https://github.com/pkuehne/dots/compare/v1.2.0...v1.3.0) (2026-06-25)


### Features

* track installed tool versions and assert repo refs ([#37](https://github.com/pkuehne/dots/issues/37)) ([aeb4e5c](https://github.com/pkuehne/dots/commit/aeb4e5cfa790a79c4c487a173153468752a10f61))


### Bug Fixes

* group apply output into consistent labeled sections ([#36](https://github.com/pkuehne/dots/issues/36)) ([15e2c10](https://github.com/pkuehne/dots/commit/15e2c10cb835bce392c7a9fa7ee4f360e62f7f3f))

## [1.2.0](https://github.com/pkuehne/dots/compare/v1.1.0...v1.2.0) (2026-06-24)


### Features

* add dots upgrade for binary self-upgrade ([#33](https://github.com/pkuehne/dots/issues/33)) ([c4e8f1b](https://github.com/pkuehne/dots/commit/c4e8f1be49632e250ff28841071b9710147ba424))
* colour shell and snippet output ([#34](https://github.com/pkuehne/dots/issues/34)) ([453ab09](https://github.com/pkuehne/dots/commit/453ab09991955648f35c2a9b2920129dadf5c5b2))

## [1.1.0](https://github.com/pkuehne/dots/compare/dots-v1.0.1...dots-v1.1.0) (2026-06-23)


### Features

* improve apply output — per-file status, drop templating call-out ([#21](https://github.com/pkuehne/dots/issues/21)) ([5bb31b4](https://github.com/pkuehne/dots/commit/5bb31b45446c2e6a7a6fb79499f2b8f363f93b54))
* report tool installs during apply ([#26](https://github.com/pkuehne/dots/issues/26)) ([#30](https://github.com/pkuehne/dots/issues/30)) ([b5f6c7f](https://github.com/pkuehne/dots/commit/b5f6c7ff41a3ce1066cb0e4663601a8a00725ff5))
* use filippo.io/age library instead of age binary ([#27](https://github.com/pkuehne/dots/issues/27)) ([8be32ba](https://github.com/pkuehne/dots/commit/8be32ba1215f934626bb1449b39d6d68a98e8d7d)), closes [#23](https://github.com/pkuehne/dots/issues/23)
* use mholt/archives for tool extraction ([#28](https://github.com/pkuehne/dots/issues/28)) ([2f15ad6](https://github.com/pkuehne/dots/commit/2f15ad649dd5a5118ac8adef48d7c8ed6447db29))


### Bug Fixes

* follow symlinks when ensuring parent directories ([#20](https://github.com/pkuehne/dots/issues/20)) ([7dfddeb](https://github.com/pkuehne/dots/commit/7dfddebaa3ce545823c671fe9c34a59dca506ecb))
* parse inline array-of-tables and validate config strictly ([#31](https://github.com/pkuehne/dots/issues/31)) ([edea0ec](https://github.com/pkuehne/dots/commit/edea0ecb6dd1d7f76191d9945a47d9e2e2f70e8b)), closes [#29](https://github.com/pkuehne/dots/issues/29)

## [1.0.1](https://github.com/pkuehne/dots/compare/dots-v1.0.0...dots-v1.0.1) (2026-06-21)


### Bug Fixes

* ship prebuilt binaries with releases ([#17](https://github.com/pkuehne/dots/issues/17)) ([a61a198](https://github.com/pkuehne/dots/commit/a61a198713f0321a92cd6abd8b27a14d4e0cc9d2))

## [1.0.0](https://github.com/pkuehne/dots/compare/dots-v0.1.0...dots-v1.0.0) (2026-06-21)


### ⚠ BREAKING CHANGES

* migrate dots from Python to Go ([#10](https://github.com/pkuehne/dots/issues/10))

### Features

* add arch_map for per-tool architecture name overrides ([#7](https://github.com/pkuehne/dots/issues/7)) ([ff7c7e1](https://github.com/pkuehne/dots/commit/ff7c7e1c1650d0e49468597d82442e9f33e98178))
* migrate dots from Python to Go ([#10](https://github.com/pkuehne/dots/issues/10)) ([9f84594](https://github.com/pkuehne/dots/commit/9f845946363bd2f6a2f90f545ecf1d15f6ca7ea6))
* run tool installs as part of dots apply ([#6](https://github.com/pkuehne/dots/issues/6)) ([b939562](https://github.com/pkuehne/dots/commit/b939562cf1eea57308496962db2abef254556417))


### Bug Fixes

* add missing issues permission and opt into Node.js 24 for release workflow ([#2](https://github.com/pkuehne/dots/issues/2)) ([e63a0d0](https://github.com/pkuehne/dots/commit/e63a0d0f880e350a8b5ff5867b5eda65189968c4))
* correct GitHub username and raw URL in install script and README ([#4](https://github.com/pkuehne/dots/issues/4)) ([65aaa2c](https://github.com/pkuehne/dots/commit/65aaa2c4db2b4d0ab99c019c08f56d0841334239))
* create rc file and handle broken symlinks in idempotent_insert ([#5](https://github.com/pkuehne/dots/issues/5)) ([af18827](https://github.com/pkuehne/dots/commit/af1882739888484367fd9b7289e6b661c66b9b9f))
* generate per-shell snippets when init uses {shell} placeholder ([#9](https://github.com/pkuehne/dots/issues/9)) ([4e8ba6d](https://github.com/pkuehne/dots/commit/4e8ba6db811f84baff01b8da434fb439cbc80d7a))
* smarter binary extraction from github release archives ([#8](https://github.com/pkuehne/dots/issues/8)) ([9d293a6](https://github.com/pkuehne/dots/commit/9d293a65393e1a9fa9c7d72f3122f3af17256ba9))

## Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!-- release-please-start -->
<!-- release-please-end -->

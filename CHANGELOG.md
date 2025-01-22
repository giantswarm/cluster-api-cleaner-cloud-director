# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Migrate to App Build Suite.
- Update CircleCI config to use app-build-suite executor.

## [0.5.0] - 2024-07-25

### Changed

- Update renovate to json5 config.
- Upgrade `k8s.io/api`, `k8s.io/client-go` and `k8s.io/apimachinery` from `0.24.1` to `0.29.3`
- Upgrade `sigs.k8s.io/cluster-api` from `1.1.4` to `1.6.5`
- Upgrade `sigs.k8s.io/cluster-api-provider-cloud-director` from `0.0.0-20221214193317-51dffb617a19` to `1.3.0`
- Upgrade `sigs.k8s.io/controller-runtime` from `0.12.1` to `0.17.3`

## [0.4.2] - 2024-03-12

### Changed

- Remove finalizers if Status.InfraId is empty.

## [0.4.1] - 2024-03-08

### Changed

- Ignore CVE-2024-24786 until 2024-07-01 (low risk CVE).

## [0.4.0] - 2024-03-08

### Added

- PSS compliancy

### Changed

- Configure `gsoci.azurecr.io` as the default container image registry.

## [0.3.1] - 2023-08-24

### Changed

- Extend ignore for CVE-2020-8561.
- Ignore CVE-2023-3978 & CVE-2023-32731.

## [0.3.0] - 2023-05-02

### Added

- Add cleaner for Application Port Profiles.
- Add `compatibleProviders` to `Chart.yaml`.
- Add use of runtime/default seccomp profile.
- Add cleaner for NamedDisks(Volumes).

## [0.2.0] - 2022-11-24

### Added

- Add cleaner for Load Balancer Pools.
- Add cleaner for DNAT rules.

## [0.1.0] - 2022-11-21


[Unreleased]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/cluster-api-cleaner-cloud-director/releases/tag/v0.1.0

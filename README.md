# ecs

Sugary helper to retrieve high level EC2 instance information from your ECS cluster.

[![Apache 2.0 License](https://img.shields.io/badge/License-Apache%202.0-blue)](./LICENSE)
![Go Version](https://img.shields.io/github/go-mod/go-version/jimschubert/ecs)
![Go](https://github.com/jimschubert/ecs/workflows/Build/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/jimschubert/ecs)](https://goreportcard.com/report/github.com/jimschubert/ecs)
<!-- [![codecov](https://codecov.io/gh/jimschubert/ecs/branch/master/graph/badge.svg)](https://codecov.io/gh/jimschubert/ecs) --> 

## Installation

Latest binary releases are available via [GitHub Releases](https://github.com/jimschubert/ecs/releases).

Mac binaries are not signed and will display a warning about the binary being from an unidentified developer. After downloading, 
right-click/two-finger-click on the file and select "Open". If you've already received a warning, go to System Preferences and 
click "Allow" on the notification in the General tab.

This tool supports the same configurations as AWS CLI/SDK and assumes you have these configured.
Use [environment variables](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html) to configure this tool 
where appropriate. For example, to use a specific profile and default region to query your clusters:

```bash
export AWS_DEFAULT_PROFILE=custom
export AWS_DEFAULT_REGION=us-east-1
./ecs
```

## Build

Build a local distribution for evaluation using goreleaser.

```bash
goreleaser release --skip-publish --snapshot --rm-dist
```

This will create an executable application for your os/architecture under `dist`:

```
dist
├── checksums.txt
├── config.yaml
├── ecs_darwin_amd64
│   └── ecs
├── ecs_linux_386
│   └── ecs
├── ecs_linux_amd64
│   └── ecs
├── ecs_linux_arm64
│   └── ecs
├── ecs_linux_arm_6
│   └── ecs
```

Build and execute locally:

* Get dependencies
```shell
go get -d ./...
```
* Build
```shell
go build cmd/main.go
```
* Run
```shell
./main
```

## License

This project is [licensed](./LICENSE) under Apache 2.0.

# 开发与发布

本文面向维护者。用户安装和使用请见 [README.md](README.md)。

## 环境

- Go 1.25+
- Node.js 20+（仅在构建/发布 npm 包时需要）
- macOS / Linux（Windows 构建已由 CI 覆盖）

## 常用命令

```bash
make build        # 构建 bin/pmock-cli
make test         # go test -race ./...
make vet
make fmt          # gofmt 检查
make check        # fmt + vet + test
make package      # 生成 5 个平台压缩包 + checksums.txt
make npm          # 生成 npm 平台包 + wrapper
make snapshot     # GoReleaser 本地快照
```

## 目录

```text
main.go                  # go:embed openapi/*.yaml
cmd/                     # Cobra 命令树、MCP Server、doctor
internal/spec/           # OpenAPI 解析、overlay、命令/flags 生成
internal/client/         # AK/SK 签名、HTTP、重试、信封
internal/config/         # ~/.pmock-cli/config.json
internal/output/         # 分页折叠、脱敏、json/pretty/table
internal/phonefile/      # 多段上传
openapi/                 # vendored spec + overlay.yaml
skills/                  # 预留
scripts/                 # install / package / build-npm / publish-npm
```

## spec 驱动

`openapi/pmock-openapi.yaml` 是唯一事实源，`openapi/overlay.yaml` 提供：

- 用户可见命令路径、别名
- 风险等级：`read` / `write` / `costly` / `destructive`
- 异步策略：`phone` / `action` / `rpa`
- 请求体默认值，例如截图 `isRemove: true`
- 示例、内部命令隐藏

新增或修改接口：

1. 更新 `openapi/pmock-openapi.yaml`（`./scripts/update-spec.sh`）；
2. 在 `overlay.yaml` 增加/调整映射；
3. 运行 `go test ./internal/spec`；
4. 如有必要补充 `cmd` 或 `internal` 的测试。

`internal/spec/catalog_test.go` 会强制 60 个 operation 与 overlay 一一对应，
漏项或重复都会失败。

## 测试

```bash
go test -race -count=1 ./...
```

覆盖：

- `auth`：签名固定向量
- `client`：httptest 校验签名头、查询编码、重试、业务错误、分页响应
- `spec`：60 个 operation / overlay 覆盖 / 关键命令 flag
- `cmd`：请求组装、`--json`、必填校验、`--wait`、MCP 工具/参数
- `phonefile`：MD5、分片、MinIO/OBS mergeList、断点续传
- `output`：分页折叠、脱敏、JSON 转义、表格、中文宽度

真实 API 冒烟需要对应环境的 AK/SK，建议只读：

```bash
export PMOCK_BASE_URL=https://user.ts.pmock.cn/api
export PMOCK_ACCESS_KEY=...
export PMOCK_SECRET_KEY=...
pmock-cli doctor
pmock-cli phone list --page-size 1
```

## MCP

`cmd/mcp.go` 把 `spec.Catalog` 映射为 12 个分组 tool，每个 action 通过
in-process CLI 执行。新增 action 时不需要另写一套 API 客户端，只要保证
命令在 catalog 中可见。

MCP SDK 把工具参数解码进 `map[string]any`，JSON 数字一律变成 `float64`。
手机、任务等 64 位 ID 超过 2^53 后会在这一步丢精度，所以约定调用方用
字符串传 ID（`"id": "1900000000000000123"`）；`formatMCPParam` 对 ≥2^53 的
数字直接报错，避免把四舍五入后的 ID 发给 API。改参数处理时请保留该保护，
并覆盖 `TestBuildCLIArgsLargeIDs`。

## 发布

### GitHub Release

```bash
git tag v0.1.4
git push origin v0.1.4
```

`release.yml` 调用 GoReleaser 生成：

- darwin/arm64、darwin/amd64
- linux/amd64、linux/arm64
- windows/amd64
- `checksums.txt`

### npm

```bash
VERSION=0.1.4 make npm
VERSION=0.1.4 ./scripts/publish-npm.sh
```

或配置 GitHub Secret `NPM_TOKEN`，手动运行 `npm.yml`。
发布脚本是幂等的：已存在的版本跳过；单个平台失败不会阻塞 wrapper。

当前已知限制：`pmock-cli-win32-x64` 被 npm spam 审核拦截，Windows 暂时使用 zip。

### Homebrew

`Formula/pmock-cli.rb` 使用 Release 的 checksum。发布新版本后同步更新
version 和 4 个 sha256。

## 代码约定

- Go 代码通过 `gofmt`，提交前跑 `make check`
- 新增用户可见文案保持中文；命令/flag 使用英文
- 不在日志、错误、测试快照里输出真实 AK/SK
- 修改输出层时确认脱敏字段仍然生效（`password`、`adbPassword`、`accessKey`、`token`）
- 破坏性命令必须在 overlay 标 `destructive`，并在需要时要求 `--yes`

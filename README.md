# pmock-cli

[![CI](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/ci.yml)
[![Release](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml/badge.svg)](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/haidaisiganbei/pmock-cli.svg)](https://pkg.go.dev/github.com/haidaisiganbei/pmock-cli)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

PMOCK 开放 API 的独立命令行客户端。使用团队 **Access Key / Secret Key** 直接调用
`https://user.pmock.com/api/open/api/*`，不依赖 PMOCK 桌面客户端、不依赖登录态，
可以运行在终端、CI、服务器和 AI Agent 中。

> 与随客户端安装的 `pmock` 命令区分：`pmock` 走本机 MCP / 登录态，只服务本机；
> `pmock-cli` 走开放 API + AK/SK，是可独立分发的二进制。

- 语言：Go 1.25+
- 框架：Cobra
- API 规范：[pmock-openapi.yaml](openapi/pmock-openapi.yaml)
- 许可：Apache-2.0
- 当前版本：**0.1.4**

---

## 特性

| 能力 | 说明 |
|---|---|
| 开放 API 覆盖 | 60 个 operation，覆盖手机、应用、网络、文件、RPA、商品、团队 |
| 命令目录 | 62 条（60 个 operation + `phone file upload` + `doctor`） |
| 鉴权 | AK/SK HMAC-SHA256 签名，支持环境变量和配置文件 |
| 输出 | JSON / pretty / table，统一信封和稳定退出码 |
| 大文件 | MD5 → 分片上传 → MinIO/OBS 合并 → 推送到手机 |
| 异步任务 | `--wait` 等待开关机、UI 动作、RPA 任务到终态 |
| MCP | 12 个分组 tool + `pmock_help`，共 63 个 action |
| 诊断 | `doctor` 检查配置、凭证、端点、连通性和延迟 |
| 安全 | 密钥掩码、配置文件 0600、破坏性操作 `--yes` |

---

## 安装

### npm（推荐，macOS / Linux）

```bash
npm install -g pmock-cli@latest
```

如果所在网络使用 npmmirror，安装时指定官方源：

```bash
npm install -g pmock-cli@latest --registry=https://registry.npmjs.org
```

> Windows 的 `pmock-cli-win32-x64` 仍在 npm 审核中，Windows 暂时使用 Release 中的 `.zip`。

### 一行安装（macOS / Linux，无需 npm）

```bash
curl -fsSL https://raw.githubusercontent.com/haidaisiganbei/pmock-cli/main/scripts/install.sh | sh
```

安装到 `~/.local/bin/pmock-cli`，自动校验 `checksums.txt`。

### Go install

```bash
go install github.com/haidaisiganbei/pmock-cli@latest
```

### 从源码

```bash
git clone https://github.com/haidaisiganbei/pmock-cli.git
cd pmock-cli
make build
./bin/pmock-cli --version
```

### 预编译产物

https://github.com/haidaisiganbei/pmock-cli/releases

---

## 快速开始

```bash
# 1. 配置凭证
pmock-cli config init

# 2. 验证
pmock-cli doctor
pmock-cli auth status

# 3. 只读查询
pmock-cli product list
pmock-cli phone list --page-num 1 --page-size 100
pmock-cli phone list --format table

# 4. 开关机
pmock-cli phone start --id 123 --wait
pmock-cli phone stop --id 123 --wait

# 5. 看屏与 UI 自动化
pmock-cli phone screen --id 123
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200 --wait
pmock-cli phone action input --code A1 --text "hello" --wait

# 6. 上传文件 / APK
pmock-cli phone file upload --id 123 --file ./video.mp4
pmock-cli phone file upload --id 123 --file ./app.apk --type app

# 7. RPA
pmock-cli rpa template list
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --wait
```

---

## 配置

### 交互式

```bash
pmock-cli config init
```

Secret 输入不回显。配置保存到 `~/.pmock-cli/config.json`，权限 `0600`。

### 非交互式

```bash
pmock-cli config init \
  --base-url "https://user.pmock.com/api" \
  --access-key "AK..." \
  --secret-key "SK..."
```

### 环境变量

```bash
export PMOCK_ACCESS_KEY="AK..."
export PMOCK_SECRET_KEY="SK..."
export PMOCK_BASE_URL="https://user.pmock.com/api"
export PMOCK_TIMEOUT="30s"
```

优先级：

```text
命令行 flag > 环境变量 > 配置文件 > 内置默认值
```

### 测试环境

```bash
pmock-cli config init --base-url "https://user.ts.pmock.cn/api"
```

测试环境和正式环境的团队、AK/SK 完全独立。切换环境时必须同时切换对应环境的密钥，
否则会返回 `10123 无效的开发者编号`。

### 多份配置

```bash
export PMOCK_CLI_CONFIG="$HOME/.pmock-cli/test.json"
pmock-cli config init --base-url "https://user.ts.pmock.cn/api"
```

之后固定使用：

```bash
export PMOCK_CLI_CONFIG="$HOME/.pmock-cli/test.json"
```

---

## 命令

| 命令组 | 内容 |
|---|---|
| `product` | 商品、SKU、价格 |
| `phone` | 手机列表、创建、启停、重启、重置、续费、截图、GPS、网络、授权 |
| `phone action` | 点击、双击、长按、滑动、拖动、输入、应用操作 |
| `phone file` | 文件扫描、下载、上传 |
| `app` | 应用列表、分类、安装、升级、启停 |
| `network` | 代理列表、网络检测 |
| `rpa` | 模板市场、团队模板、任务增删改查、停止、重试 |
| `team` | 团队成员 |
| `task` | 后台任务记录 |
| `auth` / `config` | 登录态检查、配置读写 |
| `doctor` | 本地诊断 |
| `mcp` | MCP Server |
| `docs` / `commands` | 命令文档和机器可读目录 |

完整目录：

```bash
pmock-cli commands | jq .
pmock-cli docs phone start
pmock-cli phone list --help
```

常用过滤：

```bash
pmock-cli phone list --name-or-code demo
pmock-cli phone list --status 1
pmock-cli phone list --group-id 123
pmock-cli phone list --tag-ids 1,2
```

---

## 输出与退出码

默认 stdout 是单个 JSON 信封，诊断信息走 stderr：

```json
{
  "traceId": "...",
  "code": 200,
  "msg": "查询成功",
  "data": {}
}
```

- `code = 0` 或 `code = 200` 都表示成功；
- 其他 code 表示失败，`msg` 为原因；
- 分页接口的 `total` / `rows` 会统一折叠到 `data`：

```json
{
  "data": {
    "total": 6,
    "rows": [ ... ]
  }
}
```

- `password`、`adbPassword`、`accessKey`、`token` 等字段输出前自动替换为 `****`。

`--format`：

```bash
pmock-cli phone list --format json
pmock-cli phone list --format pretty
pmock-cli phone list --format table
```

退出码：

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 业务失败（`code` 不是 0/200，信封仍输出） |
| 2 | 用法 / 配置错误（未发请求） |
| 3 | 运行时失败（网络、超时、5xx、AK/SK 被拒） |

`--allow-business-error` 可让业务失败也返回 0。

---

## 进阶

### 嵌套请求体

标量字段都有独立 flag；嵌套对象或对象数组用 `--json`：

```bash
pmock-cli phone create --spu-id 1 --sku-id 2 --num 1 \
  --json '{"tagIds":[1,2],"realResolution":true}'

pmock-cli rpa task add --template-id 9 --name daily --json @task.json
cat task.json | pmock-cli rpa task add --template-id 9 --name daily --json -
```

flag 会覆盖 `--json` 中的同名 key。

### 文件上传

```bash
pmock-cli phone file upload --id 123 --file ./video.mp4
pmock-cli phone file upload --id 123 --file ./app.apk --type app
```

- 自动计算 MD5；
- 默认 30 MiB 分片，`--chunk-size-mb` 支持 30–300；
- 支持服务端 `exitPartList` 断点续传；
- MinIO 服务端合并，OBS 自动收集 ETag；
- 默认推送到 `/sdcard/Download`，可用 `--dest` 覆盖；
- `--type app` 返回的 `envFileId` 可传给 `app install --file-id`。

### 异步等待

以下命令支持 `--wait`：

```text
phone start / stop / restart / reset / new
phone action click / double-click / long-click / swipe / drag / input / app
rpa task add
```

```bash
pmock-cli phone start --id 123 --wait
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --wait
```

结果写进同一个信封：

```json
{
  "taskId": 1,
  "wait": {
    "ok": true,
    "taskStatus": 2,
    "phoneStatus": 1,
    "attempts": 2,
    "waitedMs": 1200
  }
}
```

- 超时退出码 3；
- 任务失败退出码 1；
- `--wait-timeout` 默认 3 分钟；
- `--poll-interval` 默认 2 秒。

### 截图清理

`phone screen` 默认发送 `isRemove=true`，截图完成后删除云手机相册中的文件：

```bash
pmock-cli phone screen --id 123
```

需要保留截图文件时：

```bash
pmock-cli phone screen --id 123 --is-remove=false
```

### 调试

```bash
pmock-cli phone start --id 123 --dry-run
pmock-cli phone list --page-size 1 --verbose
```

`--dry-run` 只打印 method、URL 和 body，不发送请求。

---

## MCP Server

```json
{
  "mcpServers": {
    "pmock": {
      "command": "pmock-cli",
      "args": ["mcp"],
      "env": {
        "PMOCK_BASE_URL": "https://user.pmock.com/api",
        "PMOCK_ACCESS_KEY": "AK...",
        "PMOCK_SECRET_KEY": "SK..."
      }
    }
  }
}
```

- 12 个分组 tool：`pmock_phone`、`pmock_phone_action`、`pmock_phone_file`、`pmock_app`、`pmock_network`、`pmock_rpa_template`、`pmock_rpa_task`、`pmock_product`、`pmock_brand`、`pmock_team`、`pmock_task`、`pmock_account`
- 63 个 action
- `pmock_help` 返回任意命令的完整 flag 列表
- `params` 的 key 是去掉 `--` 的 flag
- 嵌套请求体放在 `params.json`
- 破坏性 action 需要 `params.yes = true`
- 失败结果带 `isError` 并附命令帮助

调用示例：

```json
{
  "action": "start",
  "params": { "id": 123, "wait": true }
}
```

```json
{
  "action": "add",
  "params": {
    "template-id": 9,
    "name": "daily",
    "json": { "taskList": [{ "phoneId": 123 }] }
  }
}
```

MCP 与 CLI 共用同一份 `spec.Catalog`，命令与 action 不会漂移。

---

## 诊断

```bash
pmock-cli doctor
```

检查：

- 配置文件是否存在、权限是否为 `0600`；
- AK/SK 是否可解析；
- base URL 是否合法；
- 一次轻量 API 调用是否成功，以及延迟和 traceId。

`data.ok=false` 时：

- 缺凭证：退出码 2；
- 网络或鉴权失败：退出码 3。

---

## 设计

```text
openapi/pmock-openapi.yaml   # 唯一事实源
openapi/overlay.yaml         # 命令名、别名、风险、异步、默认值、示例
        │
        ▼
internal/spec                # 解析 / 校验 / 生成 flags
        │
        ▼
internal/client              # AK/SK 签名、重试、信封解析
        │
        ▼
cmd                          # Cobra 命令树 + MCP Server
        │
        ├── internal/phonefile   # 分片上传
        └── internal/output      # 分页折叠、脱敏、格式化
```

采用 spec 驱动，而不是一个命令一个文件：

1. **不会漂移**：命令树、参数、help、MCP action 都来自同一份 spec；
   `internal/spec/catalog_test.go` 强制 60 个 operation 与 overlay 一一对应。
2. **新增接口只改两处**：spec 更新 + overlay 加一条映射，`go test` 会提示漏项。
3. **没有大量重复命令代码**：请求编码、类型转换、必填校验只有一份实现。
4. **保留人工控制**：命令命名、风险级别、示例、默认值由 overlay 决定。

---

## 开发与测试

```bash
make build        # 构建 bin/pmock-cli
make test         # go test -race ./...
make vet
make fmt          # gofmt 检查
make check        # fmt + vet + test
make package      # 生成 5 个平台压缩包和 checksums.txt
make npm          # 生成 npm 平台包和 wrapper
make snapshot     # GoReleaser 本地快照
```

测试不访问真实 API：

- `auth`：签名固定向量；
- `client`：httptest 校验签名头、查询编码、重试、业务错误、分页响应；
- `spec`：60 个 operation / overlay 覆盖 / 关键命令 flag；
- `cmd`：请求组装、`--json` 合并、必填校验、`--wait`、MCP 工具/参数；
- `phonefile`：MD5、分片、MinIO/OBS mergeList、断点续传；
- `output`：分页折叠、脱敏、JSON 转义、表格、中文宽度。

更新上游 OpenAPI：

```bash
./scripts/update-spec.sh
go test ./internal/spec
```

---

## 发布

```bash
VERSION=0.1.4 make package
```

输出：

```text
dist/pmock-cli_0.1.4_darwin_arm64.tar.gz
dist/pmock-cli_0.1.4_darwin_amd64.tar.gz
dist/pmock-cli_0.1.4_linux_amd64.tar.gz
dist/pmock-cli_0.1.4_linux_arm64.tar.gz
dist/pmock-cli_0.1.4_windows_amd64.zip
dist/checksums.txt
```

GitHub Release：

```bash
git tag v0.1.4
git push origin v0.1.4
```

[.github/workflows/release.yml](.github/workflows/release.yml) 调用 GoReleaser 生成产物并创建 Release。

npm：

```bash
VERSION=0.1.4 make npm
VERSION=0.1.4 ./scripts/publish-npm.sh
```

或在 GitHub 仓库配置 `NPM_TOKEN` Secret，然后手动运行
[.github/workflows/npm.yml](.github/workflows/npm.yml)。

Homebrew：

```bash
brew install --formula ./Formula/pmock-cli.rb
```

---

## 安全

- 配置文件默认 `0600`，目录 `0700`；
- CI 中使用 `PMOCK_ACCESS_KEY` / `PMOCK_SECRET_KEY`，不要写进仓库；
- `remove`、`reset`、`new`、RPA 删除类命令需要 `--yes`；
- `create`、`renew` 会打印花钱警告；
- CLI 不提供任意 shell、任意 ADB 透传、任意后端路径代理；
- 输出会脱敏 `password`、`adbPassword`、`accessKey`、`token` 等字段；
- AK/SK 是团队维度，切换团队或环境后需要重新配置。

---

## Roadmap

- Windows npm 平台包审核跟进
- Homebrew tap
- Skills 文档生成
- Docker 镜像
- 批量开关机、按名称唯一匹配
- RPA 高层封装

---

## License

Apache-2.0，见 [LICENSE](LICENSE)。

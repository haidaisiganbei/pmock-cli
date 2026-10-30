# pmock-cli

[![Release](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml/badge.svg)](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

PMOCK 开放 API 的命令行客户端。用团队的 **Access Key / Secret Key** 直接管理云手机、
应用、文件、网络和 RPA 任务，不需要打开客户端，也不需要本机登录态。

适合：

- 在终端批量管理云手机
- 写自动化脚本 / CI 任务
- 给 AI Agent 提供可执行的 PMOCK 工具（MCP）

---

## 安装

### npm（macOS / Linux）

```bash
npm install -g pmock-cli@latest
```

国内镜像如果还没同步，指定官方源：

```bash
npm install -g pmock-cli@latest --registry=https://registry.npmjs.org
```

验证：

```bash
pmock-cli version
```

### 一行安装（macOS / Linux，不需要 npm）

```bash
curl -fsSL https://raw.githubusercontent.com/haidaisiganbei/pmock-cli/main/scripts/install.sh | sh
```

安装到 `~/.local/bin/pmock-cli`，并自动校验 checksums。

### Windows

Windows 暂时使用 GitHub Release 中的 `.zip`：

https://github.com/haidaisiganbei/pmock-cli/releases

### 其他安装方式

```bash
go install github.com/haidaisiganbei/pmock-cli@latest
```

---

## 五分钟上手

```bash
# 1. 配置 AK/SK
pmock-cli config init

# 2. 检查配置和连通性
pmock-cli doctor

# 3. 查询设备
pmock-cli phone list --page-num 1 --page-size 100

# 4. 表格查看
pmock-cli phone list --format table
```

### 使用测试环境

```bash
pmock-cli config init --base-url "https://user.ts.pmock.cn/api"
```

测试环境和正式环境的团队、AK/SK 完全独立。切换环境时，AK/SK 也要使用对应环境的，
否则会返回 `10123 无效的开发者编号`。

---

## 配置

### 交互式

```bash
pmock-cli config init
```

Secret 输入不会回显。配置保存在 `~/.pmock-cli/config.json`，权限 `0600`。

### 非交互式

```bash
pmock-cli config init \
  --base-url "https://user.pmock.com/api" \
  --access-key "AK..." \
  --secret-key "SK..."
```

### 环境变量

CI 中推荐使用环境变量：

```bash
export PMOCK_ACCESS_KEY="AK..."
export PMOCK_SECRET_KEY="SK..."
export PMOCK_BASE_URL="https://user.pmock.com/api"
export PMOCK_TIMEOUT="30s"
```

优先级：

```text
命令行参数 > 环境变量 > 配置文件 > 默认值
```

### 多套配置

```bash
export PMOCK_CLI_CONFIG="$HOME/.pmock-cli/test.json"
pmock-cli config init --base-url "https://user.ts.pmock.cn/api"
```

之后固定使用：

```bash
export PMOCK_CLI_CONFIG="$HOME/.pmock-cli/test.json"
```

---

## 常用命令

| 命令组 | 用途 |
|---|---|
| `phone` | 云手机列表、创建、启停、重启、重置、续费、截图、GPS |
| `phone action` | 点击、双击、长按、滑动、拖动、输入、应用操作 |
| `phone file` | 文件扫描、下载、上传 |
| `app` | 应用列表、安装、升级、启停 |
| `network` | 代理与网络检测 |
| `rpa` | RPA 模板与任务 |
| `product` | 商品、SKU、价格 |
| `team` | 团队成员 |
| `auth` / `config` | 鉴权与配置 |
| `doctor` | 诊断 |
| `mcp` | MCP Server |

### 云手机

```bash
# 列表
pmock-cli phone list --page-size 100
pmock-cli phone list --name-or-code demo
pmock-cli phone list --status 1
pmock-cli phone list --group-id 123

# 开关机 / 重启
pmock-cli phone start --id 123 --wait
pmock-cli phone stop --id 123 --wait
pmock-cli phone restart --id 123 --wait

# 截图（默认删除手机中的截图文件）
pmock-cli phone screen --id 123
pmock-cli phone screen --id 123 --is-remove=false

# 需要花钱或不可逆的操作
pmock-cli phone create --spu-id 1 --sku-id 2 --num 1
pmock-cli phone remove --id 123 --yes
```

### UI 自动化

动作类命令使用 **code**，不是 id：

```bash
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200 --wait
pmock-cli phone action input --code A1 --text "hello" --wait
pmock-cli phone action swipe --code A1 --pos1 100,200 --pos2 100,600
```

### 文件上传

```bash
pmock-cli phone file upload --id 123 --file ./video.mp4
pmock-cli phone file upload --id 123 --file ./app.apk --type app
```

- 自动计算 MD5
- 自动分片上传
- 默认推送到 `/sdcard/Download`
- `--dest` 可指定手机目录
- `--type app` 返回的 `envFileId` 可传给 `app install --file-id`

### 应用

```bash
pmock-cli app list --page-size 10 --apk-name tiktok
pmock-cli app install --id 123 --file-id 456 --apk-pkg com.example.app \
  --apk-name Example --apk-version-name 1.0 --apk-version-code 1
```

### RPA

```bash
pmock-cli rpa template list
pmock-cli rpa task list
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --wait
```

更多命令：

```bash
pmock-cli --help
pmock-cli phone --help
pmock-cli docs phone start
pmock-cli commands
```

---

## 输出

默认输出 JSON：

```json
{
  "traceId": "...",
  "code": 200,
  "msg": "查询成功",
  "data": {}
}
```

- `code` 为 `0` 或 `200` 表示成功
- 其他 code 表示失败，`msg` 是原因
- 分页接口统一输出为 `data.total` + `data.rows`

```json
{
  "data": {
    "total": 6,
    "rows": [ ... ]
  }
}
```

- `password`、`adbPassword`、`accessKey`、`token` 等字段会自动替换为 `****`

格式：

```bash
pmock-cli phone list --format json
pmock-cli phone list --format pretty
pmock-cli phone list --format table
```

退出码：

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 业务失败 |
| 2 | 参数 / 配置错误 |
| 3 | 网络、超时、鉴权失败 |

`--allow-business-error` 可以让业务失败也返回 0，适合只判断接口可达性的脚本。

---

## 更多用法

### 嵌套参数

标量参数用 flag，嵌套对象或数组用 `--json`：

```bash
pmock-cli phone create --spu-id 1 --sku-id 2 --num 1 \
  --json '{"tagIds":[1,2],"realResolution":true}'

pmock-cli rpa task add --template-id 9 --name daily --json @task.json
```

### 先看请求再执行

```bash
pmock-cli phone start --id 123 --dry-run
```

只输出 method、URL 和 body，不发送请求。

### 异步等待

```bash
pmock-cli phone start --id 123 --wait
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200 --wait
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --wait
```

结果会写进 `data.wait`：

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

---

## MCP（AI Agent 接入）

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

工具按模块分组，调用时用 `action` 选择动作，`params` 传参数：

```json
{
  "action": "start",
  "params": { "id": 123, "wait": true }
}
```

- 支持 `pmock_phone`、`pmock_phone_action`、`pmock_phone_file`、`pmock_app`、`pmock_network`、`pmock_rpa_template`、`pmock_rpa_task`、`pmock_product`、`pmock_brand`、`pmock_team`、`pmock_task`、`pmock_account`
- `pmock_help` 可以查询任意命令的完整参数
- 删除、重置等破坏性动作需要 `params.yes = true`

---

## 故障排查

### `10123 无效的开发者编号`

通常是环境或团队不匹配：

```bash
pmock-cli config show
```

确认：

- base URL 是测试还是正式
- AK/SK 是否属于同一环境的同一团队
- 客户端当前选中的团队是否一致

### 有设备但列表为空

先确认版本：

```bash
pmock-cli version
```

0.1.4 起已支持 `rows` 分页响应；旧版本可能显示 `data: null`。

### npm 安装后还是旧版本

```bash
npm install -g pmock-cli@latest --registry=https://registry.npmjs.org --prefer-online
pmock-cli version
```

### 配置文件权限

```bash
chmod 600 ~/.pmock-cli/config.json
```

### 诊断

```bash
pmock-cli doctor
```

---

## 安全

- AK/SK 是团队维度凭证，不要提交到仓库或发到公开渠道
- 配置文件默认 `0600`
- CI 使用 `PMOCK_ACCESS_KEY` / `PMOCK_SECRET_KEY`
- `remove`、`reset`、`new` 等不可逆操作需要 `--yes`
- 输出会隐藏密码、ADB 密码、token 等敏感字段
- 如果凭证泄露，请在 PMOCK 客户端中重置

---

## 链接

- 开放 API 文档：https://help.pmock.com/zh/api-docs/api-docs.html
- Releases：https://github.com/haidaisiganbei/pmock-cli/releases
- 问题反馈：https://github.com/haidaisiganbei/pmock-cli/issues

参与开发请见 [CONTRIBUTING.md](CONTRIBUTING.md)。

---

Apache-2.0，见 [LICENSE](LICENSE)。

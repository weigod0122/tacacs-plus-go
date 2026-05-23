# Contributing

感谢你考虑为本项目做贡献!本文档说明本地开发、提交规范、PR 流程与许可证约定。

---

## 行为准则

参与贡献即表示你同意保持友善、专业的协作氛围。请勿对人不对事。

---

## 本地开发

### 环境要求

| 依赖 | 版本 |
|---|---|
| Go | `>= 1.26` (与 `go.mod` 一致) |
| MySQL | `>= 5.7` 或 `8.0` (InnoDB) |
| Make | 任意现代版本 |
| OS | Linux / macOS (amd64 或 arm64) |

### 拉取与构建

```bash
git clone <your-fork-url>
cd tacacs
go mod download
make build            # 构建当前平台的 server / client / swm
```

二进制输出到 `build/<os>_<arch>/`。

### 跑测试

```bash
go test ./...
go vet ./...
gofmt -l .            # 输出非空表示有未格式化文件
```

提交前请确保上述三条全部通过。

### 本地起一套环境

参考 [README 「🚀 快速开始」](README.md#-快速开始) 配 MySQL + 三份 yaml,然后:

```bash
./scripts/deploy.sh start server
./scripts/deploy.sh start client
./scripts/deploy.sh start swm
```

---

## 提交规范

### Commit Message 格式

本仓库采用 [Conventional Commits](https://www.conventionalcommits.org/) 风格(中英文均可):

```
<type>(<scope>): <subject>

<可选的详细描述>
```

**type** 取值:

| type | 用途 |
|---|---|
| `feat` | 新功能 |
| `fix` | bug 修复 |
| `refactor` | 重构(不改外部行为) |
| `perf` | 性能优化 |
| `chore` | 杂项(依赖升级、配置调整等) |
| `docs` | 文档变更 |
| `test` | 测试相关变更 |
| `ci` | CI 配置变更 |

**scope** 是可选的,推荐用模块名:`cfg` / `feishu` / `server` / `client` / `swm` / `tacplus` / `db` 等。

**示例**(参考现有 git 历史):

```
feat(cfg): 新增 manager 配置字段用于 panic 飞书通知
refactor(config): 将 Apollo 配置外置为 YAML 文件并支持本地降级
perf: 优化审批通知发送与权限更新响应速度
fix(client): 修复值班人员授权 bug
```

### 分支策略

- `master` 是稳定主分支,所有发布从这里 tag
- 在自己 fork 的 feature 分支上开发,命名建议:`feat/xxx` / `fix/xxx` / `refactor/xxx`
- 一个 PR 聚焦一件事,避免"顺手改一堆别的"

### 单次提交大小

- 一个 commit 做一件可独立描述的事
- 大的重构请拆成多个 commit,便于 review 和 bisect

---

## Pull Request 流程

1. **先开 issue 讨论**(新功能 / 不向后兼容的变更),小修小补可以直接提 PR
2. Fork → 创建分支 → 提交变更 → push
3. 在 GitHub 上发起 PR,目标分支 `master`
4. PR 描述请包含:
   - 这个 PR 解决什么问题
   - 怎么解决的(关键设计选择)
   - 测试覆盖(单元测试 / 手工验证步骤)
   - **是否会改变外部行为**(配置项、HTTP 接口、数据库 schema、协议格式)
5. CI 必须全绿(`go vet` / `go build` / `go test`)
6. 至少一位 maintainer review 通过后合并

### Reviewer 关注什么

- 正确性(逻辑、并发、错误处理)
- 安全(参考 [SECURITY.md 的 Scope](SECURITY.md#scope))
- 接口/配置/schema 的向后兼容
- 测试覆盖
- 代码风格与可读性

---

## 不建议的 PR 类型

- 纯格式化(空格 / 换行 / 注释样式)且无逻辑改动 — 易制造 conflict
- 引入大型第三方依赖且无强需求
- 同时混合多个无关变更
- 把全文档从中文改成英文(或反之),除非 maintainer 已规划国际化

---

## 涉及安全的改动

如果你的 PR 涉及鉴权 / 授权 / 加密 / 签名 / 凭据存储,请在 PR 描述中**显式说明**,以便单独 review。

发现**已存在**的安全漏洞请走 [SECURITY.md](SECURITY.md) 的私密披露流程,不要直接开 public issue / PR。

---

## License

本项目采用 [GNU General Public License v3.0](LICENSE)。**提交 PR 即视为你同意以 GPL v3.0 授权你的贡献**。

如果你的贡献来自所在公司,请确认你有授权代表公司提交。

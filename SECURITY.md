# Security Policy

## Supported Versions

只为最近一个发布版本提供安全更新。请保持升级到 `master` 上的最新 tag。

| Version | Supported |
|---|---|
| latest tagged release | ✅ |
| older tags / master HEAD between releases | ❌ |

## Reporting a Vulnerability

**请不要在公开 issue 中报告安全漏洞。**

漏洞披露**通过 GitHub Security Advisory 私密渠道**进行:

👉 **[Report a vulnerability](../../security/advisories/new)**

(也可以在仓库主页 → **Security** 标签 → **Report a vulnerability** 进入)

### 请在报告中尽量包含

- 受影响的组件(`server` / `client` / `swm`)与代码位置(文件:行号 或 commit hash)
- 受影响的版本范围
- 漏洞类型(认证绕过 / 命令注入 / 信息泄露 / 拒绝服务 / 越权 / 其他)
- 最小复现步骤或 PoC
- 潜在影响评估(攻击前置条件、权限要求、可触达资产)
- 你建议的修复方向(可选)

### 响应时间承诺

| 阶段 | 目标响应时间 |
|---|---|
| 收到报告确认 | 3 个工作日内 |
| 初步评估(确认是否成立、严重等级) | 7 个工作日内 |
| 修复 + 发布安全公告 | 严重 ≤ 30 天 / 高 ≤ 60 天 / 中低 ≤ 90 天 |

## Scope

### ✅ 在范围内

- 认证 / 授权绕过(SwM ACL、`swm_auth` HMAC 签名校验、TACACS+ 协议层)
- 注入类(SQL、命令、模板)
- 越权访问(普通用户读/改其他用户数据、非管理员调用管理员接口)
- 密码学误用(弱随机、密钥泄露、bcrypt 误用、TLS 配置降级)
- 反序列化 / 解析器漏洞
- 默认配置导致的不安全状态

### ❌ 不在范围内

- 在**违反 [README 的「⚠️ 部署前安全须知」](README.md#️-部署前安全须知)** 的部署环境下产生的问题(典型例子:`Server 8899 暴露公网`、`swm_auth.enforce: false`、`使用默认共享密钥`)
- 来自第三方依赖且上游已有公开 CVE 的问题(请直接升级依赖)
- 缺乏可达性证明的理论性弱点
- 仅影响自签开发证书的 TLS 警告
- 任何需要本地 root / 物理访问前提的攻击

## Public Disclosure

漏洞修复发布后,我们会:

1. 发布 GitHub Security Advisory(GHSA),包含 CVE(若申请到)、影响版本、修复版本
2. 在 [`CHANGELOG.md`](CHANGELOG.md) 标注 `security` 条目
3. 在 GitHub Release Notes 单独高亮

报告者若同意,我们会在公告中署名致谢。

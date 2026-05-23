package swm

import "embed"

// StaticFS 把 pkg/swm/static/ 整个目录在编译期打进二进制，运行时
// 由 pkg/swm/http 通过 http.FS / template.ParseFS 提供给 gin。
// `all:` 前缀确保 . 开头的文件也被打包（默认会跳过）。
//
//go:embed all:static
var StaticFS embed.FS

// 历史上这里还嵌入过 certificate/cert.pem + key.pem 作为默认自签证书,
// 但开源仓库里嵌默认证书等于让所有部署者共用同一把私钥,谁拿到源码都能 MITM,
// 比"裸 HTTP"还危险。现在策略改为:cfg.cert_file/key_file 双空就降级明文 HTTP
// (仅适合反代后端 / 浏览器用 http://localhost 自连开发),双填就走 HTTPS,
// 半填直接拒绝启动。详见 pkg/swm/http/http.go 顶部对 Secure cookie 与
// secure context 关系的说明。

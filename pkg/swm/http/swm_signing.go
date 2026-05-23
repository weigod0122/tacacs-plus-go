package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// swmSystemUser 是 SwM 进程内部直调 tacacs-server 时使用的伪用户名。
//
// 场景：登录验密、admin 列表缓存刷新、注册创建用户——这些请求由 SwM 自己发起，
// 还没有真实用户上下文（或不应代表某个真实用户）。共享密钥签名证明请求来自
// SwM 进程，X-SwM-Is-Admin=1 让后端的 admin-only 接口（比如 /get-admin）放行。
//
// 后端 handler 不读 X-SwM-User 头，仅审计日志会记录这个名字——能在日志里跟真实
// 用户调用区分开。
const swmSystemUser = "__swm-system__"

// computeSignatureHeader 计算 X-SwM-Signature 的完整头值（"t=...,n=...,v1=..."）。
//
// canonical 必须与 tacacs-server 的 verifySwmSignature 严格一致：
//
//	method\npath\nts\nnonce\nuser\nisAdmin\nsha256_hex(body)
//
// path 不带 query string。body 为空时 hash 是空字符串的 sha256。
func computeSignatureHeader(method, path, user, isAdmin string, body []byte, secret string) string {
	bodyHash := sha256.Sum256(body)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := newNonce()

	canonical := strings.Join([]string{
		method, path, ts, nonce, user, isAdmin,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("t=%s,n=%s,v1=%s", ts, nonce, sig)
}

func newNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极不应该发生；退化用纳秒时间戳确保至少不空
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

package http

import (
	"fmt"
	"net/url"
	"tacacs/pkg/public/cfg"
	httpclient "tacacs/pkg/public/httpclient"
)

// signedInternalGet / signedInternalPost 是 SwM 进程**自己**直调 tacacs-server 时
// 的入口（场景：登录验密、admin 列表缓存刷新、注册创建用户——这些请求不在反代
// TacacsProxyHandler 路径上，所以拿不到反代里的签名注入逻辑）。
//
// 用 system 身份（X-SwM-User="__swm-system__", X-SwM-Is-Admin="1"）+ 共享密钥
// HMAC 签名。后端中间件验签通过即信任 Is-Admin=1，让 admin-only 接口（如
// /tacacs/user/get-admin）放行；handler 不读 X-SwM-User，仅审计日志会留痕。

func signedInternalGet(fullURL string) ([]byte, error) {
	headers, err := buildSwmInternalHeaders("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	return httpclient.Get(fullURL, headers)
}

func signedInternalPost(fullURL string, body []byte) ([]byte, error) {
	headers, err := buildSwmInternalHeaders("POST", fullURL, body)
	if err != nil {
		return nil, err
	}
	return httpclient.Post(fullURL, headers, body)
}

func buildSwmInternalHeaders(method, fullURL string, body []byte) (map[string]string, error) {
	secret := cfg.SwmConfig().ResolveSwmSecret()
	if secret == "" {
		// 共享密钥未配置：不注入身份/签名头。后端 enforce=false 时仍能通过；
		// enforce=true 时被拒，让运维显式发现配置缺失。
		return nil, nil
	}
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	return map[string]string{
		"X-SwM-User":      swmSystemUser,
		"X-SwM-Is-Admin":  "1",
		"X-SwM-Signature": computeSignatureHeader(method, parsed.Path, swmSystemUser, "1", body, secret),
	}, nil
}

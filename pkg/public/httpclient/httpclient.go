// Package httpclient 提供共享连接池的 HTTP 客户端，是项目里所有外发 HTTP 调用的统一入口。
//
// 关键设计：
//   - 使用全局共享 *http.Client + *http.Transport，启用 keep-alive 与连接复用
//   - 用 context.WithTimeout 而不是 http.Client.Timeout，避免慢响应阻塞连接归还
//
// swm 反代调 tacacs-server / 后续 server/client 之间的 HTTP 互调都走这里。
package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"tacacs/pkg/public/log"
	"time"
)

var (
	clientOnce   sync.Once
	sharedClient *http.Client
)

// client 返回带连接池的全局共享 *http.Client。
// 配置 keep-alive、连接复用，避免每次请求新建 TCP 连接。
func client() *http.Client {
	clientOnce.Do(func() {
		sharedClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
	})
	return sharedClient
}

func GetWithTimeout(url string, header map[string]string, timeout int) ([]byte, error) {
	return request("GET", url, header, nil, timeout)
}

func Get(url string, header map[string]string) ([]byte, error) {
	return request("GET", url, header, nil, 15)
}

func Post(url string, header map[string]string, data []byte) ([]byte, error) {
	return request("POST", url, header, data, 15)
}

func Put(url string, header map[string]string, data []byte) ([]byte, error) {
	return request("PUT", url, header, data, 15)
}

func Delete(url string, header map[string]string) ([]byte, error) {
	return request("DELETE", url, header, nil, 15)
}

func PostRetry(attempts int, sleep time.Duration, url string, header map[string]string, data []byte) ([]byte, error) {
	return Retry(attempts, sleep, func() ([]byte, error) {
		return Post(url, header, data)
	})
}

func GetRetry(attempts int, sleep time.Duration, url string, header map[string]string) ([]byte, error) {
	return Retry(attempts, sleep, func() ([]byte, error) {
		return Get(url, header)
	})
}

func request(method, url string, header map[string]string, data []byte, timeout int) ([]byte, error) {
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return respBody, err
	}

	if resp.StatusCode != http.StatusOK {
		return respBody, fmt.Errorf("resp.StatusCode:%d", resp.StatusCode)
	}
	return respBody, nil
}

func Retry(attempts int, sleep time.Duration, callback func() ([]byte, error)) ([]byte, error) {
	var data []byte
	var err error
	for i := 0; ; i++ {
		data, err = callback()
		if err == nil {
			return data, nil
		}
		if i >= attempts-1 {
			break
		}
		time.Sleep(sleep)
		log.Logger.Errorf("retrying after error: %v", err)
	}
	return nil, fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var adminOnlyPrefixes = []string{
	"/tacacs/user/get-admin",
	"/tacacs/user/delete",
	"/tacacs/user/create",
	"/tacacs/user/clear/",
}

// adminWritePrefixes 是"读对所有人开放、写仅管理员"的路径前缀。命中此前缀的
// GET / HEAD / OPTIONS 不挡 ACL；POST / PUT / PATCH / DELETE 仍要 admin。
// /tacacs/system/ 下的配置写操作只对管理员开放；日志查询接口不在这里，
// 由 handler 根据当前模式和已验证身份再次校验；ClickHouse 直查的行级范围
// 由 handler 强制施加。
var adminWritePrefixes = []string{
	"/tacacs/system/",
}

var adminOnlyExact = map[string]struct{}{
	"/tacacs/user/reset/password": {},
	"/tacacs/meta/refresh":        {},
	"/tacacs/system/log-config":   {},
}

var ownershipBodyUserPaths = map[string]struct{}{
	"/tacacs/user/update/password":  {},
	"/tacacs/user/update/notes":     {},
	"/tacacs/user/update/basicInfo": {},
	"/tacacs/approval/create":       {},
}

const (
	approvalUpdatePath  = "/tacacs/approval/update"
	approvalStatusClose = 0
	maxBodyPeek         = 256 * 1024
)

func pathRequiresAdmin(path, method string) bool {
	if _, ok := adminOnlyExact[path]; ok {
		return true
	}
	for _, p := range adminOnlyPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	if hasBodyMethod(method) {
		for _, p := range adminWritePrefixes {
			if strings.HasPrefix(path, p) {
				return true
			}
		}
	}
	return false
}

func hasBodyMethod(m string) bool {
	return m == http.MethodPost || m == http.MethodPut || m == http.MethodDelete || m == http.MethodPatch
}

// readAndRestoreBody 读完 body 并把它还原成可重新读取的 ReadCloser，超过 maxBodyPeek
// 返回 nil（让后端兜底校验），避免内存炸。
func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	limited := io.LimitReader(r.Body, maxBodyPeek+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	if int64(len(body)) > maxBodyPeek {
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
		return nil, fmt.Errorf("body too large for peek")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	return body, nil
}

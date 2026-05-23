// Package clog 是 client 专属的日志器集合。
//
// 通过放在 pkg/client/internal/ 下，Go 编译器只允许 pkg/client 子树和 cmd/client
// import 本包，server/swm 误 import 会编译报错。
package clog

import (
	"fmt"
	"tacacs/pkg/public/log"

	gologger "github.com/weigod0122/go-logger"
)

// TacPlusAccountLogger 写 client_tac_plus_account.log：TACACS+ 协议 account 类型日志。
var TacPlusAccountLogger = gologger.NewLogger()

// TacPlusAuthenLogger 写 client_tac_plus_authen.log：TACACS+ 协议 authentication 类型日志。
var TacPlusAuthenLogger = gologger.NewLogger()

// TacPlusAuthorLogger 写 client_tac_plus_author.log：TACACS+ 协议 authorization 类型日志。
var TacPlusAuthorLogger = gologger.NewLogger()

// Init 把 client 专属 logger 落到 logPath 目录。logPath 由 utils.InitAppLog 已经创建过。
// 文件名带 client_ 前缀,与同目录下 server/swm 的日志区分。
func Init(logPath string) {
	logPath = log.NormalizeLogPath(logPath)

	_ = TacPlusAccountLogger.Detach("console")
	_ = TacPlusAccountLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/client_tac_plus_account.log", logPath),
		DateSlice: "h",
		Format:    "%body%",
	})

	_ = TacPlusAuthenLogger.Detach("console")
	_ = TacPlusAuthenLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/client_tac_plus_authen.log", logPath),
		DateSlice: "h",
		Format:    "%body%",
	})

	_ = TacPlusAuthorLogger.Detach("console")
	_ = TacPlusAuthorLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/client_tac_plus_author.log", logPath),
		DateSlice: "h",
		Format:    "%body%",
	})
}

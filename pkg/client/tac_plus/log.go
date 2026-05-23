package tac_plus

import (
	"tacacs/pkg/client/internal/clog"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/tacplus"
	"time"

	"github.com/bytedance/sonic"
)

func LogAccount(accountInfo *tacplus.AccountInfo) {
	accountInfo.Time = accountInfo.StartTime.Format("2006-01-02 15:04:05.000")
	accountInfo.TimeStamp = accountInfo.StartTime.UnixNano()
	accountInfo.TimeRange = time.Since(accountInfo.StartTime).Nanoseconds()
	accountInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(accountInfo)
	clog.TacPlusAccountLogger.Info(infoJSON)
}

func LogAuthen(authenInfo *tacplus.AuthenInfo) {
	authenInfo.Time = authenInfo.StartTime.Format("2006-01-02 15:04:05.000")
	authenInfo.TimeStamp = authenInfo.StartTime.UnixNano()
	authenInfo.TimeRange = time.Since(authenInfo.StartTime).Nanoseconds()
	authenInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(authenInfo)
	clog.TacPlusAuthenLogger.Info(infoJSON)
}

func LogAuthor(authorInfo *tacplus.AuthorInfo) {
	authorInfo.Time = authorInfo.StartTime.Format("2006-01-02 15:04:05.000")
	authorInfo.TimeStamp = authorInfo.StartTime.UnixNano()
	authorInfo.TimeRange = time.Since(authorInfo.StartTime).Nanoseconds()
	authorInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(authorInfo)
	clog.TacPlusAuthorLogger.Info(infoJSON)
}

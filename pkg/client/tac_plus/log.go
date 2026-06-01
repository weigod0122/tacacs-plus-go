package tac_plus

import (
	"context"
	"tacacs/pkg/client/internal/clog"
	"tacacs/pkg/client/logHub"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/tacplus"
	"time"

	"github.com/bytedance/sonic"
)

func LogAccount(ctx context.Context, accountInfo *tacplus.AccountInfo) {
	startLog := time.Now()
	accountInfo.Time = accountInfo.StartTime.Format("2006-01-02 15:04:05.000")
	accountInfo.TimeStamp = accountInfo.StartTime.UnixNano()
	accountInfo.TimeRange = time.Since(accountInfo.StartTime).Nanoseconds()
	accountInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(accountInfo)
	clog.TacPlusAccountLogger.Info(infoJSON)
	logHub.LogAccount(ctx, accountInfo)
	if env.DEBUG {
		log.DebugLog("TacPlus Account Info time cost:", time.Since(startLog))
	}
}

func LogAuthen(ctx context.Context, authenInfo *tacplus.AuthenInfo) {
	startLog := time.Now()
	authenInfo.Time = authenInfo.StartTime.Format("2006-01-02 15:04:05.000")
	authenInfo.TimeStamp = authenInfo.StartTime.UnixNano()
	authenInfo.TimeRange = time.Since(authenInfo.StartTime).Nanoseconds()
	authenInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(authenInfo)
	clog.TacPlusAuthenLogger.Info(infoJSON)
	logHub.LogAuthen(ctx, authenInfo)
	if env.DEBUG {
		log.DebugLog("TacPlus Authentication Info time cost:", time.Since(startLog))
	}
}

func LogAuthor(ctx context.Context, authorInfo *tacplus.AuthorInfo) {
	startLog := time.Now()
	authorInfo.Time = authorInfo.StartTime.Format("2006-01-02 15:04:05.000")
	authorInfo.TimeStamp = authorInfo.StartTime.UnixNano()
	authorInfo.TimeRange = time.Since(authorInfo.StartTime).Nanoseconds()
	authorInfo.TacacsClient = env.HostName
	infoJSON, _ := sonic.MarshalString(authorInfo)
	clog.TacPlusAuthorLogger.Info(infoJSON)
	logHub.LogAuthor(ctx, authorInfo)
	if env.DEBUG {
		log.DebugLog("TacPlus Authorization Info time cost:", time.Since(startLog))
	}
}

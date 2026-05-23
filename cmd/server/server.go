package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"
	allWg "tacacs/pkg/public/waitGroup"
	tacacsserver "tacacs/pkg/server"
	"tacacs/pkg/server/approvalSystem"
	"tacacs/pkg/server/feishuws"
	"tacacs/pkg/server/http"
	"tacacs/pkg/server/permissionSystem"
)

var (
	Signals = make(chan os.Signal, 1)
	as      *approvalSystem.ApprovalSystem
	ps      *permissionSystem.PermissionSystem
	fws     *feishuws.Service
)

func main() {
	item := "server"

	if err := env.ParameterInitialization(item); err != nil {
		fmt.Printf("init item failed:%v", err)
		return
	}

	//解析配置文件
	cfgFile := flag.String("c", "cfg.yaml", "configuration file")
	flag.Parse()
	parseConfigErr := cfg.ParseConfig(item, *cfgFile)
	if parseConfigErr != nil {
		fmt.Printf("parse config file failed:%v", parseConfigErr)
		return
	}

	initLoggerErr := log.InitAppLog(item, cfg.ServerConfig().LogFilePath)
	if initLoggerErr != nil {
		fmt.Printf("init logger failed:%v", initLoggerErr)
		return
	}
	tacacsserver.InitLog(cfg.ServerConfig().LogFilePath)

	//数据库连接
	err := db.Init()
	if err != nil {
		log.Logger.Errorf("open db fail:%v", err)
		return
	}

	//http启动
	http.Start(cfg.ServerConfig().Http)

	//权限审批系统启动
	as = approvalSystem.NewApprovalSystem()
	as.Start()

	//权限管理系统启动
	ps = permissionSystem.NewPermissionSystem()
	ps.Start()

	//飞书长连接（卡片按钮回调）启动；cfg.Feishu.Enabled=false 或凭据空时是 no-op。
	fws = feishuws.New()
	if err := fws.Start(); err != nil {
		log.Logger.Errorf("feishu ws start failed: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			body := fmt.Sprintf("**主机**：%v\n**tacacs_server panic**：%v", env.HostName, r)
			log.Logger.Info(strings.Replace(body, "\n", ",", 1))
			sendUrgentCardToPersonByUserIdErr := feishu.SendUrgentCardToPersonByUserId(cfg.ServerConfig().Manager, feishu.BuildSystemAlertCard("TACACS Server 进程崩溃", body, "red"), feishu.UrgentApp)
			if sendUrgentCardToPersonByUserIdErr != nil {
				log.Logger.Errorf("SendUrgentCardToPersonByUserId err:%v", sendUrgentCardToPersonByUserIdErr)
			}
		}
	}()

	// READY 标记必须在所有初始化完成、即将进入信号等待之前打印,
	// 走 stdout 是因为 InitAppLog 已经 Detach 了 console,log.Logger 只会写文件。
	// nohup 把 stdout 重定向到 .out 文件,deploy.sh grep 这一行就能在 1-2s 内
	// 确认起服成功,不用死等 12s 观察窗口。
	fmt.Println("READY: server initialized, awaiting signals")

	//优雅退出
	catchStopSignalServer()

}

func catchStopSignalServer() {
	signal.Notify(Signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		s := <-Signals
		log.Logger.Infof("catch signals which is %v", s)
		switch s {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			http.Stop()
			as.Stop()
			ps.Stop()
			if fws != nil {
				fws.Stop()
			}
			allWg.GlobalWg.Wait()
			log.Logger.Info("system stop")
			return
		}
	}
}

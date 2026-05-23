package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	tacacsclient "tacacs/pkg/client"
	"tacacs/pkg/client/http"
	"tacacs/pkg/client/tac_plus"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"
)

var (
	Signals = make(chan os.Signal, 1)
	tps     *tac_plus.TacPlusSystem
)

func main() {
	item := "client"

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

	initLoggerErr := log.InitAppLog(item, cfg.ClientConfig().LogFilePath)
	if initLoggerErr != nil {
		fmt.Printf("init logger failed:%v", initLoggerErr)
		return
	}
	tacacsclient.InitLog(cfg.ClientConfig().LogFilePath)

	//数据库连接
	err := db.Init()
	if err != nil {
		log.Logger.Errorf("open db fail:%v", err)
		return
	}

	//tacacs认证服务启动
	tps = tac_plus.NewTacPlusSystem()
	tpsStartErr := tps.Start()
	if tpsStartErr != nil {
		log.Logger.Errorf("tps start failed:%v", tpsStartErr)
		return
	}
	log.Logger.Infof("tacacs system start success")

	//http启动
	http.Start(cfg.ClientConfig().Http)

	defer func() {
		if r := recover(); r != nil {
			body := fmt.Sprintf("**主机**：%v\n**tacacs_client panic**：%v", env.HostName, r)
			log.Logger.Info(strings.Replace(body, "\n", ",", 1))
			sendUrgentCardToPersonByUserIdErr := feishu.SendUrgentCardToPersonByUserId(cfg.ClientConfig().Manager, feishu.BuildSystemAlertCard("TACACS Client 进程崩溃", body, "red"), feishu.UrgentApp)
			if sendUrgentCardToPersonByUserIdErr != nil {
				log.Logger.Errorf("SendUrgentCardToPersonByUserId err:%v", sendUrgentCardToPersonByUserIdErr)
			}
		}
	}()

	// 见 server.go 中同名标记的说明: stdout → nohup .out → deploy.sh grep,
	// 让 deploy 在 1-2s 内确认起服成功,不死等 12s。
	fmt.Println("READY: client initialized, awaiting signals")

	//优雅退出
	catchStopSignalClient()

}

func catchStopSignalClient() {
	signal.Notify(Signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		s := <-Signals
		log.Logger.Infof("catch signals which is %v", s)
		switch s {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			http.Stop()
			tps.Stop()
			log.Logger.Info("system stop")
			return
		}
	}
}

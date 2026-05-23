package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"

	"tacacs/pkg/public/cfg"
	tacacsswm "tacacs/pkg/swm"
	"tacacs/pkg/swm/http"
)

var (
	Signals = make(chan os.Signal, 1)
)

func main() {
	item := "swm"

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

	initLoggerErr := log.InitAppLog(item, cfg.SwmConfig().LogFilePath)
	if initLoggerErr != nil {
		fmt.Printf("init logger failed:%v", initLoggerErr)
		return
	}
	tacacsswm.InitLog(cfg.SwmConfig().LogFilePath)

	//http 启动（前端 + /tacacs/* 反代），把嵌入的 static 注入
	//cert_file/key_file 双空就降级 HTTP, 双填走 HTTPS (见 pkg/swm/http/http.go)
	http.Start(cfg.SwmConfig().Http, tacacsswm.StaticFS)

	defer func() {
		if r := recover(); r != nil {
			body := fmt.Sprintf("**主机**：%v\n**tacacs_swm panic**：%v", env.HostName, r)
			log.Logger.Info(strings.Replace(body, "\n", ",", 1))
			sendUrgentCardToPersonByUserIdErr := feishu.SendUrgentCardToPersonByUserId(cfg.SwmConfig().Manager, feishu.BuildSystemAlertCard("TACACS Swm 进程崩溃", body, "red"), feishu.UrgentApp)
			if sendUrgentCardToPersonByUserIdErr != nil {
				log.Logger.Errorf("SendUrgentCardToPersonByUserId err:%v", sendUrgentCardToPersonByUserIdErr)
			}
		}
	}()

	// 见 server.go 中同名标记的说明: stdout → nohup .out → deploy.sh grep,
	// 让 deploy 在 1-2s 内确认起服成功,不死等 12s。
	fmt.Println("READY: swm initialized, awaiting signals")

	//优雅退出
	catchStopSignalSwm()

}

func catchStopSignalSwm() {
	signal.Notify(Signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		s := <-Signals
		log.Logger.Infof("catch signals which is %v", s)
		switch s {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			http.Stop()
			log.Logger.Info("system stop")
			return
		}
	}
}

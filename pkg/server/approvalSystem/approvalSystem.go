package approvalSystem

import (
	"sync"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"
	"tacacs/pkg/public/waitGroup"
	"time"
)

const (
	ApprovalAll                   = 5     //
	ApprovalPass                  = 4     //审批通过
	ApprovalUnderReview           = 3     //审批中
	ApprovalFailed                = 2     //审批不通过
	TimeoutShutdown               = 1     //超时关闭
	ManualShutdown                = 0     //手动关闭
	timeoutApprovalTaskCheckCycle = 60    //工单审批超时的检查周期，单位秒。一分钟
	approvalTimeout               = 86400 //工单审批超时时间，单位秒。一天
)

// 定于审批系统结构体
type ApprovalSystem struct {
	approvalMission map[int][]*db.TacacsApproval
	mutex           sync.RWMutex
	run             bool
}

func NewApprovalSystem() *ApprovalSystem {
	as := &ApprovalSystem{
		approvalMission: make(map[int][]*db.TacacsApproval),
	}
	go as.running()
	return as
}

func (as *ApprovalSystem) Start() {
	as.mutex.Lock()
	defer as.mutex.Unlock()
	as.run = true
}

func (as *ApprovalSystem) Stop() {
	as.mutex.Lock()
	defer as.mutex.Unlock()
	as.run = false

}

func checkCreateTimeTimeout() {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	missions, err := db.GetTacacsApproval(ApprovalUnderReview)
	if err != nil {
		return
	}

	for _, mission := range missions {
		now := time.Now()
		duration := now.Sub(mission.CreateTime)
		if duration.Seconds() > approvalTimeout {
			_ = db.UpdateTacacsApprovalStatus(mission.ID, TimeoutShutdown)
			if err := feishu.SendCardToPersonByTacacsUserName([]string{mission.User}, feishu.BuildApprovalLifecycleCard(mission, "timeout", 0)); err != nil {
				log.Logger.Errorf("send timeout card to %v err: %v", mission.User, err)
			}
			log.Logger.Infof("missionID(%v) approval timeout, status change to TimeoutShutdown", mission.ID)
		}
	}
}

func checkPermissionTimeout() {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	missions, err := db.GetTacacsApproval(ApprovalPass)
	if err != nil {
		return
	}
	for _, mission := range missions {
		now := time.Now()
		duration := now.Sub(mission.EndTime)
		if duration.Seconds() > 1 {
			_ = db.UpdateTacacsApprovalStatus(mission.ID, ManualShutdown)
			if err := feishu.SendCardToPersonByTacacsUserName([]string{mission.User}, feishu.BuildApprovalLifecycleCard(mission, "expired", 0)); err != nil {
				log.Logger.Errorf("send expired card to %v err: %v", mission.User, err)
			}
			log.Logger.Infof("missionID(%v) permission timeout, status change to ManualShutdown", mission.ID)
		}
	}
}

// 守护进程
func (as *ApprovalSystem) running() {
	go func() { // 每分钟检查一次，关闭超时工单
		for {
			if as.run {
				as.mutex.Lock()

				checkCreateTimeTimeout()

				as.mutex.Unlock()
			}
			time.Sleep(timeoutApprovalTaskCheckCycle * time.Second)
		}
	}()
	go func() {
		for { // 每分钟检查一次，关闭已到期的工单
			if as.run {
				as.mutex.Lock()

				checkPermissionTimeout()

				as.mutex.Unlock()
			}
			time.Sleep(timeoutApprovalTaskCheckCycle * time.Second)
		}
	}()
}

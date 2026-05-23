package env

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"tacacs/pkg/public/apolloConfig"
	"tacacs/pkg/public/log"

	"github.com/apolloconfig/agollo/v5/storage"
)

const (
	Prod = "prod"
	Test = "test"
)

var (
	InitFunc = make(map[string]func() error)
	HostName string
	Env      = Test
	Item     = ""
	DEBUG    = false
)

func init() {
	InitFunc["InitHostname"] = initHostname
	InitFunc["InitGOMAXPROCS"] = initGOMAXPROCS
	InitFunc["InitApolloConfig"] = apolloConfig.Init
}

func ParameterInitialization(item string) error {
	if initItemErr := initItem(item); initItemErr != nil {
		return fmt.Errorf("init item failed:%v", initItemErr)
	}
	initEnv()

	for name, f := range InitFunc {
		if err := f(); err != nil {
			if name == "InitApolloConfig" {
				log.Logger.Errorf("apollo init failed (will use local config): %v", err)
				continue
			}
			return fmt.Errorf("func(%v) init failed:%v", name, err)
		}
	}
	if apolloConfig.IsBeSet() {
		if debug, err := apolloConfig.GetConfig("debug"); err == nil {
			DEBUG = debug == "true"
		}
		apolloConfig.AddChangeListener(&debugListener{})
	}
	return nil
}

func initItem(item string) error {
	if item == "server" || item == "client" || item == "swm" {
		Item = item
		return nil
	}
	return fmt.Errorf("item must be server, client or swm")
}

func initGOMAXPROCS() error {
	num := getContainerCPUCount()
	if num <= 0 {
		num = runtime.NumCPU()
	}
	log.Logger.Infof("set GOMAXPROCS: %d", num)
	runtime.GOMAXPROCS(num)
	return nil
}

func getContainerCPUCount() int {
	if n := readCgroupV2CPU(); n > 0 {
		return n
	}
	if n := readCgroupV1CPU(); n > 0 {
		return n
	}
	return 0
}

func readCgroupV1CPU() int {
	quota := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quota <= 0 || period <= 0 {
		return 0
	}
	return quota / period
}

func readCgroupV2CPU() int {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0
	}
	parts := strings.Fields(strings.TrimSpace(string(data)))
	if len(parts) < 2 || parts[0] == "max" {
		return 0
	}
	quota, err := strconv.Atoi(parts[0])
	if err != nil || quota <= 0 {
		return 0
	}
	period, err := strconv.Atoi(parts[1])
	if err != nil || period <= 0 {
		return 0
	}
	return quota / period
}

func readIntFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}

func initHostname() error {
	hostname, getHostNameErr := os.Hostname()
	if getHostNameErr != nil {
		log.Logger.Errorf("InitHostname:%v", getHostNameErr)
		return getHostNameErr
	}
	HostName = hostname
	return nil
}

func initEnv() {
	result := os.Getenv("APP_ENV")
	if strings.TrimSpace(result) == "prod" {
		Env = Prod
	}
}

type debugListener struct{}

func (d *debugListener) OnChange(event *storage.ChangeEvent) {
	if v, ok := event.Changes["debug"]; ok {
		DEBUG = v.NewValue == "true"
		log.Logger.Infof("debug mode changed to: %v", DEBUG)
	}
}

func (d *debugListener) OnNewestChange(_ *storage.FullChangeEvent) {}

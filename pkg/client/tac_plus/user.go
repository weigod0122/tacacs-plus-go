package tac_plus

import (
	"net"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	userStatusNormal = "1"
	userStatusPause  = "2"
	userStatusDelete = "0"
	userUpdateCycle  = time.Minute * 5
)

var (
	tempTacacsTableUpdateTime  = ""
	tacacsUserInfo             atomic.Pointer[utils.TypedSyncMap[string, *UserInfo]]
	isUpdateUsersRunning       int32
	passwordEncryptCache       atomic.Pointer[utils.TypedSyncMap[string, bool]]
	passwordCheckSingleFlight  singleflight.Group
	cmdServerCheckCache        atomic.Pointer[utils.TypedSyncMap[string, cmdCheckResult]] // 添加缓存
	cmdServerCheckSingleFlight singleflight.Group
)

// cmdCheckResult 是 cmdServerCheckCache 的具体值类型，避免 map[string]interface{} 多次断言。
type cmdCheckResult struct {
	isPass bool
	reason string
}

func init() {
	tacacsUserInfo.Store(utils.NewTypedSyncMap[string, *UserInfo]())
	passwordEncryptCache.Store(utils.NewTypedSyncMap[string, bool]())
	cmdServerCheckCache.Store(utils.NewTypedSyncMap[string, cmdCheckResult]())
}

type roleCmdsServers struct {
	permitCmds    []string
	permitServers []string
}

type roleAcl struct {
	roleName      string
	permitCmds    []*regexp.Regexp
	permitServers struct {
		ips    []string
		ipNets []*net.IPNet
	}
}

type UserInfo struct {
	isOnDuty bool
	role     []roleAcl
	rolesKey string // role.roleName 排序后 join，缓存于此避免每次授权请求重复计算
	password string
	args     []string
}

func getAllRoleCmdsServers() (map[string]roleCmdsServers, error) {
	roles, err := db.GetTacacsRoleTemplates()
	if err != nil || len(roles) == 0 {
		return nil, err
	}
	newCache := make(map[string]roleCmdsServers)
	for _, role := range roles {
		permitCmds := make([]string, 0)
		permitServers := make([]string, 0)
		roleTemplate := db.GetTacacsRoleTemplateByTemplate(role.Template)
		if roleTemplate.ID == 0 {
			continue
		}
		for _, serverTemplateList := range strings.Split(roleTemplate.ServerTemplateList, ",") {
			if strings.TrimSpace(serverTemplateList) == "" { // 跳过空值
				continue
			}
			serverTemplateListDetails, _ := db.GetTacacsServerTemplatesByTemplate(serverTemplateList)
			for _, server := range serverTemplateListDetails {
				permitServers = append(permitServers, server.ServerTemplate)
			}
		}
		for _, commandTemplateList := range strings.Split(roleTemplate.CommandTemplateList, ",") {
			if strings.TrimSpace(commandTemplateList) == "" { // 跳过空值
				continue
			}
			commandTemplateListDetails, _ := db.GetTacacsCommandTemplatesByTemplate(commandTemplateList)
			for _, cmd := range commandTemplateListDetails {
				permitCmds = append(permitCmds, cmd.CommandTemplate)
			}
		}
		newCache[role.Template] = roleCmdsServers{
			permitCmds:    permitCmds,
			permitServers: permitServers,
		}
	}
	return newCache, nil
}

func clearTempTacacsTableUpdateTime() {
	tempTacacsTableUpdateTime = ""
}

func updateUsers() {
	if !atomic.CompareAndSwapInt32(&isUpdateUsersRunning, 0, 1) {
		log.Logger.Info("updateUsers is already running")
		return
	}

	defer func() {
		atomic.StoreInt32(&isUpdateUsersRunning, 0)
	}()

	tacacsTableUpdateTime, err := db.GetTablesUpdateTime()
	if err != nil {
		log.Logger.Errorf("get tacacsTableUpdateTime err, because: %v", err)
		return
	}

	if tempTacacsTableUpdateTime == tacacsTableUpdateTime {
		return
	}

	roleInfo, err := getAllRoleCmdsServers()
	if err != nil {
		log.Logger.Errorf("getAllRoleCmdsServers err, because: %v", err)
		return
	}

	tacacsUserInfos, err := db.GetTacacsUserInfos()
	if err != nil {
		log.Logger.Errorf("GetTacacsUserInfos err, because: %v", err)
		return
	}

	onDutyUsers := db.GetTacacsOnDutyUser()

	startTime := time.Now()
	log.DebugLog("1 tacacs info has updated, start handle database`s data")
	newTacacsUserInfo := utils.NewTypedSyncMap[string, *UserInfo]()
	for _, user := range tacacsUserInfos {
		if user.Status != userStatusNormal {
			continue
		}
		userRoles := strings.Split(user.Role, ",")
		roles := make([]roleAcl, 0)
		for _, role := range userRoles {
			rolePermitCmdsServers, ok := roleInfo[role]
			if !ok {
				continue
			}
			newPermitCmds := make([]*regexp.Regexp, 0)
			for _, cmd := range rolePermitCmdsServers.permitCmds {
				//tempCmd := strings.ReplaceAll(cmd, "*", ".*")
				//tempCmd = "^" + tempCmd
				// 这里就编译好，而不是在检查时编译
				if regex, err := regexp.Compile(cmd); err == nil {
					newPermitCmds = append(newPermitCmds, regex)
				}
			}
			ips := make([]string, 0)
			ipNets := make([]*net.IPNet, 0)
			for _, server := range rolePermitCmdsServers.permitServers {
				strType, ipNet := utils.GetNetworkType(server)
				if strType == 1 {
					ips = append(ips, server)
				} else if strType == 2 {
					ipNets = append(ipNets, ipNet)
				}
			}

			roles = append(roles, roleAcl{roleName: role, permitServers: struct {
				ips    []string
				ipNets []*net.IPNet
			}{
				ips:    ips,
				ipNets: ipNets,
			}, permitCmds: newPermitCmds})
		}

		newTacacsUserInfo.Set(user.User, &UserInfo{
			isOnDuty: utils.IsValueInList(user.User, onDutyUsers),
			role:     roles,
			rolesKey: generateRoleKey(roles),
			password: user.Password,
			args: []string{
				"priv-lvl=15",
			},
		})
	}
	log.DebugLog("2 handle database`s data done, time consuming: %v", time.Since(startTime))

	// The database may change while this snapshot is being assembled. Check
	// the metadata once more before publishing it; otherwise a concurrent
	// password/role update can be acknowledged with a stale snapshot and wait
	// for the five-minute safety refresh.
	latestTacacsTableUpdateTime, err := db.GetTablesUpdateTime()
	if err != nil {
		log.Logger.Errorf("recheck tacacsTableUpdateTime err, because: %v", err)
		return
	}
	if latestTacacsTableUpdateTime != tacacsTableUpdateTime {
		log.Logger.Infof("database changed while rebuilding user cache, retrying (before=%s after=%s)", tacacsTableUpdateTime, latestTacacsTableUpdateTime)
		return
	}

	startTime2 := time.Now()
	log.DebugLog("3 start update tacacsUserInfo")
	updateTacacsUserInfoAndCache(newTacacsUserInfo)
	// Only acknowledge the metadata version after the complete user/role
	// snapshot has been rebuilt.  If a read fails or a replica briefly returns
	// an inconsistent snapshot, the next poll must retry instead of suppressing
	// the change until the five-minute safety refresh.
	tempTacacsTableUpdateTime = latestTacacsTableUpdateTime
	log.DebugLog("4 update tacacsUserInfo done, time consuming: %v", time.Since(startTime2))

}

func updateTacacsUserInfoAndCache(newTacacsUserInfo *utils.TypedSyncMap[string, *UserInfo]) {
	tacacsUserInfo.Store(newTacacsUserInfo)
	passwordEncryptCache.Store(utils.NewTypedSyncMap[string, bool]())
	cmdServerCheckCache.Store(utils.NewTypedSyncMap[string, cmdCheckResult]())
}

func getTacacsUserInfo(user string) (*UserInfo, bool) {
	return tacacsUserInfo.Load().Get(user)
}

func checkUserPassword(password, hash string) bool {
	cacheKey := password + "\n" + hash

	// 第一步：快速路径 - 检查缓存
	if v, ok := passwordEncryptCache.Load().Get(cacheKey); ok {
		return v
	}

	// 第二步：在 singleflight 保护下执行检查
	result, _, _ := passwordCheckSingleFlight.Do(cacheKey, func() (interface{}, error) {
		// 在 singleflight 内再次检查缓存
		cache := passwordEncryptCache.Load()
		if v, ok := cache.Get(cacheKey); ok {
			return v, nil
		}

		// 执行实际验证
		isPass := utils.CheckPasswordHash(password, hash)

		// 写入当前缓存（TypedSyncMap 自身线程安全）
		cache.Set(cacheKey, isPass)
		return isPass, nil
	})

	v, ok := result.(bool)
	if !ok {
		return false
	}
	return v
}

func checkCmdAndServerInRole(cmd, server, rolesKey string, roles []roleAcl) (isPass bool, reason string) {
	if roles == nil || len(roles) == 0 {
		return false, "role is nil"
	}

	cacheKey := cmd + "\n" + server + "\n" + rolesKey

	// 第一步：快速路径 - 检查缓存
	if r, ok := cmdServerCheckCache.Load().Get(cacheKey); ok {
		return r.isPass, r.reason
	}

	// 第二步：在 singleflight 保护下执行
	result, _, _ := cmdServerCheckSingleFlight.Do(cacheKey, func() (interface{}, error) {
		// 二次检查缓存
		cache := cmdServerCheckCache.Load()
		if r, ok := cache.Get(cacheKey); ok {
			return r, nil
		}

		var r cmdCheckResult
		for _, role := range roles {
			if roleName := checkRoleMatch(cmd, server, role); roleName != "" {
				r.isPass = true
				r.reason = roleName
				break
			}
		}
		if !r.isPass {
			r.reason = "cmd and server are not match"
		}

		// 写入当前缓存
		cache.Set(cacheKey, r)
		return r, nil
	})

	r, ok := result.(cmdCheckResult)
	if !ok {
		return false, "singleflight 返回类型异常"
	}
	return r.isPass, r.reason
}

func checkRoleMatch(cmd, server string, role roleAcl) string {
	// 检查命令
	cmdIsPass := cmd == ""
	if !cmdIsPass {
		for _, permitCmd := range role.permitCmds {
			if permitCmd.MatchString(cmd) {
				cmdIsPass = true
				break
			}
		}
	}
	if !cmdIsPass {
		return ""
	}

	// 检查服务器
	serverIsPass := server == ""
	if !serverIsPass {
		for _, permitServer := range role.permitServers.ips {
			if server == permitServer {
				serverIsPass = true
				break
			}
		}
	}

	if !serverIsPass && len(role.permitServers.ipNets) > 0 {
		ip := net.ParseIP(server)
		if ip != nil {
			for _, permitServer := range role.permitServers.ipNets {
				if permitServer.Contains(ip) {
					serverIsPass = true
					break
				}
			}
		}
	}

	if serverIsPass {
		return role.roleName
	}
	return ""
}

func generateRoleKey(roles []roleAcl) string {
	if len(roles) == 0 {
		return ""
	}
	names := make([]string, len(roles))
	for i, role := range roles {
		names[i] = role.roleName
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

package tac_plus

import (
	"context"
	"fmt"
	"net"
	"strings"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/tacplus"
	"time"

	"github.com/bytedance/sonic"
)

// 定义处理器，实现tacplus包中的RequestHandler接口
type myHandler struct{}

func (h *myHandler) getSwitchIp(addr net.Addr) string {
	if addr == nil {
		return "addr is nil"
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		return a.IP.String()
	case *net.IPAddr:
		return a.IP.String()
	}
	return "addr is nil"
}

// 处理认证请求
func (h *myHandler) HandleAuthenStart(ctx context.Context, a *tacplus.AuthenStart, s *tacplus.ServerSession) *tacplus.AuthenReply {
	info := &tacplus.AuthenInfo{
		StartTime:       time.Now(),
		User:            a.User,
		ServerAddr:      a.RemAddr,
		SwitchAddr:      h.getSwitchIp(s.RemoteAddr()),
		IsSingleConnect: s.IsSingleConnect(),
	}

	var user string
	if a.User != "" {
		user = a.User
		info.User = user
		if env.DEBUG {
			info.AuthenStatus = "getUserDone"
			info.Details = "Authen pkt has User"
			infoJSON, _ := sonic.MarshalString(info)
			log.DebugLog("%s", infoJSON)
		}
	}
	getUserNum := 0
	for user == "" {
		getUserNum++
		if env.DEBUG {
			info.AuthenStatus = "getUserIng"
			info.Details = "Get User"
			infoJSON, _ := sonic.MarshalString(info)
			log.DebugLog("%s", infoJSON)
		}
		c, err := s.GetUser(context.Background(), "User:")
		if err != nil || c.Abort || getUserNum > 5 {
			info.AuthenStatus = "getUserError"
			info.Details = "Get User Num Exceed 5"
			LogAuthen(info)
			return &tacplus.AuthenReply{Status: tacplus.AuthenStatusFail}
		}
		user = c.Message
		if user != "" {
			info.User = user
			if env.DEBUG {
				info.AuthenStatus = "getUserDone"
				info.Details = "Get User Done"
				infoJSON, _ := sonic.MarshalString(info)
				log.DebugLog("%s", infoJSON)
			}
		}

	}

	var pass string
	getPassNum := 0
	for pass == "" {
		getPassNum++
		if env.DEBUG {
			info.AuthenStatus = "getPassIng"
			info.Details = "Get User Password"
			infoJSON, _ := sonic.MarshalString(info)
			log.DebugLog("%s", infoJSON)
		}
		c, err := s.GetPass(context.Background(), "Password:")
		if err != nil || c.Abort || getPassNum > 5 {
			info.AuthenStatus = "getPassError"
			info.Details = "Get User Password Num Exceed 5"
			LogAuthen(info)
			return &tacplus.AuthenReply{Status: tacplus.AuthenStatusFail}
		}
		pass = c.Message
		if env.DEBUG && pass != "" {
			info.AuthenStatus = "getPassDone"
			info.Details = "Get User Password Done"
			infoJSON, _ := sonic.MarshalString(info)
			log.DebugLog("%s", infoJSON)
		}
	}

	isPass, detail := func() (status bool, detail string) {

		//当前用户存在，并且密码认证正确，并且角色不是null才通过
		userInfo, exists := getTacacsUserInfo(user)
		if !exists {
			return false, "User does not exist"
		}
		//密码检查
		if !checkUserPassword(pass, userInfo.password) {
			return false, fmt.Sprintf("password is wrong, inputPass:'%v'", pass)
		}
		//值班人员不用检查角色
		if userInfo.isOnDuty {
			return true, "isOnDuty"
		}
		//非值班人员需要检查是否存在角色，没有角色即使密码对也不通过
		if len(userInfo.role) == 0 {
			return false, "User role is null"
		}
		//用户存在、密码正确、角色存在
		return true, "User role is pass"
	}()

	info.Details = detail

	var AuthenReply *tacplus.AuthenReply
	if isPass {
		info.AuthenStatus = "pass"
		AuthenReply = &tacplus.AuthenReply{Status: tacplus.AuthenStatusPass}
	} else {
		info.AuthenStatus = "noPass"
		AuthenReply = &tacplus.AuthenReply{Status: tacplus.AuthenStatusFail}
	}

	LogAuthen(info)
	return AuthenReply
}

// 处理授权请求
func (h *myHandler) HandleAuthorRequest(ctx context.Context, a *tacplus.AuthorRequest, s *tacplus.ServerSession) *tacplus.AuthorResponse {
	info := &tacplus.AuthorInfo{
		StartTime:       time.Now(),
		User:            a.User,
		ServerAddr:      a.RemAddr,
		SwitchAddr:      h.getSwitchIp(s.RemoteAddr()),
		IsSingleConnect: s.IsSingleConnect(),
	}

	isPass, detail, userArgs := func() (status bool, detail string, userArgs []string) {
		info.Cmd = func() string {
			var cmd string
			addArgs := make([]string, 0, len(a.Arg))
			for _, arg := range a.Arg {
				if strings.EqualFold(arg, "cmd*") { //命令初始化会发送这个，直接通过就行
					continue
				}
				eq := strings.IndexByte(arg, '=')
				if eq <= 0 {
					continue
				}
				key, val := arg[:eq], arg[eq+1:]
				switch {
				case strings.EqualFold(key, "cmd"):
					cmd = val
				case strings.EqualFold(key, "cmd-arg"):
					if val != "<cr>" {
						addArgs = append(addArgs, val)
					}
				}
			}
			if cmd == "" {
				return ""
			}
			if len(addArgs) == 0 {
				return cmd
			}
			// 预算总长度，一次分配 buf，避免 cmd += " " + ... 的二次方拼接
			n := len(cmd)
			for _, v := range addArgs {
				n += 1 + len(v)
			}
			b := make([]byte, 0, n)
			b = append(b, cmd...)
			for _, v := range addArgs {
				b = append(b, ' ')
				b = append(b, v...)
			}
			return string(b)
		}()
		userInfo, exists := getTacacsUserInfo(a.User)
		if !exists {
			return false, "User does not exist", nil
		}
		if userInfo.isOnDuty {
			return true, "isOnDuty", userInfo.args
		}
		status, detail = checkCmdAndServerInRole(info.Cmd, info.ServerAddr, userInfo.rolesKey, userInfo.role)
		return status, detail, userInfo.args
	}()

	info.Details = detail

	var AuthorResponse *tacplus.AuthorResponse
	if isPass {
		info.AuthorStatus = "pass"
		AuthorResponse = &tacplus.AuthorResponse{Status: tacplus.AuthorStatusPassAdd, Arg: userArgs}
	} else {
		info.AuthorStatus = "noPass"
		AuthorResponse = &tacplus.AuthorResponse{Status: tacplus.AuthorStatusFail}
	}

	LogAuthor(info)
	return AuthorResponse

}

// 处理记账请求
func (h *myHandler) HandleAcctRequest(ctx context.Context, a *tacplus.AcctRequest, s *tacplus.ServerSession) *tacplus.AcctReply {
	info := &tacplus.AccountInfo{
		StartTime:       time.Now(),
		IsSingleConnect: s.IsSingleConnect(),
	}

	var cmd string
	for _, arg := range a.Arg {
		if len(arg) >= 4 && strings.EqualFold(arg[:4], "cmd=") {
			cmd = arg[4:]
			break
		}
	}

	cmd = strings.TrimSuffix(cmd, " <cr>")

	info.SwitchAddr = h.getSwitchIp(s.RemoteAddr())
	info.ServerAddr = a.RemAddr
	info.User = a.User
	info.Port = a.Port
	info.Flags = int16(a.Flags)
	info.AuthenMethod = int16(a.AuthenMethod)
	info.PrivLvl = int16(a.PrivLvl)
	info.AuthenType = int16(a.AuthenType)
	info.AuthenService = int16(a.AuthenService)
	info.Arg = a.Arg
	info.Cmd = cmd
	LogAccount(info)
	return &tacplus.AcctReply{
		Status: tacplus.AcctStatusSuccess,
	}
}

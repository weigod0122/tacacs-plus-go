package db

import (
	"database/sql"
	"errors"
	"fmt"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"

	_ "github.com/go-sql-driver/mysql"
)

// DbWrite 仅在 server 进程被初始化;client 进程始终为 nil,
// 任何写函数被 client 链路误调时会以 nil pointer panic 立刻暴露,
// 配合 cfg_client.yaml 不再持有 write 账号密码,从根上掐掉横向写库。
var DbWrite *sql.DB
var DbRead *sql.DB

func Init() error {
	switch env.Item {
	case "client":
		// Client 只用只读账号:授权热路径完全靠本地缓存 + read 库,
		// 不加载 DbWrite,也不要求 cfg_client.yaml 里有 write 块。
		return initReadOnly()
	case "server":
		return initReadWrite()
	default:
		return errors.New("item is not one of server or client")
	}
}

func initReadOnly() error {
	info := cfg.ClientConfig().Database[env.Env]["read"]
	if info.Address == "" {
		return fmt.Errorf("client database.%s.read config is missing", env.Env)
	}
	dbR, err := openSql(buildDSN(info))
	if err != nil {
		return err
	}
	DbRead = dbR
	log.Logger.Infof("init mysql (read-only) success.")
	return nil
}

func initReadWrite() error {
	write := cfg.ServerConfig().Database[env.Env]["write"]
	read := cfg.ServerConfig().Database[env.Env]["read"]
	dbW, err := openSql(buildDSN(write))
	if err != nil {
		return err
	}
	dbR, err := openSql(buildDSN(read))
	if err != nil {
		return err
	}
	DbWrite = dbW
	DbRead = dbR
	log.Logger.Infof("init mysql success.")
	return nil
}

func buildDSN(info cfg.DatabaseInfo) string {
	return fmt.Sprintf("%v:%v@tcp(%v)/%v?loc=Local&parseTime=true",
		info.Username, info.Password, info.Address, info.Table)
}

func openSql(url string) (*sql.DB, error) {
	var err error
	db, err := sql.Open("mysql", url)
	if err != nil {
		return nil, err
	}

	db.SetMaxIdleConns(60)
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, err
}

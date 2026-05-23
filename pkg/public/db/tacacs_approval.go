package db

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"tacacs/pkg/public/log"
	"time"
)

type TacacsApproval struct {
	ID                  int64
	CreateTime          time.Time
	User                string
	ApprovalPermissions string
	StartTime           time.Time
	EndTime             time.Time
	Status              int
	Approver            string    // 仅在 status 为 2/4 时有意义；旧数据可能为空
	ApproveTime         time.Time // 同上；零值表示 NULL
}

const approvalSelectColumns = "id, create_time, user, approval_permissions, start_time, end_time, status, approver, approve_time"

// scanApproval 把一行数据扫到 TacacsApproval，处理 approver/approve_time 的 NULL 情况。
func scanApproval(row interface {
	Scan(dest ...any) error
}) (*TacacsApproval, error) {
	a := &TacacsApproval{}
	var approver sql.NullString
	var approveTime sql.NullTime
	err := row.Scan(&a.ID, &a.CreateTime, &a.User, &a.ApprovalPermissions,
		&a.StartTime, &a.EndTime, &a.Status, &approver, &approveTime)
	if err != nil {
		return nil, err
	}
	a.Approver = approver.String
	if approveTime.Valid {
		a.ApproveTime = approveTime.Time
	}
	return a, nil
}

func GetTacacsApproval(status int) ([]*TacacsApproval, error) {
	var selectSql string
	switch status {
	case 0, 1, 2, 3, 4:
		selectSql = fmt.Sprintf("SELECT %s FROM tacacs_approval WHERE status = %v", approvalSelectColumns, status)
	case 5:
		selectSql = fmt.Sprintf("SELECT %s FROM tacacs_approval", approvalSelectColumns)
	default:
		return []*TacacsApproval{}, fmt.Errorf("status(%v) input is error", status)

	}
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsApproval from database is failed, because %v", err)
		return nil, err
	}
	defer rows.Close()

	var tacacsApprovals []*TacacsApproval
	for rows.Next() {
		i, err := scanApproval(rows)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return nil, err
		}
		tacacsApprovals = append(tacacsApprovals, i)
	}
	return tacacsApprovals, nil
}

// GetTacacsApprovalByID 按 id 取一条工单。找不到返回 (nil, nil) 而不是 error，
// 让 handler 区分 "查询失败" vs "工单不存在"。
func GetTacacsApprovalByID(id int64) (*TacacsApproval, error) {
	row := DbRead.QueryRow(fmt.Sprintf("SELECT %s FROM tacacs_approval WHERE id = ?", approvalSelectColumns), id)
	a, err := scanApproval(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Logger.Errorf("query tacacs_approval by id %v failed: %v", id, err)
		return nil, err
	}
	return a, nil
}

func CreateTacacsApproval(tacacsApproval TacacsApproval) (int64, error) {
	insertSql := "INSERT INTO tacacs_approval(create_time, user, approval_permissions, start_time, end_time, status) VALUES(?,?,?,?,?,?)"
	res, err := DbWrite.Exec(insertSql, tacacsApproval.CreateTime, tacacsApproval.User, tacacsApproval.ApprovalPermissions, tacacsApproval.StartTime, tacacsApproval.EndTime, tacacsApproval.Status)
	if err != nil {
		log.Logger.Errorf("insert tacacs_approval to database is failed, because %v", err)
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Logger.Errorf("get last insert id of tacacs_approval failed, because %v", err)
		return 0, err
	}
	return id, nil
}

func UpdateTacacsApprovalStatus(id int64, status int) error {
	updateSql := "UPDATE tacacs_approval SET status = ? WHERE id = ?"
	_, err := DbWrite.Exec(updateSql, status, id)
	if err != nil {
		log.Logger.Errorf("update tacacs_approval status in database is failed, because %v", err)
		return err
	}
	return nil
}

// ApproveWithLock 用乐观锁更新工单状态：仅当当前 status=3（审批中）时才会改写，
// 同时记录 approver 和 approve_time。RowsAffected=0 表示工单已被处理（被另一
// admin 抢先，或者超时关闭，或者根本不存在）——调用方应据此给前端友好反馈。
func ApproveWithLock(id int64, newStatus int, approver string) (int64, error) {
	res, err := DbWrite.Exec(
		"UPDATE tacacs_approval SET status = ?, approver = ?, approve_time = ? WHERE id = ? AND status = 3",
		newStatus, approver, time.Now(), id,
	)
	if err != nil {
		log.Logger.Errorf("approve-with-lock id=%v status=%v failed: %v", id, newStatus, err)
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func GetTacacsApprovalData2Md5() (string, error) {
	selectSql := "SELECT CONCAT(id, create_time, user, approval_permissions, start_time, end_time, status) AS data_string FROM tacacs_approval"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsApproval from database is failed, because %v", err)
		return "", err
	}
	defer rows.Close()

	var allData strings.Builder
	for rows.Next() {
		var dataString string
		if err := rows.Scan(&dataString); err != nil {
			log.Logger.Errorf("scan row data failed, because %v", err)
			return "", err
		}
		allData.WriteString(dataString)
	}

	if err = rows.Err(); err != nil {
		log.Logger.Errorf("error occurred while iterating rows, because %v", err)
		return "", err
	}

	md5Hash := md5.Sum([]byte(allData.String()))
	return hex.EncodeToString(md5Hash[:]), nil
}

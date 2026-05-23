// Package utils /* 定义公共函数 */
package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"tacacs/pkg/public/log"

	"golang.org/x/crypto/bcrypt"
)

// IsValueInList 判断a值是否在切片b中
func IsValueInList[T comparable](value T, list []T) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// bcryptCost 是密码哈希的工作因子。
// 2026 年硬件下 cost=12 ≈ 250ms/次,显著抬高离线爆破成本;升高需关注登录 QPS。
// 修改后不影响存量密码 —— bcrypt 把 cost 编码在哈希前缀里,验证时自动适配。
const bcryptCost = 12

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// UniqueStringSlice 对字符串切片去重
func UniqueStringSlice(slice []string) []string {
	uniqueMap := make(map[string]bool)
	var uniqueSlice []string
	for _, item := range slice {
		if _, ok := uniqueMap[item]; !ok {
			uniqueMap[item] = true
			uniqueSlice = append(uniqueSlice, item)
		}
	}
	return uniqueSlice
}

// IsValidIP 检查字符串是否为有效的IP地址
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// ReadFile 读文件
func ReadFile(filename string) (s string) {
	cmd := exec.Command("dos2unix", filename)
	_ = cmd.Run()
	f, err := os.ReadFile(filename)
	if err != nil {
		log.Logger.Errorf("%v", err)
		return ""
	}
	s = string(f)
	return
}

// PathExists 判断一个文件或文件夹是否存在,输入文件路径，根据返回的bool值来判断文件或文件夹是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ListFilesInDirectory 列出指定目录下的所有文件（包括子目录中的文件）
// 输入：目录路径
// 输出：包含完整文件路径的切片
func ListFilesInDirectory(dirPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 如果遇到错误，记录但不中断遍历
			return nil
		}

		// 只收集文件，忽略目录
		if !info.IsDir() {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("遍历目录 %s 失败: %v", dirPath, err)
	}

	return files, nil
}

// ListFilesInDirectoryOnly 列出指定目录下的文件（不包括子目录中的文件）
// 输入：目录路径
// 输出：包含完整文件路径的切片
func ListFilesInDirectoryOnly(dirPath string) ([]string, error) {
	var files []string

	// 读取目录内容
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录 %s 失败: %v", dirPath, err)
	}

	// 遍历目录条目
	for _, entry := range entries {
		// 只收集文件，忽略目录
		if !entry.IsDir() {
			fullPath := filepath.Join(dirPath, entry.Name())
			files = append(files, fullPath)
		}
	}

	return files, nil
}

// 判断字符串类型：1=IP地址, 2=网段, 0=无效格式
func GetNetworkType(str string) (int, *net.IPNet) {
	str = strings.TrimSpace(str)

	// 检查是否包含CIDR标记（/）
	if strings.Contains(str, "/") {
		// 尝试解析为网段
		_, ipNet, err := net.ParseCIDR(str)
		if err == nil {
			return 2, ipNet // 网段
		}
	} else {
		// 尝试解析为IP地址
		ip := net.ParseIP(str)
		if ip != nil {
			return 1, nil // IP地址
		}
	}

	return 0, nil // 无效格式
}

// 求两个集合的交集,时间复杂度 O(n+m)
func GetIntersection(listA, listB []string) (intersection map[string]bool) {
	intersection = make(map[string]bool)
	// 将 listB 转为 map，时间复杂度 O(m)
	bMap := make(map[string]bool)
	for _, v := range listB {
		bMap[v] = true
	}

	// 遍历 listA，时间复杂度 O(n)
	for _, v := range listA {
		if bMap[v] {
			intersection[v] = true // 交集
		}
	}
	return
}

// GetFunctionName 获取函数的包名和函数名
func GetFunctionName(i interface{}) (packageName, functionName string) {
	// 获取函数的完整名称
	fullName := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()

	// 分割包名和函数名
	if lastDotIndex := strings.LastIndex(fullName, "."); lastDotIndex != -1 {
		packageName = fullName[:lastDotIndex]
		functionName = fullName[lastDotIndex+1:]
	} else {
		functionName = fullName
	}

	return
}

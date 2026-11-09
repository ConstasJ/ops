package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// CheckDirWritable 检查指定目录是否具备写权限（通过创建并删除极简临时文件的方式精确验证）
func CheckDirWritable(dir string) error {
	if dir == "" {
		return nil
	}

	// 若目录不存在，递归寻找第一个已存在的祖先目录进行写权限探测
	target := dir
	for {
		fi, err := os.Stat(target)
		if err == nil {
			if !fi.IsDir() {
				return fmt.Errorf("路径 %q 不是目录", target)
			}
			break
		}
		parent := filepath.Dir(target)
		if parent == target || parent == "." {
			break
		}
		target = parent
	}

	// 在目标目录下尝试创建临时文件验证实际写入权限
	tmpFile, err := os.CreateTemp(target, ".ops-perm-check-*")
	if err != nil {
		return fmt.Errorf("权限不足: 无法向目录 %q 写入文件 (底层错误: %w)。\n请确认当前账号是否属于对应管理组，或使用 sudo 运行", dir, err)
	}
	tmpName := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpName)

	return nil
}

// CheckFileWritable 检查指定目标文件（若不存在则检查其父目录）是否可写
func CheckFileWritable(filePath string) error {
	if filePath == "" {
		return nil
	}
	return CheckDirWritable(filepath.Dir(filePath))
}

package api

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// handleBackup 导出一个 zip：数据库一致快照 + 全部上传的 PDF + 恢复说明。
// 解压覆盖到程序旁的 data/ 目录即可完整恢复，不依赖任何外部工具。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	// 先在临时目录生成快照；失败时还能返回 JSON 错误（此时尚未写响应头）
	tmpDir, err := os.MkdirTemp("", "pm-backup-")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建临时目录")
		return
	}
	defer os.RemoveAll(tmpDir)
	snapPath := filepath.Join(tmpDir, "paper-manager.db")
	if err := s.store.Snapshot(snapPath); err != nil {
		writeError(w, http.StatusInternalServerError, "数据库快照失败: "+err.Error())
		return
	}

	name := "paper-manager-backup-" + time.Now().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	zw := zip.NewWriter(w)
	defer zw.Close()

	if err := zipAddFile(zw, snapPath, "data/paper-manager.db"); err != nil {
		return // 响应头已发出，只能中断
	}
	if entries, err := os.ReadDir(s.uploadDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if err := zipAddFile(zw, filepath.Join(s.uploadDir, e.Name()), "data/uploads/"+e.Name()); err != nil {
				return
			}
		}
	}
	_ = zipAddText(zw, "恢复说明.txt", `恢复方法
1. 退出 paper-manager（关闭窗口，或 Ctrl+C）
2. 把本压缩包里的 data/ 目录解压到程序所在目录，覆盖同名文件
   （不确定位置时看程序启动日志里的「数据目录」）
3. 重新启动程序即可

说明
- data/paper-manager.db 是导出时刻的一致性快照（含未落盘事务）
- data/uploads/ 是全部 PDF 原件，文件名与数据库记录一一对应
- 备份包不含 AI API Key 之外的任何外部依赖；恢复不需要联网
`)
}

func zipAddFile(zw *zip.Writer, src, name string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Deflate
	dst, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}

func zipAddText(zw *zip.Writer, name, text string) error {
	dst, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = dst.Write([]byte(text))
	return err
}

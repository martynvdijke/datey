package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/datey/datey/internal/db"
)

type backupFileInfo struct {
	Name     string
	Size     int64
	Modified time.Time
}

func listBackupFiles(dir string) []backupFileInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []backupFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".db" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, backupFileInfo{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
	}
	return out
}

func isValidBackupFilename(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if filepath.IsAbs(name) {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	return true
}

func (h *Handler) settingsBackupDownload(w http.ResponseWriter, r *http.Request) {
	dbPath := h.cfg.DataDir + "/datey.db"
	f, err := os.Open(dbPath)
	if err != nil {
		slog.Error("backup download open", "error", err)
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="datey.db"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	if _, err := io.Copy(w, f); err != nil {
		slog.Error("backup download copy", "error", err)
	}
}

func (h *Handler) settingsBackupRestore(w http.ResponseWriter, r *http.Request) {
	// Confirmation check.
	confirm := r.FormValue("confirm")
	if confirm != "RESTORE" {
		h.renderBackupWithError(w, r, "Type RESTORE to confirm.")
		return
	}

	var candidatePath string
	var originalName string
	var tmpUpload string

	// Try multipart upload first.
	file, header, err := r.FormFile("file")
	hasUpload := err == nil && header != nil && header.Filename != ""
	if hasUpload {
		originalName = header.Filename
		if !isValidBackupFilename(filepath.Base(originalName)) {
			// Use base for validation of traversal; but still need to sanitize
			// originalName traversal check: if header.Filename contains path sep -> reject
			if !isValidBackupFilename(originalName) {
				h.renderBackupWithError(w, r, "Invalid filename.")
				return
			}
		}
		tmp, err := os.CreateTemp("", "datey-restore-upload-*.db")
		if err != nil {
			h.renderBackupWithError(w, r, "Failed to handle upload.")
			return
		}
		tmpUpload = tmp.Name()
		if _, err := io.Copy(tmp, file); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpUpload)
			h.renderBackupWithError(w, r, "Failed to handle upload.")
			return
		}
		_ = tmp.Close()
		_ = file.Close()
		candidatePath = tmpUpload
		defer func() { _ = os.Remove(tmpUpload) }()
	} else {
		// Close file if opened but no filename (empty upload)
		if file != nil {
			_ = file.Close()
		}
		// Backup dir selection
		name := r.FormValue("file")
		if name == "" {
			h.renderBackupWithError(w, r, "Select a backup file or upload a database.")
			return
		}
		if !isValidBackupFilename(name) {
			h.renderBackupWithError(w, r, "Invalid filename.")
			return
		}
		candidatePath = filepath.Join(h.cfg.BackupDir, name)
		originalName = name
		if _, err := os.Stat(candidatePath); err != nil {
			h.renderBackupWithError(w, r, "Backup file not found.")
			return
		}
	}

	// Validate candidate (exported function does header + integrity check via temp copy)
	if err := db.ValidateCandidate(candidatePath); err != nil {
		slog.Warn("restore validation failed", "file", originalName, "error", err)
		h.renderBackupWithError(w, r, fmt.Sprintf("Validation failed: %v", err))
		return
	}

	// Stage file into DataDir as restore-pending.db
	staged := db.RestoreStagedPath(h.cfg.DataDir)
	// Ensure datadir exists
	if err := os.MkdirAll(h.cfg.DataDir, 0755); err != nil {
		h.renderBackupWithError(w, r, "Failed to stage restore.")
		return
	}
	if err := copyFileForRestore(candidatePath, staged); err != nil {
		slog.Error("restore stage copy", "error", err)
		h.renderBackupWithError(w, r, "Failed to stage restore.")
		return
	}
	if err := db.WriteRestorePending(h.cfg.DataDir, originalName); err != nil {
		_ = os.Remove(staged)
		h.renderBackupWithError(w, r, "Failed to stage restore.")
		return
	}
	h.auditRecord(r, "backup.restore_staged", originalName)
	http.Redirect(w, r, "/settings/backup?success=Restore+staged.+Restart+to+apply.", http.StatusSeeOther)
}

func copyFileForRestore(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func (h *Handler) renderBackupWithError(w http.ResponseWriter, r *http.Request, msg string) {
	files := listBackupFiles(h.cfg.BackupDir)
	pending, _ := db.ReadRestorePending(h.cfg.DataDir)
	var pendingInfo *db.RestorePending
	if pending != nil {
		pendingInfo = pending
	}
	h.render(w, r, "settings.html", map[string]any{
		"Title":               "Datey - Settings",
		"SettingsTab":         "backup",
		"BackupDir":           h.cfg.BackupDir,
		"BackupRetentionDays": h.cfg.BackupRetentionDays,
		"BackupFiles":         files,
		"RestorePending":      pendingInfo,
		"RestoreError":        msg,
	})
}

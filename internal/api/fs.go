package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// fsEntry is a filesystem entry returned by the directory browser.
type fsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// fsRoot is a top-level starting point (drive or "/", plus the home directory).
type fsRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleFsList lets an authenticated admin browse the server filesystem so the
// UI can offer a directory picker (source directories, local channel paths).
// It lists names/sizes only — it never returns file contents.
func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request) {
	roots := listRoots()
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		writeJSON(w, http.StatusOK, map[string]any{"path": "", "parent": "", "roots": roots, "entries": []fsEntry{}})
		return
	}

	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "fs", "无法访问该路径: "+err.Error())
		return
	}
	if !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "fs", "不是目录")
		return
	}

	dirents, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "fs", "读取目录失败: "+err.Error())
		return
	}
	entries := make([]fsEntry, 0, len(dirents))
	for _, de := range dirents {
		e := fsEntry{Name: de.Name(), Path: filepath.Join(p, de.Name()), IsDir: de.IsDir()}
		if !de.IsDir() {
			if fi, err := de.Info(); err == nil {
				e.Size = fi.Size()
			}
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // directories first
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(p)
	if parent == p {
		parent = "" // already at a filesystem root
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "roots": roots, "entries": entries})
}

// listRoots returns filesystem starting points: drive letters on Windows or "/"
// elsewhere, plus the current user's home directory.
func listRoots() []fsRoot {
	var roots []fsRoot
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			d := string(c) + ":\\"
			if _, err := os.Stat(d); err == nil {
				roots = append(roots, fsRoot{Name: string(c) + ":", Path: d})
			}
		}
	} else {
		roots = append(roots, fsRoot{Name: "/", Path: "/"})
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, fsRoot{Name: "主目录", Path: home})
	}
	return roots
}

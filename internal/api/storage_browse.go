package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"archivesync/internal/storage"
)

// objEntry is a folder-style entry within a channel's object store.
type objEntry struct {
	Name         string     `json:"name"`
	Key          string     `json:"key"` // full object key (files) or prefix ending in "/" (folders)
	IsDir        bool       `json:"is_dir"`
	Size         int64      `json:"size"`
	LastModified *time.Time `json:"last_modified,omitempty"`
}

// listChannelObjects returns a folder-style view of the objects stored in a
// channel under an optional key prefix, synthesizing virtual folders from the
// next path segment.
func (s *Server) listChannelObjects(w http.ResponseWriter, r *http.Request) {
	ch, err := s.store.GetChannel(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	backend, err := storage.New(*ch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "channel", err.Error())
		return
	}
	prefix := r.URL.Query().Get("prefix")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	objs, err := backend.List(ctx, prefix)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "channel", "列举对象失败: "+err.Error())
		return
	}

	entries := folderView(objs, prefix)
	writeJSON(w, http.StatusOK, map[string]any{
		"prefix":  prefix,
		"parent":  parentPrefix(prefix),
		"entries": entries,
	})
}

// folderView collapses a flat object list into the immediate children of prefix:
// virtual folders (from the next "/" segment) and files.
func folderView(objs []storage.Object, prefix string) []objEntry {
	base := prefix
	if base != "" && !strings.HasSuffix(base, "/") {
		base += "/"
	}
	dirs := map[string]bool{}
	var files []objEntry
	for _, o := range objs {
		if !strings.HasPrefix(o.Key, base) {
			continue
		}
		rest := o.Key[len(base):]
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dirs[rest[:i]] = true
		} else {
			lm := o.LastModified
			e := objEntry{Name: rest, Key: o.Key, Size: o.Size}
			if !lm.IsZero() {
				e.LastModified = &lm
			}
			files = append(files, e)
		}
	}

	folders := make([]objEntry, 0, len(dirs))
	for d := range dirs {
		folders = append(folders, objEntry{Name: d, Key: base + d + "/", IsDir: true})
	}
	// Newest-first ordering: date/time folders and timestamped files sort well
	// in reverse lexicographic order.
	sort.Slice(folders, func(i, j int) bool { return folders[i].Name > folders[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return append(folders, files...)
}

// parentPrefix returns the parent folder prefix of a key prefix ("" at root).
func parentPrefix(prefix string) string {
	p := strings.TrimSuffix(prefix, "/")
	if p == "" {
		return ""
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i+1]
	}
	return ""
}

// downloadChannelObject streams a single object from a channel to the client as
// a file download.
func (s *Server) downloadChannelObject(w http.ResponseWriter, r *http.Request) {
	ch, err := s.store.GetChannel(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" || strings.HasSuffix(key, "/") {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少有效的对象 key")
		return
	}
	backend, err := storage.New(*ch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "channel", err.Error())
		return
	}

	rc, err := backend.Get(r.Context(), key)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "channel", "读取对象失败: "+err.Error())
		return
	}
	defer rc.Close()

	name := path.Base(key)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	_, _ = io.Copy(w, rc)
}

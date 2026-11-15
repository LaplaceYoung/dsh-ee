package api

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

func publicDshRelease(r *store.DshRelease, base string) map[string]any {
	return map[string]any{
		"id": r.ID, "target": r.Target, "kind": r.Kind, "version": r.Version, "released_at": r.ReleasedAt,
		"size": r.Size, "sha512": r.SHA512, "recommended": r.Recommended, "verified": r.Kind != "dmg",
		"download_url": base + "/dl/" + r.Target + "/" + r.File,
	}
}

func (s *Server) handleDshReleases(w http.ResponseWriter, r *http.Request) {
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var out []map[string]any
	if set.Distribution.Enabled {
		targets := make([]string, 0, len(s.releaseFeeds))
		for target := range s.releaseFeeds {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			items, err := s.store.ListDshReleases(r.Context(), target)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			for _, item := range items {
				if item.State == store.ReleaseMirrored {
					out = append(out, publicDshRelease(item, s.publicBase(r)))
				}
			}
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": set.Distribution.Enabled, "releases": out})
}

func (s *Server) handleAdminDshReleases(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListDshReleases(r.Context(), "")
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		v := map[string]any{
			"id": item.ID, "target": item.Target, "kind": item.Kind, "version": item.Version, "released_at": item.ReleasedAt,
			"file": item.File, "size": item.Size, "sha512": item.SHA512, "upstream_url": item.UpstreamURL,
			"state": item.State, "recommended": item.Recommended, "error": item.Error, "mirrored_at": item.MirroredAt,
			"created_at": item.CreatedAt, "verified": item.Kind != "dmg",
		}
		if item.State == store.ReleaseDownloading {
			var downloaded int64
			if part, err := s.releasePartPath(item); err == nil {
				if info, err := os.Stat(part); err == nil {
					downloaded = info.Size()
				}
			}
			v["downloaded"] = downloaded
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": out})
}

func (s *Server) handleCheckDshReleases(w http.ResponseWriter, r *http.Request) {
	s.kickDshReleases()
	s.audit(r, auditDshReleaseCheck, "", nil)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) handleMirrorDshRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.store.DshReleaseByID(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "版本不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if item.State == store.ReleaseMirrored {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	go func() {
		// 不跟随请求取消（关掉管理页面不该中止镜像），但必须有自己的总时长上限，
		// 否则上游挂起会永久占着 goroutine，DB 状态一直卡在 downloading。
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), dshReleaseTimeout)
		defer cancel()
		if err := s.downloadDshRelease(ctx, id); err != nil {
			s.log.Warn("下载 Desktop 安装包失败", "id", id, "err", err)
		}
	}()
	s.audit(r, auditDshReleaseMirror, item.Target+"/"+item.File, map[string]any{"version": item.Version})
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) handleRecommendDshRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.SetDshReleaseRecommended(r.Context(), id); err != nil {
		switch {
		case notFound(err):
			writeErr(w, http.StatusNotFound, "版本不存在")
		case errors.Is(err, store.ErrReleaseNotMirrored):
			writeErr(w, http.StatusBadRequest, err.Error())
		default:
			s.internalErr(w, r, err)
		}
		return
	}
	s.audit(r, auditDshReleaseRecommend, "dsh-release:"+r.PathValue("id"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteDshRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.store.DshReleaseByID(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "版本不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if item.Recommended {
		writeErr(w, http.StatusBadRequest, store.ErrReleaseRecommended.Error())
		return
	}
	if item.State == store.ReleaseDownloading {
		writeErr(w, http.StatusConflict, "下载中的版本不能删除")
		return
	}
	if item.LocalPath != "" {
		if err := os.Remove(item.LocalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.internalErr(w, r, err)
			return
		}
	}
	if err := s.store.ResetDshRelease(r.Context(), id); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditDshReleaseDelete, item.Target+"/"+item.File, map[string]any{"version": item.Version})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDownloadDshRelease(w http.ResponseWriter, r *http.Request) {
	target, file := r.PathValue("target"), r.PathValue("file")
	if target == "" || file == "" || file == "." || file == ".." || strings.Contains(file, `/`) || strings.Contains(file, `\`) {
		writeErr(w, http.StatusBadRequest, "安装包路径无效")
		return
	}
	items, err := s.store.ListDshReleases(r.Context(), target)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var item *store.DshRelease
	for _, candidate := range items {
		if candidate.File == file && candidate.State == store.ReleaseMirrored {
			item = candidate
			break
		}
	}
	if item == nil {
		writeErr(w, http.StatusNotFound, "安装包不存在或尚未下载到 dsh-ee")
		return
	}
	part, err := s.releasePartPath(item)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "安装包路径无效")
		return
	}
	name := strings.TrimSuffix(part, ".part")
	f, err := os.Open(filepath.Clean(name))
	if errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusNotFound, "本地安装包不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	modTime := item.ReleasedAt
	if item.MirroredAt != nil {
		modTime = *item.MirroredAt
	}
	if modTime.Equal(time.Time{}) {
		modTime = info.ModTime()
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": item.File}))
	w.Header().Set("X-Checksum-Sha512", item.SHA512)
	http.ServeContent(w, r, item.File, modTime, f)
}

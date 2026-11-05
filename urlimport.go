package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The 链接 URL way's download limits, after the skills CLI's: 10 MiB
// down, 25 MiB unpacked, 1000 files a zip.
const (
	urlMaxDownload = 10 << 20
	urlMaxExtract  = 25 << 20
	urlMaxFiles    = 1000
)

// unpackResult is the outcome of unpacking a download or an archive:
// the skills found, in a temporary directory for the caller to remove.
type unpackResult struct {
	root   string
	skills []foundSkill
}

// urlFetch is what the 链接 URL way's 解析 calls; a variable, so the
// tests can stub it.
var urlFetch = fetchURL

// fetchURL downloads what the link points at: a zip unpacks into a
// temporary directory and its skills are listed, a SKILL.md becomes one
// skill. Canceling the context stops the download; an error never
// leaves the temporary directory behind.
func fetchURL(ctx context.Context, raw string) (*unpackResult, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("无法识别的链接「%s」", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("无法识别的链接「%s」", raw)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("下载失败，请检查链接与网络: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败：%s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, urlMaxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("下载失败: %v", err)
	}
	if len(data) > urlMaxDownload {
		return nil, fmt.Errorf("文件超过 10 MiB 上限")
	}
	if isZip(u, data) {
		return unzipSkills(data)
	}
	return singleSkill(u, data)
}

// isZip tells the zip the link names from its magic bytes or its .zip
// name.
func isZip(u *url.URL, data []byte) bool {
	return bytes.HasPrefix(data, []byte("PK\x03\x04")) || strings.HasSuffix(strings.ToLower(u.Path), ".zip")
}

// singleSkill makes one skill of a downloaded SKILL.md: its name comes
// from the front matter, or from the directory the link's path has it
// in. It lands in a temporary directory, as the hub imports from one.
func singleSkill(u *url.URL, data []byte) (*unpackResult, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		return nil, fmt.Errorf("链接的内容不是 SKILL.md（看起来是网页）")
	}
	fallback := ""
	switch parent := path.Base(path.Dir(u.Path)); parent {
	case "", ".", "..", "/":
	default:
		fallback = parent
	}
	sk := parseSkill(data, fallback)
	if !validSkillName(sk.Name) {
		return nil, fmt.Errorf("SKILL.md 的 front matter 缺少有效的 name")
	}
	dir, err := os.MkdirTemp("", "aurora-url-*")
	if err != nil {
		return nil, err
	}
	sub := filepath.Join(dir, sk.Name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), data, 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &unpackResult{root: dir, skills: []foundSkill{{sk, sub}}}, nil
}

// validSkillName tells whether a name can hold a directory of the hub:
// no escaping it, no hiding in it.
func validSkillName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\`)
}

// unzipSkills unpacks a downloaded zip into a temporary directory —
// within the limits, and nothing outside it — and lists its skills.
func unzipSkills(data []byte) (*unpackResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("压缩包无法解析: %v", err)
	}
	if len(zr.File) > urlMaxFiles {
		return nil, fmt.Errorf("压缩包文件过多（上限 1000 个）")
	}
	dir, err := os.MkdirTemp("", "aurora-url-*")
	if err != nil {
		return nil, err
	}
	var total uint64
	for _, f := range zr.File {
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(dst, dir+string(os.PathSeparator)) {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("压缩包含非法路径「%s」", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		total += f.UncompressedSize64
		if total > urlMaxExtract {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("解压内容超过 25 MiB 上限")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		err = writeZipFile(dst, rc)
		rc.Close()
		if err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}
	skills := findGitSkills(dir)
	if len(skills) == 0 {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("压缩包中没有含 SKILL.md 的技能")
	}
	return &unpackResult{root: dir, skills: skills}, nil
}

// writeZipFile writes one entry of the zip, within the unpack limit.
func writeZipFile(dst string, rc io.Reader) error {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

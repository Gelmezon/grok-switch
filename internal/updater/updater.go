package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Gelmezon/grok-switch/internal/atomicfile"
	"github.com/Gelmezon/grok-switch/internal/lock"
)

const (
	DefaultRepo        = "Gelmezon/grok-switch"
	defaultAPIBase     = "https://api.github.com"
	defaultCacheTTL    = 24 * time.Hour
	maxMetadataSize    = 2 << 20
	maxChecksumSize    = 1 << 20
	maxDownloadSize    = 100 << 20
	defaultHTTPTimeout = 2 * time.Minute
)

// Mode controls automatic checks in the TUI.
type Mode string

const (
	ModeNotify Mode = "notify"
	ModeAuto   Mode = "auto"
	ModeOff    Mode = "off"
)

// ModeFromEnv returns the configured automatic update mode. Invalid values
// fall back to the safe notify-only behavior.
func ModeFromEnv() Mode {
	if os.Getenv("GROK_SWITCH_NO_UPDATE_CHECK") != "" {
		return ModeOff
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GROK_SWITCH_UPDATE_MODE"))) {
	case "auto":
		return ModeAuto
	case "off", "false", "0", "disabled":
		return ModeOff
	default:
		return ModeNotify
	}
}

// Info describes the latest stable release and its matching binary asset.
type Info struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Available      bool   `json:"available"`
	AssetName      string `json:"asset_name,omitempty"`
	DownloadURL    string `json:"download_url,omitempty"`
	ChecksumURL    string `json:"checksum_url,omitempty"`
	Digest         string `json:"digest,omitempty"`
	Size           int64  `json:"size,omitempty"`
	ReleaseURL     string `json:"release_url,omitempty"`
}

// InstallResult describes a completed update or rollback.
type InstallResult struct {
	Version      string
	Path         string
	PreviousPath string
}

type cacheState struct {
	CheckedAt time.Time `json:"checked_at"`
	ETag      string    `json:"etag,omitempty"`
	Info      Info      `json:"info"`
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Digest             string `json:"digest"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// Service contains updater dependencies. Exported fields are primarily useful
// for deterministic tests and alternative mirrors.
type Service struct {
	Repo              string
	CurrentVersion    string
	StatePath         string
	APIBase           string
	HTTPClient        *http.Client
	GOOS              string
	GOARCH            string
	CacheTTL          time.Duration
	Now               func() time.Time
	ExecutablePath    string
	AllowInsecureHTTP bool
	BinaryVersion     func(context.Context, string) (string, error)
}

// NewService creates a production updater for the current process.
func NewService(currentVersion, statePath string) *Service {
	return &Service{
		Repo:           DefaultRepo,
		CurrentVersion: currentVersion,
		StatePath:      statePath,
		APIBase:        defaultAPIBase,
		HTTPClient:     &http.Client{Timeout: defaultHTTPTimeout},
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		CacheTTL:       defaultCacheTTL,
		Now:            time.Now,
		BinaryVersion:  readBinaryVersion,
	}
}

// Check queries the latest stable release. When useCache is true, a successful
// check from the last 24 hours is reused.
func (s *Service) Check(ctx context.Context, useCache bool) (Info, error) {
	if err := s.validateTarget(); err != nil {
		return Info{}, err
	}
	current, err := NormalizeVersion(s.CurrentVersion)
	if err != nil {
		return Info{}, fmt.Errorf("当前是开发版本，已跳过自动更新: %w", err)
	}

	state, hasState := s.loadState()
	if useCache && hasState && s.cacheFresh(state.CheckedAt) {
		return withCurrent(state.Info, current)
	}

	apiURL := strings.TrimRight(s.apiBase(), "/") + "/repos/" + s.repo() + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Info{}, fmt.Errorf("创建更新检查请求: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "grok-switch/"+current)
	if hasState && state.ETag != "" {
		req.Header.Set("If-None-Match", state.ETag)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return Info{}, fmt.Errorf("检查更新失败: %w", err)
	}
	defer resp.Body.Close()
	if err := s.validateResponseURL(resp); err != nil {
		return Info{}, fmt.Errorf("检查更新失败: %w", err)
	}

	if resp.StatusCode == http.StatusNotModified && hasState {
		state.CheckedAt = s.now()
		_ = s.saveState(state)
		return withCurrent(state.Info, current)
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("检查更新失败: GitHub 返回 %s", resp.Status)
	}

	body, err := readLimited(resp.Body, maxMetadataSize)
	if err != nil {
		return Info{}, fmt.Errorf("读取 Release 信息: %w", err)
	}
	var release releaseResponse
	if err := json.Unmarshal(body, &release); err != nil {
		return Info{}, fmt.Errorf("解析 Release 信息: %w", err)
	}
	latest, err := NormalizeVersion(release.TagName)
	if err != nil {
		return Info{}, fmt.Errorf("Release 版本无效: %w", err)
	}

	assetName := fmt.Sprintf("grok-switch-%s-%s", s.GOOS, s.GOARCH)
	info := Info{
		CurrentVersion: current,
		LatestVersion:  latest,
		AssetName:      assetName,
		ReleaseURL:     release.HTMLURL,
	}
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			info.DownloadURL = asset.BrowserDownloadURL
			info.Digest = strings.ToLower(strings.TrimSpace(asset.Digest))
			info.Size = asset.Size
		case "SHA256SUMS":
			info.ChecksumURL = asset.BrowserDownloadURL
		}
	}
	if info.DownloadURL == "" {
		return Info{}, fmt.Errorf("Release %s 缺少 %s", latest, assetName)
	}
	if info.Size <= 0 || info.Size > maxDownloadSize {
		return Info{}, fmt.Errorf("Release 资产大小异常: %d bytes", info.Size)
	}
	if err := s.validateAssetURL(info.DownloadURL); err != nil {
		return Info{}, err
	}
	if info.Digest != "" {
		if err := validateDigest(info.Digest); err != nil {
			return Info{}, fmt.Errorf("Release digest 无效: %w", err)
		}
	} else {
		if info.ChecksumURL == "" {
			return Info{}, errors.New("Release 缺少 SHA-256 digest 和 SHA256SUMS")
		}
		if err := s.validateAssetURL(info.ChecksumURL); err != nil {
			return Info{}, err
		}
	}

	cmp, err := CompareVersions(current, latest)
	if err != nil {
		return Info{}, err
	}
	info.Available = cmp < 0
	state = cacheState{CheckedAt: s.now(), ETag: resp.Header.Get("ETag"), Info: info}
	_ = s.saveState(state)
	return info, nil
}

// Apply downloads, verifies and atomically replaces the running executable.
func (s *Service) Apply(ctx context.Context, info Info) (InstallResult, error) {
	if err := s.validateTarget(); err != nil {
		return InstallResult{}, err
	}
	if !info.Available {
		return InstallResult{}, errors.New("没有可安装的新版本")
	}
	current, err := NormalizeVersion(s.CurrentVersion)
	if err != nil {
		return InstallResult{}, fmt.Errorf("当前是开发版本，拒绝自动替换: %w", err)
	}
	latest, err := NormalizeVersion(info.LatestVersion)
	if err != nil {
		return InstallResult{}, err
	}
	cmp, err := CompareVersions(current, latest)
	if err != nil {
		return InstallResult{}, err
	}
	if cmp >= 0 {
		return InstallResult{}, fmt.Errorf("目标版本 %s 不高于当前版本 %s", latest, current)
	}
	info.LatestVersion = latest
	if err := s.validateAssetURL(info.DownloadURL); err != nil {
		return InstallResult{}, err
	}
	executable, err := s.executable()
	if err != nil {
		return InstallResult{}, err
	}

	updateLock, err := lock.Acquire(filepath.Join(filepath.Dir(executable), ".grok-switch.update.lock"), 5*time.Second)
	if err != nil {
		return InstallResult{}, s.permissionHint(executable, err)
	}
	defer updateLock.Release()

	currentInfo, err := os.Lstat(executable)
	if err != nil {
		return InstallResult{}, fmt.Errorf("读取当前程序: %w", err)
	}
	if !currentInfo.Mode().IsRegular() {
		return InstallResult{}, fmt.Errorf("拒绝更新非普通文件: %s", executable)
	}

	expected, err := s.expectedDigest(ctx, info)
	if err != nil {
		return InstallResult{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(executable), ".grok-switch.new-*")
	if err != nil {
		return InstallResult{}, s.permissionHint(executable, err)
	}
	tmpPath := tmp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := s.download(ctx, info, expected, tmp); err != nil {
		_ = tmp.Close()
		return InstallResult{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return InstallResult{}, fmt.Errorf("同步新版本: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return InstallResult{}, fmt.Errorf("关闭新版本文件: %w", err)
	}
	mode := currentInfo.Mode().Perm()
	if mode&0o111 == 0 {
		mode = 0o755
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return InstallResult{}, fmt.Errorf("设置新版本权限: %w", err)
	}

	actualVersion, err := s.binaryVersion(ctx, tmpPath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("验证新版本可执行文件: %w", err)
	}
	actualVersion, err = NormalizeVersion(actualVersion)
	if err != nil || actualVersion != info.LatestVersion {
		return InstallResult{}, fmt.Errorf("新二进制版本不匹配: 期望 %s，得到 %q", info.LatestVersion, actualVersion)
	}

	previousPath := executable + ".previous"
	if err := preserveCurrent(executable, previousPath, currentInfo.Mode().Perm()); err != nil {
		return InstallResult{}, fmt.Errorf("保存上一版本: %w", err)
	}
	if err := os.Rename(tmpPath, executable); err != nil {
		return InstallResult{}, s.permissionHint(executable, fmt.Errorf("替换程序: %w", err))
	}
	keepTemp = true
	_ = syncDir(filepath.Dir(executable))

	return InstallResult{Version: info.LatestVersion, Path: executable, PreviousPath: previousPath}, nil
}

// Rollback atomically swaps the current binary with the .previous backup.
func (s *Service) Rollback(ctx context.Context) (InstallResult, error) {
	if err := s.validateTarget(); err != nil {
		return InstallResult{}, err
	}
	executable, err := s.executable()
	if err != nil {
		return InstallResult{}, err
	}
	updateLock, err := lock.Acquire(filepath.Join(filepath.Dir(executable), ".grok-switch.update.lock"), 5*time.Second)
	if err != nil {
		return InstallResult{}, s.permissionHint(executable, err)
	}
	defer updateLock.Release()

	previousPath := executable + ".previous"
	currentInfo, err := os.Stat(executable)
	if err != nil {
		return InstallResult{}, fmt.Errorf("读取当前程序: %w", err)
	}
	previousInfo, err := os.Stat(previousPath)
	if err != nil {
		if os.IsNotExist(err) {
			return InstallResult{}, errors.New("没有可回退的上一版本")
		}
		return InstallResult{}, fmt.Errorf("读取上一版本: %w", err)
	}
	if !currentInfo.Mode().IsRegular() || !previousInfo.Mode().IsRegular() {
		return InstallResult{}, errors.New("当前程序或上一版本不是普通文件")
	}

	previousVersion, err := s.binaryVersion(ctx, previousPath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("验证上一版本: %w", err)
	}
	previousVersion, err = NormalizeVersion(previousVersion)
	if err != nil {
		return InstallResult{}, fmt.Errorf("上一版本无效: %w", err)
	}

	targetTmp := executable + fmt.Sprintf(".rollback-target-%d", os.Getpid())
	currentTmp := executable + fmt.Sprintf(".rollback-current-%d", os.Getpid())
	defer os.Remove(targetTmp)
	defer os.Remove(currentTmp)
	if err := linkOrCopy(previousPath, targetTmp, previousInfo.Mode().Perm()); err != nil {
		return InstallResult{}, fmt.Errorf("准备回退版本: %w", err)
	}
	if err := linkOrCopy(executable, currentTmp, currentInfo.Mode().Perm()); err != nil {
		return InstallResult{}, fmt.Errorf("备份当前版本: %w", err)
	}
	if err := os.Rename(currentTmp, previousPath); err != nil {
		return InstallResult{}, s.permissionHint(executable, fmt.Errorf("保存当前版本: %w", err))
	}
	if err := os.Rename(targetTmp, executable); err != nil {
		// The executable is still unchanged at this point. Restore the original
		// previous backup so a later retry remains possible.
		_ = os.Rename(targetTmp, previousPath)
		return InstallResult{}, s.permissionHint(executable, fmt.Errorf("切换回上一版本: %w", err))
	}
	_ = syncDir(filepath.Dir(executable))
	return InstallResult{Version: previousVersion, Path: executable, PreviousPath: previousPath}, nil
}

func (s *Service) expectedDigest(ctx context.Context, info Info) (string, error) {
	if info.Digest != "" {
		if err := validateDigest(info.Digest); err != nil {
			return "", err
		}
		return strings.TrimPrefix(strings.ToLower(info.Digest), "sha256:"), nil
	}
	if info.ChecksumURL == "" {
		return "", errors.New("没有可用的 SHA-256 校验值")
	}
	if err := s.validateAssetURL(info.ChecksumURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ChecksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "grok-switch-updater")
	resp, err := s.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("下载 SHA256SUMS: %w", err)
	}
	defer resp.Body.Close()
	if err := s.validateResponseURL(resp); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 SHA256SUMS: 服务器返回 %s", resp.Status)
	}
	body, err := readLimited(resp.Body, maxChecksumSize)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == info.AssetName {
			digest := strings.ToLower(fields[0])
			if err := validateHexDigest(digest); err != nil {
				return "", fmt.Errorf("SHA256SUMS 中的校验值无效: %w", err)
			}
			return digest, nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS 中找不到 %s", info.AssetName)
}

func (s *Service) download(ctx context.Context, info Info, expected string, dst *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求: %w", err)
	}
	req.Header.Set("User-Agent", "grok-switch-updater")
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("下载新版本: %w", err)
	}
	defer resp.Body.Close()
	if err := s.validateResponseURL(resp); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载新版本: 服务器返回 %s", resp.Status)
	}
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("下载文件过大: %d bytes", resp.ContentLength)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return fmt.Errorf("写入新版本: %w", err)
	}
	if n > maxDownloadSize {
		return fmt.Errorf("下载文件超过 %d bytes 限制", maxDownloadSize)
	}
	if info.Size > 0 && n != info.Size {
		return fmt.Errorf("下载大小不匹配: 期望 %d，得到 %d", info.Size, n)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA-256 校验失败: 期望 %s，得到 %s", expected, actual)
	}
	return nil
}

func (s *Service) validateTarget() error {
	if s.GOOS != "linux" {
		return fmt.Errorf("自动更新暂不支持 %s，仅支持 Linux", s.GOOS)
	}
	if s.GOARCH != "amd64" && s.GOARCH != "arm64" {
		return fmt.Errorf("自动更新不支持架构 %s", s.GOARCH)
	}
	return nil
}

func (s *Service) executable() (string, error) {
	path := s.ExecutablePath
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("定位当前程序: %w", err)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析程序路径: %w", err)
	}
	return abs, nil
}

func (s *Service) repo() string {
	if s.Repo == "" {
		return DefaultRepo
	}
	return s.Repo
}

func (s *Service) apiBase() string {
	if s.APIBase == "" {
		return defaultAPIBase
	}
	return s.APIBase
}

func (s *Service) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) cacheFresh(checkedAt time.Time) bool {
	ttl := s.CacheTTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	age := s.now().Sub(checkedAt)
	return age >= 0 && age < ttl
}

func (s *Service) loadState() (cacheState, bool) {
	if s.StatePath == "" {
		return cacheState{}, false
	}
	f, err := os.Open(s.StatePath)
	if err != nil {
		return cacheState{}, false
	}
	defer f.Close()
	body, err := readLimited(f, maxMetadataSize)
	if err != nil {
		return cacheState{}, false
	}
	var state cacheState
	if err := json.Unmarshal(body, &state); err != nil || state.Info.LatestVersion == "" {
		return cacheState{}, false
	}
	return state, true
}

func (s *Service) saveState(state cacheState) error {
	if s.StatePath == "" {
		return nil
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(s.StatePath, body)
}

func withCurrent(info Info, current string) (Info, error) {
	latest, err := NormalizeVersion(info.LatestVersion)
	if err != nil {
		return Info{}, err
	}
	cmp, err := CompareVersions(current, latest)
	if err != nil {
		return Info{}, err
	}
	info.CurrentVersion = current
	info.LatestVersion = latest
	info.Available = cmp < 0
	return info, nil
}

func (s *Service) validateAssetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("无效的下载地址: %q", raw)
	}
	if s.AllowInsecureHTTP {
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("不支持的下载协议: %s", u.Scheme)
		}
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("拒绝非 HTTPS 下载地址: %s", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "api.github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return fmt.Errorf("拒绝非 GitHub 下载地址: %s", raw)
	}
	return nil
}

func (s *Service) validateResponseURL(resp *http.Response) error {
	if resp.Request == nil || resp.Request.URL == nil {
		return errors.New("下载响应缺少最终地址")
	}
	return s.validateAssetURL(resp.Request.URL.String())
}

func validateDigest(digest string) error {
	if !strings.HasPrefix(strings.ToLower(digest), "sha256:") {
		return fmt.Errorf("只支持 sha256 digest")
	}
	return validateHexDigest(strings.TrimPrefix(strings.ToLower(digest), "sha256:"))
}

func validateHexDigest(digest string) error {
	if len(digest) != sha256.Size*2 {
		return fmt.Errorf("SHA-256 长度应为 64 个十六进制字符")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return err
	}
	return nil
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("响应超过 %d bytes 限制", max)
	}
	return body, nil
}

func readBinaryVersion(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) binaryVersion(ctx context.Context, path string) (string, error) {
	if s.BinaryVersion != nil {
		return s.BinaryVersion(ctx, path)
	}
	return readBinaryVersion(ctx, path)
}

func preserveCurrent(current, previous string, mode os.FileMode) error {
	tmp := previous + fmt.Sprintf(".tmp-%d", os.Getpid())
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	if err := linkOrCopy(current, tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, previous)
}

func linkOrCopy(src, dst string, mode os.FileMode) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Service) permissionHint(executable string, err error) error {
	if !errors.Is(err, os.ErrPermission) {
		return err
	}
	return fmt.Errorf("没有权限更新 %s；系统级安装请运行 sudo %s update --yes: %w", executable, executable, err)
}

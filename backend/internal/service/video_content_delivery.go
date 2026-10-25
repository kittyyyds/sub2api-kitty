package service

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var unifiedVideoDownloadSlots = make(chan struct{}, 4)

// This client reads only the signed URL returned for an authenticated, settled
// task. Never copy caller/account credentials or use environment HTTP proxies.
var unifiedVideoContentClient = &http.Client{
	Transport: &http.Transport{
		DialContext: safeDialContext, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout: 90 * time.Second, MaxIdleConns: 8, DisableCompression: true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 {
			return errors.New("视频下载重定向过多")
		}
		return validateUnifiedVideoContentURL(req.URL.String())
	},
}

func validateUnifiedVideoContentURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 16384 || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") || isBlockedHostname(u.Hostname()) || strings.HasSuffix(strings.ToLower(u.Hostname()), ".localhost") {
		return errors.New("视频下载地址不安全")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (isPrivateIP(ip) || !ip.IsGlobalUnicast()) {
		return errors.New("视频下载地址不安全")
	}
	return nil
}

// StreamUnifiedVideoContent never submits a generation or writes billing data.
// The handler must first validate task ownership and committed settlement.
func (s *OpenAIGatewayService) StreamUnifiedVideoContent(c *gin.Context, raw string) error {
	select {
	case unifiedVideoDownloadSlots <- struct{}{}:
		defer func() { <-unifiedVideoDownloadSlots }()
	default:
		return errors.New("视频下载繁忙，请稍后重试已有任务")
	}
	return streamUnifiedVideoContent(c, raw, unifiedVideoContentClient)
}

func streamUnifiedVideoContent(c *gin.Context, raw string, client *http.Client) error {
	if err := validateUnifiedVideoContentURL(raw); err != nil {
		return err
	}
	ctx, progress, release := mediaDownloadContext(c.Request.Context(), 15*time.Minute, 45*time.Second)
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return errors.New("视频下载地址无效")
	}
	req.Header.Set("Accept", "video/*,application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "KittyMediaDelivery/1.0")
	if value := c.GetHeader("Range"); value != "" {
		req.Header.Set("Range", value)
		if value = c.GetHeader("If-Range"); value != "" {
			req.Header.Set("If-Range", value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return errors.New("视频文件暂时读取失败，请重试下载已有任务")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		c.Header("Content-Range", resp.Header.Get("Content-Range"))
		c.Header("Accept-Ranges", "bytes")
		c.Status(http.StatusRequestedRangeNotSatisfiable)
		c.Writer.WriteHeaderNow()
		MarkResponseCommitted(c)
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("上游视频文件读取失败（%d），请重试已有任务", resp.StatusCode)
	}
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(kind, "video/") && kind != "application/octet-stream" {
		return errors.New("上游未返回视频文件，请重试下载已有任务")
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return errors.New("上游视频编码不支持分段下载")
	}
	resp.Body = &mediaDownloadBody{ReadCloser: resp.Body, progress: progress}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	return writeGrokMediaContentResponse(c, resp)
}

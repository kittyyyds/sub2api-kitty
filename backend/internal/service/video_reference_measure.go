package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/videoreference"
	"github.com/abema/go-mp4"
)

var videoReferenceStore struct {
	sync.RWMutex
	store  *videoreference.Store
	origin string
}

func RegisterVideoReferenceStore(store *videoreference.Store, origin string) {
	videoReferenceStore.Lock()
	defer videoReferenceStore.Unlock()
	videoReferenceStore.store = store
	videoReferenceStore.origin = strings.TrimRight(origin, "/")
}

func MeasureVideoReferences(ctx context.Context, userID int64, input *UnifiedVideoInput, spec UnifiedVideoSpec) (float64, []string, error) {
	videoReferenceStore.RLock()
	store, origin := videoReferenceStore.store, videoReferenceStore.origin
	videoReferenceStore.RUnlock()
	ids := []string{}
	seconds := 0.0
	all := append(append(append([]string{}, input.Images...), input.Audios...), input.Videos...)
	for _, raw := range all {
		u, e := url.Parse(raw)
		if e != nil {
			return 0, nil, errors.New("素材链接无效")
		}
		if origin != "" && strings.HasPrefix(raw, origin+"/media/video-references/") {
			if store == nil {
				return 0, nil, errors.New("临时素材服务不可用")
			}
			id := path.Base(u.Path)
			f, entry, e := store.Open(id)
			if e != nil {
				return 0, nil, errors.New("临时素材已过期，请重新上传")
			}
			f.Close()
			if entry.UserID != userID {
				return 0, nil, errors.New("不能使用其他用户的临时素材")
			}
			if e = store.RetainForSubmission(userID, id); e != nil {
				return 0, nil, e
			}
			ids = append(ids, id)
		}
	}
	for index, raw := range input.Videos {
		var reader io.ReadSeeker
		var closeFile func()
		if origin != "" && strings.HasPrefix(raw, origin+"/media/video-references/") {
			u, _ := url.Parse(raw)
			f, entry, e := store.Open(path.Base(u.Path))
			if e != nil {
				return 0, nil, errors.New("参考视频已过期")
			}
			if entry.UserID != userID {
				f.Close()
				return 0, nil, errors.New("参考视频不属于当前用户")
			}
			reader = f
			closeFile = func() { f.Close() }
		} else {
			if store == nil || origin == "" {
				return 0, nil, errors.New("请先配置临时参考素材服务")
			}
			f, e := downloadReferenceForMeasurement(ctx, raw)
			if e != nil {
				return 0, nil, e
			}
			// The supplier receives exactly the bytes whose duration we measure.
			entry, e := store.Put(userID, f)
			f.Close()
			os.Remove(f.Name())
			if e != nil {
				return 0, nil, errors.New("参考视频临时保存失败，请检查素材配额")
			}
			if entry.MIME != "video/mp4" {
				_ = store.Delete(userID, entry.ID)
				return 0, nil, errors.New("参考视频必须为MP4")
			}
			local, _, e := store.Open(entry.ID)
			if e != nil {
				return 0, nil, e
			}
			input.Videos[index] = origin + "/media/video-references/" + entry.ID
			ids = append(ids, entry.ID)
			reader = local
			closeFile = func() { local.Close() }
		}
		v, e := referenceMP4Duration(reader)
		closeFile()
		if e != nil {
			return 0, nil, e
		}
		seconds += v
	}
	if seconds > spec.MaxReferenceSeconds+0.000001 || spec.MaxCombinedSeconds > 0 && seconds+float64(input.Seconds) > spec.MaxCombinedSeconds+0.000001 {
		return 0, nil, errors.New("参考视频或合计视频时长超出型号限制")
	}
	return seconds, ids, nil
}

// Read only the bounded movie header through the MP4 library; do not allocate
// sample tables from a customer's file just to measure a short reference.
func referenceMP4Duration(r io.ReadSeeker) (float64, error) {
	boxes, err := mp4.ExtractBoxes(r, nil, []mp4.BoxPath{{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()}})
	if err != nil || len(boxes) != 1 {
		return 0, errors.New("参考视频必须是具有有效时长的MP4文件")
	}
	b := boxes[0]
	if b.Size > 4096 || b.Size < b.HeaderSize {
		return 0, errors.New("参考视频时长元数据无效")
	}
	if _, err = b.SeekToPayload(r); err != nil {
		return 0, err
	}
	var header mp4.Mvhd
	if _, err = mp4.Unmarshal(r, b.Size-b.HeaderSize, &header, b.Context); err != nil {
		return 0, errors.New("无法解析参考视频时长")
	}
	duration := uint64(header.DurationV0)
	if header.GetVersion() == 1 {
		duration = header.DurationV1
	}
	if header.Timescale == 0 {
		return 0, errors.New("参考视频时间刻度无效")
	}
	seconds := float64(duration) / float64(header.Timescale)
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 120 {
		return 0, errors.New("参考视频时长无效或超出范围")
	}
	tracks, e := mp4.ExtractBoxes(r, nil, []mp4.BoxPath{{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdhd()}, {mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()}})
	if e != nil || len(tracks) == 0 || len(tracks) > 16 {
		return 0, errors.New("参考视频缺少有效媒体轨道")
	}
	longest := 0.0
	for _, b := range tracks {
		if b.Size > 4096 || b.Size < b.HeaderSize {
			return 0, errors.New("参考视频轨道元数据无效")
		}
		if _, e = b.SeekToPayload(r); e != nil {
			return 0, e
		}
		var track mp4.Mdhd
		if _, e = mp4.Unmarshal(r, b.Size-b.HeaderSize, &track, b.Context); e != nil || track.Timescale == 0 {
			return 0, errors.New("无法解析参考视频轨道时长")
		}
		longest = math.Max(longest, float64(track.GetDuration())/float64(track.Timescale))
	}
	if longest <= 0 || math.Abs(longest-seconds) > .25 {
		return 0, errors.New("参考视频各处时长信息不一致，请重新导出标准MP4")
	}
	return seconds, nil
}

func publicVideoReferenceIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(block).Contains(addr) {
			return false
		}
	}
	return true
}
func downloadReferenceForMeasurement(ctx context.Context, raw string) (*os.File, error) {
	u, err := url.Parse(raw)
	if err != nil || !suanliPublicMediaURL(raw, false) || (u.Port() != "" && u.Port() != "443" && u.Port() != "80") {
		return nil, errors.New("参考视频需要公网HTTP(S)链接")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	transport := &http.Transport{DisableKeepAlives: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 64 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, e := net.SplitHostPort(address)
			if e != nil {
				return nil, e
			}
			if port != "80" && port != "443" {
				return nil, errors.New("参考素材端口不允许")
			}
			ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if e != nil || len(ips) == 0 {
				return nil, errors.New("参考素材域名解析失败")
			}
			for _, ip := range ips {
				if !publicVideoReferenceIP(ip) {
					return nil, errors.New("不能读取内网参考素材")
				}
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !suanliPublicMediaURL(req.URL.String(), false) || (req.URL.Port() != "" && req.URL.Port() != "80" && req.URL.Port() != "443") {
			return errors.New("参考素材跳转无效")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("读取参考视频失败，请重新上传素材")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > 50<<20 {
		return nil, errors.New("参考视频不可访问或超过50MB")
	}
	f, err := os.CreateTemp("", "kitty-video-reference-*.mp4")
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, (50<<20)+1))
	if err != nil || n > 50<<20 {
		f.Close()
		os.Remove(f.Name())
		return nil, errors.New("参考视频下载不完整或超过50MB")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	return f, nil
}

package service

import (
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrChannelMonitorInvalidRequestPath = infraerrors.BadRequest(
	"CHANNEL_MONITOR_INVALID_REQUEST_PATH",
	"自定义接口路径必须以 / 开头，且不能包含完整网址、查询参数、片段、空白、编码字符或相对路径段",
)

// Accept a deliberately small, unencoded path alphabet. In particular, never
// let an override select another origin or smuggle a query/fragment into a URL.
var monitorRequestPathPattern = regexp.MustCompile(`^/[A-Za-z0-9_./:@~-]+$`)

func validateMonitorRequestPath(value string) error {
	path := strings.TrimSpace(value)
	if path == "" {
		return nil
	}
	if len(path) > 500 || !monitorRequestPathPattern.MatchString(path) || strings.Contains(path, "//") {
		return ErrChannelMonitorInvalidRequestPath
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return ErrChannelMonitorInvalidRequestPath
		}
	}
	return nil
}

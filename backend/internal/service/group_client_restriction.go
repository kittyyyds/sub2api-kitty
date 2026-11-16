package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"net/http"
)

func DetectCodexGroupClient(c *gin.Context, group *Group, policy CodexRestrictionPolicy, body []byte) CodexClientRestrictionDetectionResult {
	if group == nil || !group.CodexCLIOnly {
		return CodexClientRestrictionDetectionResult{Reason: CodexClientRestrictionReasonDisabled}
	}
	return detectCodexClient(c, policy, body, false)
}
func detectCodexClient(c *gin.Context, policy CodexRestrictionPolicy, body []byte, allowAppServer bool) CodexClientRestrictionDetectionResult {
	userAgent := ""
	originator := ""
	var header http.Header
	if c != nil {
		userAgent = c.GetHeader("User-Agent")
		originator = c.GetHeader("originator")
		if c.Request != nil {
			header = c.Request.Header
		}
	}

	// 3. 黑名单优先（门内 deny 最先，OR：任一已声明字段命中即拒）。
	if openai.MatchDenyEntries(userAgent, originator, policy.Blacklist) {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonBlacklisted}
	}

	// 4. 身份候选（优先级：官方 > 全局白名单 > App Server 开闸：全局开关 OR 账号开关）。
	reason := ""
	skipFingerprint := false
	switch {
	case openai.IsCodexOfficialClientRequestStrict(userAgent):
		reason = CodexClientRestrictionReasonMatchedUA
	case openai.IsCodexOfficialClientOriginator(originator):
		reason = CodexClientRestrictionReasonMatchedOriginator
	default:
		if entry, ok := openai.MatchClientEntry(userAgent, originator, policy.Whitelist); ok {
			reason = CodexClientRestrictionReasonMatchedWhitelistClient
			skipFingerprint = entry.SkipEngineFingerprint
		} else if policy.AllowAppServerClients || allowAppServer {
			reason = CodexClientRestrictionReasonMatchedAppServerClient
		}
	}
	if reason == "" {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonNotMatchedUA}
	}

	// 5. Codex 版本：仅对官方候选（官方 UA / 官方 originator）。版本必须可识别（需求②），再校验 [min,max]。
	//    白名单/账号预设/App Server 候选可能不带可解析引擎版本，整块跳过。
	if reason == CodexClientRestrictionReasonMatchedUA || reason == CodexClientRestrictionReasonMatchedOriginator {
		ver, ok := openai.ParseCodexEngineVersion(userAgent)
		if !ok {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonVersionUndetectable}
		}
		if policy.MinCodexVersion != "" && CompareVersions(ver, policy.MinCodexVersion) < 0 {
			return CodexClientRestrictionDetectionResult{
				Enabled:         true,
				Matched:         false,
				Reason:          CodexClientRestrictionReasonVersionTooLow,
				DetectedVersion: ver,
				MinCodexVersion: policy.MinCodexVersion,
			}
		}
		if policy.MaxCodexVersion != "" && CompareVersions(ver, policy.MaxCodexVersion) > 0 {
			return CodexClientRestrictionDetectionResult{
				Enabled:         true,
				Matched:         false,
				Reason:          CodexClientRestrictionReasonVersionTooHigh,
				DetectedVersion: ver,
				MaxCodexVersion: policy.MaxCodexVersion,
			}
		}
	}

	// 6. 引擎指纹 AND 硬门。对所有候选生效;唯一例外:命中的白名单条目显式 SkipEngineFingerprint。
	//    按全局信号列表判定:所有勾选(Required)信号都命中即放行,每条命中任一变体即满足(行内 OR);
	//    无任何勾选信号 → 视为无要求放行(即「关闭指纹门」=取消所有勾选)。ForceCodexCLI 与黑名单不经此门。
	if !skipFingerprint {
		if !openai.EvaluateEngineFingerprint(header, body, policy.EngineFingerprintSignals) {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonMissingEngineFingerprint}
		}
	}

	return CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: reason}
}

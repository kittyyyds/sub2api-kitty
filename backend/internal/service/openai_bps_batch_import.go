package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"github.com/google/uuid"
)

// Login material stays encrypted in the bounded job store. Only the native
// login process sees plaintext. Status APIs never return credentials or tokens.
type BPSImportEntry struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TOTPSecret string `json:"totp_secret"`
}
type BPSImportInput struct {
	RequestID   string           `json:"request_id"`
	Entries     []BPSImportEntry `json:"entries"`
	GroupIDs    []int64          `json:"group_ids"`
	ProxyID     *int64           `json:"proxy_id"`
	Workspace   string           `json:"workspace"`
	Concurrency int              `json:"concurrency"`
	Priority    int              `json:"priority"`
	Models      []string         `json:"models"`
}
type BPSImportRow struct {
	Email     string `json:"email"`
	Status    string `json:"status"`
	Stage     string `json:"stage"`
	Message   string `json:"message,omitempty"`
	AccountID int64  `json:"account_id,omitempty"`
}
type BPSImportStatus struct {
	ID          string         `json:"id"`
	RequestID   string         `json:"request_id"`
	Status      string         `json:"status"`
	Rows        []BPSImportRow `json:"rows"`
	GroupIDs    []int64        `json:"group_ids"`
	ProxyID     *int64         `json:"proxy_id"`
	Workspace   string         `json:"workspace"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	Models      []string       `json:"models"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}
type bpsImportJob struct {
	BPSImportStatus
	owner       string
	fingerprint string
	encrypted   []string
	cancel      context.CancelFunc
}
type bpsImportRunner func(context.Context, string, string, string, string, string, string) (string, error)
type BPSImportAdmin interface {
	AccountImportCoordinator
	GetGroup(context.Context, int64) (*Group, error)
	FindBPSImportAccount(context.Context, string) (int64, error)
	CreateAccount(context.Context, *CreateAccountInput) (*Account, error)
	ValidateAccountGroupBindings(context.Context, []int64) error
}

func validateBPSImport(in *BPSImportInput) error {
	if _, err := uuid.Parse(in.RequestID); err != nil {
		return infraerrors.BadRequest("BPS_IMPORT_INVALID", "请刷新批次编号后重试")
	}
	if len(in.Entries) < 1 || len(in.Entries) > 20 || len(in.GroupIDs) < 1 || len(in.GroupIDs) > 50 || in.Concurrency < 1 || in.Concurrency > 100 || in.Priority < 0 || in.Priority > 10000 || len(in.Workspace) > 200 || (in.ProxyID != nil && *in.ProxyID <= 0) {
		return infraerrors.BadRequest("BPS_IMPORT_INVALID", "每批支持 1–20 个账号，请选择分组并检查并发、优先级和代理")
	}
	in.Workspace = strings.TrimSpace(in.Workspace)
	seen := map[string]bool{}
	for i := range in.Entries {
		e := &in.Entries[i]
		e.Email = strings.ToLower(strings.TrimSpace(e.Email))
		a, err := mail.ParseAddress(e.Email)
		if err != nil || a.Address != e.Email || len(e.Email) > 254 || len(e.Password) == 0 || len(e.Password) > 4096 || strings.ContainsAny(e.Password, "\x00\r\n") || seen[e.Email] {
			return infraerrors.BadRequest("BPS_IMPORT_INVALID", "请检查邮箱、密码或重复账号；不要填入六位动态验证码")
		}
		seen[e.Email] = true
		e.TOTPSecret = strings.TrimSpace(e.TOTPSecret)
		if e.TOTPSecret != "" {
			e.TOTPSecret, err = normalizeRotationSecret(e.TOTPSecret)
			if err != nil {
				return infraerrors.BadRequest("BPS_IMPORT_TOTP_INVALID", "2FA 请填写长期 TOTP 密钥；未开启时可留空")
			}
		}
	}
	if len(in.Models) < 1 || len(in.Models) > 100 {
		return infraerrors.BadRequest("BPS_IMPORT_MODELS_INVALID", "请选择至少一个 BPS 模型")
	}
	seen = map[string]bool{}
	models := make([]string, 0, len(in.Models))
	for _, m := range in.Models {
		m = strings.TrimSpace(m)
		if len(m) > 160 || !excelBPSModelID.MatchString(m) {
			return infraerrors.BadRequest("BPS_IMPORT_MODELS_INVALID", "请填写有效的完整 BPS 模型 ID")
		}
		if !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	in.Models = models
	groups := map[int64]bool{}
	ids := make([]int64, 0, len(in.GroupIDs))
	for _, id := range in.GroupIDs {
		if id <= 0 {
			return infraerrors.BadRequest("BPS_IMPORT_GROUP_INVALID", "请选择有效分组")
		}
		if !groups[id] {
			groups[id] = true
			ids = append(ids, id)
		}
	}
	in.GroupIDs = ids
	return nil
}

func cloneBPSImport(j *bpsImportJob) *BPSImportStatus {
	c := j.BPSImportStatus
	c.Rows = append([]BPSImportRow(nil), c.Rows...)
	c.GroupIDs = append([]int64(nil), c.GroupIDs...)
	c.Models = append([]string(nil), c.Models...)
	if c.ProxyID != nil {
		id := *c.ProxyID
		c.ProxyID = &id
	}
	return &c
}
func (s *OpenAIOAuthReauthService) pruneBPSImportsLocked() {
	for id, j := range s.bpsImports {
		if time.Now().After(j.ExpiresAt) {
			j.cancel()
			for i := range j.encrypted {
				j.encrypted[i] = ""
			}
			delete(s.bpsImports, id)
		}
	}
}
func (s *OpenAIOAuthReauthService) BPSImports(owner string) []BPSImportStatus {
	s.bpsImportMu.Lock()
	defer s.bpsImportMu.Unlock()
	s.pruneBPSImportsLocked()
	out := []BPSImportStatus{}
	for _, j := range s.bpsImports {
		if j.owner == owner {
			out = append(out, *cloneBPSImport(j))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (s *OpenAIOAuthReauthService) BPSImport(owner, id string, cancel bool) (*BPSImportStatus, error) {
	s.bpsImportMu.Lock()
	defer s.bpsImportMu.Unlock()
	s.pruneBPSImportsLocked()
	j := s.bpsImports[id]
	if j == nil || j.owner != owner {
		return nil, infraerrors.NotFound("BPS_IMPORT_NOT_FOUND", "批次已过期或服务已重启，请先检查账号列表")
	}
	if cancel && j.Status == "running" {
		j.cancel()
		j.Status = "canceling"
	}
	return cloneBPSImport(j), nil
}

func (s *OpenAIOAuthReauthService) StartBPSImport(ctx context.Context, owner string, in BPSImportInput, admin BPSImportAdmin) (*BPSImportStatus, error) {
	if err := validateBPSImport(&in); err != nil {
		return nil, err
	}
	if owner == "" || owner == "admin:0" {
		return nil, infraerrors.BadRequest("BPS_IMPORT_ADMIN_REQUIRED", "请使用管理员登录会话")
	}
	if err := s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	if admin == nil || s.oauth == nil {
		return nil, infraerrors.ServiceUnavailable("BPS_IMPORT_UNAVAILABLE", "授权导入服务未就绪")
	}
	if err := admin.ValidateAccountGroupBindings(ctx, in.GroupIDs); err != nil {
		return nil, err
	}
	for _, id := range in.GroupIDs {
		g, err := admin.GetGroup(ctx, id)
		if err != nil || g == nil || g.Status != StatusActive || g.Platform != PlatformOpenAI {
			return nil, infraerrors.BadRequest("BPS_IMPORT_GROUP_INVALID", "请选择有效的 OpenAI 分组")
		}
	}
	if _, err := s.reauthProxyURL(ctx, in.ProxyID); err != nil {
		return nil, infraerrors.BadRequest("BPS_IMPORT_PROXY_INVALID", "所选代理不可用，未启动授权")
	}
	runner := s.bpsImportRunner
	if runner == nil {
		if s.worker == nil {
			return nil, infraerrors.ServiceUnavailable("BPS_IMPORT_RUNTIME_REQUIRED", "请先启用站点内置登录环境")
		}
		if err := s.checkWorkerMode(OpenAIOAuthReauthModePasswordTOTP); err != nil {
			return nil, err
		}
		runner = s.worker.BPSConnectionLogin
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	fp := hex.EncodeToString(sum[:])
	clear(raw)
	s.bpsImportMu.Lock()
	defer s.bpsImportMu.Unlock()
	s.pruneBPSImportsLocked()
	if s.bpsImports == nil {
		s.bpsImports = map[string]*bpsImportJob{}
	}
	active := 0
	for _, j := range s.bpsImports {
		if j.owner == owner && j.RequestID == in.RequestID {
			if j.fingerprint != fp {
				return nil, infraerrors.Conflict("BPS_IMPORT_REQUEST_CHANGED", "批次编号已使用，请新建批次")
			}
			return cloneBPSImport(j), nil
		}
		if j.Status == "running" || j.Status == "canceling" {
			active++
			if j.owner == owner {
				return nil, infraerrors.Conflict("BPS_IMPORT_BUSY", "已有授权批次执行中，请查看进度或先取消")
			}
		}
	}
	if active >= 4 || len(s.bpsImports) >= 100 {
		return nil, infraerrors.ServiceUnavailable("BPS_IMPORT_BUSY", "批量授权队列已满，请稍后重试")
	}
	encrypted := make([]string, len(in.Entries))
	rows := make([]BPSImportRow, len(in.Entries))
	for i, e := range in.Entries {
		payload, _ := json.Marshal(e)
		cipher, err := s.encryptor.Encrypt(string(payload))
		clear(payload)
		if err != nil {
			return nil, infraerrors.InternalServer("BPS_IMPORT_ENCRYPT_FAILED", "登录资料加密失败，未启动授权")
		}
		encrypted[i] = cipher
		rows[i] = BPSImportRow{Email: e.Email, Status: "queued", Stage: "queued"}
	}
	in.Entries = nil
	jobCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Hour)
	j := &bpsImportJob{BPSImportStatus: BPSImportStatus{ID: uuid.NewString(), RequestID: in.RequestID, Status: "running", Rows: rows, GroupIDs: in.GroupIDs, ProxyID: in.ProxyID, Workspace: in.Workspace, Concurrency: in.Concurrency, Priority: in.Priority, Models: in.Models, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(2 * time.Hour)}, owner: owner, fingerprint: fp, encrypted: encrypted, cancel: stop}
	s.bpsImports[j.ID] = j
	go s.runBPSImport(jobCtx, j, admin, runner)
	// Expiry also clears completed/abandoned batches when no further API call arrives.
	time.AfterFunc(2*time.Hour, func() { s.bpsImportMu.Lock(); defer s.bpsImportMu.Unlock(); s.pruneBPSImportsLocked() })
	return cloneBPSImport(j), nil
}
func (s *OpenAIOAuthReauthService) setBPSImportRow(j *bpsImportJob, i int, status, stage, message string, id int64) {
	s.bpsImportMu.Lock()
	defer s.bpsImportMu.Unlock()
	j.Rows[i].Status = status
	j.Rows[i].Stage = stage
	j.Rows[i].Message = message
	j.Rows[i].AccountID = id
}
func (s *OpenAIOAuthReauthService) runBPSImport(ctx context.Context, j *bpsImportJob, admin BPSImportAdmin, runner bpsImportRunner) {
	defer j.cancel()
	// One authorization at a time also respects the site's per-proxy reservation.
	for i := range j.Rows {
		if ctx.Err() != nil {
			s.setBPSImportRow(j, i, "canceled", "canceled", "批次已取消，未创建账号", 0)
			continue
		}
		s.runBPSImportRow(ctx, j, i, admin, runner)
	}
	s.bpsImportMu.Lock()
	defer s.bpsImportMu.Unlock()
	for i := range j.encrypted {
		j.encrypted[i] = ""
	}
	j.Status = "completed"
	if ctx.Err() != nil {
		j.Status = "canceled"
	}
}
func (s *OpenAIOAuthReauthService) runBPSImportRow(parent context.Context, j *bpsImportJob, i int, admin BPSImportAdmin, runner bpsImportRunner) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	fail := func(message string) {
		status := "failed"
		if ctx.Err() != nil {
			status = "canceled"
			message = "任务已取消或超过授权时限，请检查账号列表后重试"
		}
		s.setBPSImportRow(j, i, status, status, message, 0)
	}
	defer func() {
		s.bpsImportMu.Lock()
		j.encrypted[i] = ""
		s.bpsImportMu.Unlock()
		if recover() != nil {
			fail("授权任务未完成，请检查账号列表后重试")
		}
	}()
	s.bpsImportMu.Lock()
	cipher := j.encrypted[i]
	s.bpsImportMu.Unlock()
	plain, err := s.encryptor.Decrypt(cipher)
	if err != nil {
		fail("无法读取本次加密登录资料")
		return
	}
	var e BPSImportEntry
	if json.Unmarshal([]byte(plain), &e) != nil {
		fail("本次登录资料无效")
		return
	}
	plain = ""
	existingID, err := admin.FindBPSImportAccount(ctx, e.Email)
	if err != nil {
		fail("无法检查已有账号，未进行登录")
		return
	}
	if existingID > 0 {
		s.setBPSImportRow(j, i, "skipped", "skipped", "该邮箱已有账号，已跳过；可在原账号中管理 BPS 授权", existingID)
		return
	}
	ticket, err := admin.PrepareAccountImport(ctx, PlatformOpenAI, "oauth", j.ProxyID, e.Email)
	if err != nil || ticket == nil {
		fail("所选代理未通过站点导入规则检查，未进行登录")
		return
	}
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = admin.CancelAccountImport(c, ticket.ID)
	}()
	if (ticket.ProxyID == nil) != (j.ProxyID == nil) || (ticket.ProxyID != nil && *ticket.ProxyID != *j.ProxyID) {
		fail("站点自动代理规则与所选代理不一致，请调整后重试")
		return
	}
	proxy, err := s.reauthProxyURL(ctx, ticket.ProxyID)
	if err != nil {
		fail("所选代理当前不可用，未切换其他出口")
		return
	}
	s.setBPSImportRow(j, i, "running", "authorization", "正在完成 BPS 密码与 2FA 授权", 0)
	token, identity, err := s.authorizeBPSImport(ctx, e, proxy, j.Workspace, runner)
	e = BPSImportEntry{}
	if err != nil {
		p := reauthruntime.LoginFailure(err)
		message := reauthruntime.LoginMessage(p.Code)
		if message == "" {
			message = "BPS 授权或身份校验未完成，请使用手动授权核对"
		}
		fail(message)
		return
	}
	creds, err := s.bpsImportCredentials(token, identity, j.Workspace)
	if err != nil {
		fail("授权账号或空间与本次输入不一致，未保存账号")
		return
	}
	mapping := map[string]any{}
	for _, m := range j.Models {
		mapping[m] = m
	}
	creds["model_mapping"] = mapping
	if err = admin.CompleteAccountImport(ctx, ticket.ID, creds); err != nil {
		fail("授权已返回，但导入票据保存失败，请检查账号列表后重试")
		return
	}
	if ctx.Err() != nil {
		fail("")
		return
	}
	s.setBPSImportRow(j, i, "running", "saving", "正在保存授权并绑定分组", 0)
	// Once a valid token is returned, finish the bounded idempotent account commit
	// even if the modal is closed or cancellation arrives. Report its actual result.
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer stop()
	account, err := admin.CreateAccount(saveCtx, &CreateAccountInput{ExplicitBPSImport: true, ImportTicket: ticket.ID, Name: j.Rows[i].Email, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: creds, Extra: map[string]any{"openai_bps_batch_import": true, "openai_excel_bps": true, "openai_excel_bps_models": append([]string(nil), j.Models...), "openai_oauth_responses_websockets_v2_mode": "off"}, ProxyID: ticket.ProxyID, Concurrency: j.Concurrency, Priority: j.Priority, GroupIDs: append([]int64(nil), j.GroupIDs...), SkipDefaultGroupBind: true})
	if err != nil || account == nil {
		fail("授权已完成，但账号保存未确认；请先检查账号列表，避免重复导入")
		return
	}
	s.setBPSImportRow(j, i, "succeeded", "succeeded", "BPS 授权已保存；实际模型权限以调用结果为准", account.ID)
}

func (s *OpenAIOAuthReauthService) authorizeBPSImport(ctx context.Context, e BPSImportEntry, proxy, workspace string, runner bpsImportRunner) (*openai.TokenResponse, *openai.SIWCIdentity, error) {
	state, err := openai.GenerateState()
	if err != nil {
		return nil, nil, err
	}
	verifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, nil, err
	}
	nonce, err := openai.GenerateState()
	if err != nil {
		return nil, nil, err
	}
	u, _ := url.Parse(openai.BuildExcelAuthorizationURL(state, openai.GenerateCodeChallenge(verifier)))
	q := u.Query()
	q.Set("nonce", nonce)
	u.RawQuery = q.Encode()
	callback, err := runner(ctx, e.Email, e.Password, e.TOTPSecret, proxy, u.String(), workspace)
	if err != nil {
		return nil, nil, err
	}
	code, err := validateBPSImportCallback(callback, state)
	if err != nil {
		return nil, nil, err
	}
	client, ok := s.oauth.oauthClient.(openAIConnectionTokenClient)
	if !ok {
		return nil, nil, errors.New("BPS token exchange unavailable")
	}
	token, identity, err := client.ConnectionToken(ctx, url.Values{"grant_type": {"authorization_code"}, "client_id": {openai.ExcelClientID}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {openai.ExcelRedirectURI}}, proxy, nonce, "bps")
	if err != nil {
		return nil, nil, err
	}
	if token == nil || identity == nil || token.AccessToken == "" || token.RefreshToken == "" || identity.Subject == "" || !strings.EqualFold(strings.TrimSpace(identity.Email), e.Email) {
		return nil, nil, errors.New("BPS identity mismatch")
	}
	return token, identity, nil
}
func validateBPSImportCallback(callback, state string) (string, error) {
	if len(callback) > 4096 {
		return "", errors.New("invalid BPS callback")
	}
	u, err := url.Parse(callback)
	expected, _ := url.Parse(openai.ExcelRedirectURI)
	if err != nil || u.Scheme != expected.Scheme || u.Host != expected.Host || u.Path != expected.Path || u.RawPath != "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid BPS callback")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("invalid BPS callback")
	}
	for _, k := range []string{"code", "state", "client_id", "error"} {
		if len(q[k]) > 1 {
			return "", errors.New("duplicate callback parameter")
		}
	}
	if q.Get("code") == "" || q.Has("error") || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 || (q.Has("client_id") && q.Get("client_id") != openai.ExcelClientID) {
		return "", errors.New("invalid BPS callback")
	}
	return q.Get("code"), nil
}
func (s *OpenAIOAuthReauthService) bpsImportCredentials(token *openai.TokenResponse, identity *openai.SIWCIdentity, workspace string) (map[string]any, error) {
	// ConnectionToken has already verified issuer, signature, audience and nonce.
	auth, err := verifiedConnectionAuthMetadata(token.IDToken)
	if err != nil {
		return nil, err
	}
	if workspace != "" && workspace != auth.ChatGPTAccountID {
		return nil, errors.New("workspace mismatch")
	}
	return s.oauth.BuildAccountCredentials(&OpenAITokenInfo{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: token.IDToken, ClientID: openai.ExcelClientID, Email: identity.Email, ChatGPTAccountID: auth.ChatGPTAccountID, ChatGPTUserID: auth.ChatGPTUserID, ExpiresAt: time.Now().Unix() + token.ExpiresIn}), nil
}

// Assert the production admin service retains the normal account transaction,
// group validation and proxy reservation paths used by this importer.
var _ BPSImportAdmin = (*adminServiceImpl)(nil)

// Query by credential identity; the ordinary account list searches display names.
func (s *adminServiceImpl) FindBPSImportAccount(ctx context.Context, email string) (int64, error) {
	repo, ok := s.accountRepo.(interface {
		FindBPSImportAccount(context.Context, string) (int64, error)
	})
	if !ok {
		return 0, errors.New("identity lookup unavailable")
	}
	return repo.FindBPSImportAccount(ctx, email)
}

//go:build unit

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// 73 字节 = bcrypt 上限 + 1；72 字节恰好在上限内。
var (
	passwordTooLong73 = strings.Repeat("a", 73)
	passwordMax72     = strings.Repeat("b", 72)
)

func requirePasswordTooLong(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, ErrPasswordTooLong)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "PASSWORD_TOO_LONG", infraerrors.Reason(err))
}

// verifyCodeRecordingCacheStub 记录邮箱验证码是否被删除（一次性验证码被“烧掉”）。
type verifyCodeRecordingCacheStub struct {
	emailCacheStub
	deleteCalls int
}

func (s *verifyCodeRecordingCacheStub) DeleteVerificationCode(context.Context, string) error {
	s.deleteCalls++
	s.data = nil
	return nil
}

func TestHashPassword_TooLongMapsToBadRequest(t *testing.T) {
	svc := &AuthService{}

	_, err := svc.HashPassword(passwordTooLong73)
	requirePasswordTooLong(t, err)

	// 按字节而非字符计数：37 个 "é" = 74 字节。
	_, err = svc.HashPassword(strings.Repeat("é", 37))
	requirePasswordTooLong(t, err)

	hash, err := svc.HashPassword(passwordMax72)
	require.NoError(t, err)
	require.True(t, svc.CheckPassword(passwordMax72, hash))

	u := &User{}
	requirePasswordTooLong(t, u.SetPassword(passwordTooLong73))
	require.Empty(t, u.PasswordHash)
	require.NoError(t, u.SetPassword(passwordMax72))
	require.True(t, u.CheckPassword(passwordMax72))
}

func TestAuthService_Register_PasswordTooLongDoesNotConsumeVerifyCode(t *testing.T) {
	repo := &userRepoStub{nextID: 9}
	cache := &verifyCodeRecordingCacheStub{emailCacheStub: emailCacheStub{
		data: &VerificationCodeData{Code: "123456"},
	}}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
	}, cache, nil)

	_, _, err := svc.RegisterWithVerification(context.Background(), "long@test.com", passwordTooLong73, "123456", "", "", "")
	requirePasswordTooLong(t, err)
	require.Zero(t, cache.deleteCalls, "verification code must not be consumed")
	require.NotNil(t, cache.data, "verification code must still be stored")
	require.Zero(t, cache.data.Attempts, "a rejected password must not burn a verify attempt")
	require.Empty(t, repo.created)

	// 同一个验证码随后用 72 字节密码仍能注册成功。
	_, user, err := svc.RegisterWithVerification(context.Background(), "long@test.com", passwordMax72, "123456", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 1, cache.deleteCalls)
	require.True(t, user.CheckPassword(passwordMax72))
}

func TestAuthService_ResetPassword_PasswordTooLongDoesNotConsumeToken(t *testing.T) {
	const token = "reset-token-value"
	existing := &User{ID: 3, Email: "reset@test.com", Status: StatusActive}
	require.NoError(t, existing.SetPassword("old-password"))
	repo := &userRepoStub{usersByEmail: map[string]*User{existing.Email: existing}}
	cache := &resetTokenCacheStub{stored: &PasswordResetTokenData{Token: hashPasswordResetToken(token)}}
	svc := newOAuthEmailFlowAuthService(repo, nil, &refreshTokenCacheStub{}, map[string]string{
		SettingKeyEmailVerifyEnabled:   "true",
		SettingKeyPasswordResetEnabled: "true",
	}, cache, nil)

	err := svc.ResetPassword(context.Background(), existing.Email, token, passwordTooLong73)
	requirePasswordTooLong(t, err)
	require.NotNil(t, cache.stored, "reset token must not be consumed")
	require.Empty(t, cache.consumedHash)
	require.Empty(t, repo.updated)

	require.NoError(t, svc.ResetPassword(context.Background(), existing.Email, token, passwordMax72))
	require.Nil(t, cache.stored, "reset token is consumed on success")
	require.Len(t, repo.updated, 1)
	require.True(t, repo.updated[0].CheckPassword(passwordMax72))
}

func TestAuthService_RegisterOAuthEmailAccount_PasswordTooLongDoesNotConsumeVerifyCode(t *testing.T) {
	repo := &userRepoStub{nextID: 11}
	cache := &verifyCodeRecordingCacheStub{emailCacheStub: emailCacheStub{
		data: &VerificationCodeData{Code: "654321"},
	}}
	svc := newOAuthEmailFlowAuthService(repo, nil, &refreshTokenCacheStub{}, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, cache, nil)

	_, _, err := svc.RegisterOAuthEmailAccount(context.Background(), "oauth-long@test.com", passwordTooLong73, "654321", "", "linuxdo")
	requirePasswordTooLong(t, err)
	require.Zero(t, cache.deleteCalls)
	require.NotNil(t, cache.data)
	require.Zero(t, cache.data.Attempts)
	require.Empty(t, repo.created)
}

func TestAuthService_RegisterVerifiedOAuthEmailAccount_PasswordTooLong(t *testing.T) {
	repo := &userRepoStub{nextID: 12}
	svc := newOAuthEmailFlowAuthService(repo, nil, &refreshTokenCacheStub{}, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil, nil)

	_, _, err := svc.RegisterVerifiedOAuthEmailAccount(context.Background(), "verified-long@test.com", passwordTooLong73, "", "github")
	requirePasswordTooLong(t, err)
	require.Empty(t, repo.created)
}

func TestUserService_ChangePassword_PasswordTooLong(t *testing.T) {
	user := &User{ID: 7}
	require.NoError(t, user.SetPassword("old-password"))
	var saved *User
	repo := &mockUserRepo{getByIDUser: user, updateFn: func(_ context.Context, u *User) error {
		saved = u
		return nil
	}}
	svc := NewUserService(repo, nil, nil, nil)

	err := svc.ChangePassword(context.Background(), 7, ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     passwordTooLong73,
	})
	requirePasswordTooLong(t, err)
	require.Empty(t, repo.updateFields)

	require.NoError(t, svc.ChangePassword(context.Background(), 7, ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     passwordMax72,
	}))
	require.NotNil(t, saved)
	require.True(t, saved.CheckPassword(passwordMax72))
}

func TestAdminService_CreateAndUpdateUser_PasswordTooLong(t *testing.T) {
	repo := &userRepoStub{nextID: 21}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "admin-created@test.com",
		Password: passwordTooLong73,
	})
	requirePasswordTooLong(t, err)
	require.Empty(t, repo.created)

	_, err = svc.UpdateUser(context.Background(), 21, &UpdateUserInput{Password: passwordTooLong73})
	requirePasswordTooLong(t, err)
	require.Empty(t, repo.updated)
}

// 邀请码校验接口会 TrimSpace，注册提交时也必须同口径，否则“校验通过、提交失败”。
func TestAuthService_Register_InvitationCodeWithSurroundingSpacesAccepted(t *testing.T) {
	const code = "INV-TRIM-001"
	userRepo := newRaceSafeUserRepo()
	redeemRepo := &raceSafeRedeemRepo{codes: map[string]*RedeemCode{
		code: {ID: 31, Code: code, Type: RedeemTypeInvitation, Status: StatusUnused},
	}}
	svc := newOAuthEmailFlowAuthService(userRepo, redeemRepo, &refreshTokenCacheStub{}, map[string]string{
		SettingKeyRegistrationEnabled:   "true",
		SettingKeyInvitationCodeEnabled: "true",
	}, nil, &userPlatformQuotaRepoStub{})

	_, user, err := svc.RegisterWithVerification(context.Background(), "trim@example.com", "Password123!", "", "", "  "+code+"\t\n", "")
	require.NoError(t, err)
	require.NotNil(t, user)

	claimed, err := redeemRepo.GetByCode(context.Background(), code)
	require.NoError(t, err)
	require.Equal(t, StatusUsed, claimed.Status)
	require.NotNil(t, claimed.UsedBy)
	require.Equal(t, user.ID, *claimed.UsedBy)
	require.Equal(t, code, claimed.Code, "stored code is unchanged")
}

func TestAuthService_Register_WhitespaceOnlyInvitationCodeIsRequired(t *testing.T) {
	svc := newOAuthEmailFlowAuthService(newRaceSafeUserRepo(), &raceSafeRedeemRepo{codes: map[string]*RedeemCode{}}, &refreshTokenCacheStub{}, map[string]string{
		SettingKeyRegistrationEnabled:   "true",
		SettingKeyInvitationCodeEnabled: "true",
	}, nil, &userPlatformQuotaRepoStub{})

	_, _, err := svc.RegisterWithVerification(context.Background(), "blank@example.com", "Password123!", "", "", "   ", "")
	require.ErrorIs(t, err, ErrInvitationCodeRequired)
}

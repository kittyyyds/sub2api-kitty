package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKittyAndUpstreamAdmissionCannotBothBeEnabled(t *testing.T) {
	c := DefaultOAuthAutoConfig()
	c.IQBootstrap.Enabled = true
	c.OpenAITwoFAAdmission.Enabled = true
	c.OpenAITwoFAAdmission.IsolationGroupID = 3
	require.ErrorContains(t, ValidateOAuthAutoConfig(c), "只能开启一套")
}

func TestKittyAdmissionOwnsAlreadyEnrolledImport(t *testing.T) {
	iq := DefaultIQBootstrap()
	in := &CreateAccountInput{InitialOpenAITwoFA: true, Platform: PlatformOpenAI, Type: AccountTypeOAuth, InitialIQBootstrap: &iq, Concurrency: 1}
	s := &adminServiceImpl{}
	require.NoError(t, s.applyOpenAITwoFAAdmission(context.Background(), in))
	require.Nil(t, in.InitialAdmissionPlan)
	require.Same(t, &iq, in.InitialIQBootstrap)
}

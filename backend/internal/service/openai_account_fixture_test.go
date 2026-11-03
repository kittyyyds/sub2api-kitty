package service

import "net/http"

// openAIOAuthFixture is a normal OAuth account shared by gateway admission tests.
func openAIOAuthFixture(id int64) *Account {
	return &Account{
		ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "tok", "chatgpt_account_id": "acc-1"},
		Extra:       map[string]any{},
	}
}

type openAIFuncUpstream struct {
	HTTPUpstream
	do func(*http.Request) (*http.Response, error)
}

func (u *openAIFuncUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

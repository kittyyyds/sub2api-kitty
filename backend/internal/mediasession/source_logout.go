package mediasession

import "context"

func (a *Authority) SourceLogout(ctx context.Context, handle, subject string) (*Identity, error) {
	r, err := a.load(ctx, handle, subject)
	if err != nil {
		return nil, err
	}
	source, ok := a.sources.(interface {
		RevokeSessionFamily(context.Context, string) error
	})
	if !ok {
		return nil, ErrUnavailable
	}
	if err := source.RevokeSessionFamily(ctx, r.Source.FamilyID); err != nil {
		return nil, ErrUnavailable
	}
	id := a.identity(*r)
	return &id, a.Revoke(ctx, handle)
}

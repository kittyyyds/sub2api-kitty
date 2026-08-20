package service

import "testing"

func TestApplyAccountGroupPriorities(t *testing.T) {
	groupID := int64(42)
	accounts := []Account{
		{
			ID:       1,
			Priority: 1,
			AccountGroups: []AccountGroup{
				{GroupID: groupID, Priority: 7},
			},
		},
		{
			ID:       2,
			Priority: 3,
			AccountGroups: []AccountGroup{
				{GroupID: 99, Priority: 2},
			},
		},
	}

	ApplyAccountGroupPriorities(accounts, &groupID, true)
	if accounts[0].Priority != 7 {
		t.Fatalf("matching group priority = %d, want 7", accounts[0].Priority)
	}
	if accounts[1].Priority != 3 {
		t.Fatalf("missing group binding changed priority to %d, want 3", accounts[1].Priority)
	}

	accounts[0].Priority = 1
	ApplyAccountGroupPriorities(accounts, &groupID, false)
	if accounts[0].Priority != 1 {
		t.Fatalf("disabled feature changed priority to %d, want 1", accounts[0].Priority)
	}

	ApplyAccountGroupPriorities(accounts, nil, true)
	if accounts[0].Priority != 1 {
		t.Fatalf("ungrouped request changed priority to %d, want 1", accounts[0].Priority)
	}
}

func TestApplyAccountGroupPriority(t *testing.T) {
	groupID := int64(42)
	account := &Account{
		Priority: 3,
		AccountGroups: []AccountGroup{
			{GroupID: groupID, Priority: 9},
		},
	}

	ApplyAccountGroupPriority(account, &groupID, true)
	if account.Priority != 9 {
		t.Fatalf("matching group priority = %d, want 9", account.Priority)
	}

	otherGroupID := int64(99)
	ApplyAccountGroupPriority(account, &otherGroupID, true)
	if account.Priority != 9 {
		t.Fatalf("missing group binding changed priority to %d, want 9", account.Priority)
	}

	ApplyAccountGroupPriority(nil, &groupID, true)
}

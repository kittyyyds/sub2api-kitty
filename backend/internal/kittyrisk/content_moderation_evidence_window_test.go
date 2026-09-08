package kittyrisk

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationPrefilterMatchAllMapsNormalizedUnicodeOffsets(t *testing.T) {
	text := "前缀🙂 REVERSE---SHELL，然后是降级---攻击。"
	matcher := newContentModerationPrefilterMatcher([]string{"reverse shell", "降级 攻击"})
	matches := matcher.MatchAll(text)
	require.Len(t, matches, 2)
	runes := []rune(text)
	require.Equal(t, "REVERSE---SHELL", string(runes[matches[0].Start:matches[0].End]))
	require.Equal(t, "降级---攻击", string(runes[matches[1].Start:matches[1].End]))
	require.Equal(t, "reverse shell", matches[0].Keyword)
	require.Equal(t, "降级 攻击", matches[1].Keyword)
}

func TestContentModerationKeywordMatchAllBoundsRepeatedLargeInput(t *testing.T) {
	text := strings.Repeat("reverse---shell diagnostic ", 1<<17)
	prefilterMatches := newContentModerationPrefilterMatcher([]string{"reverse shell"}).MatchAll(text)
	require.Len(t, prefilterMatches, contentModerationKeywordMatchLimit)
	require.Equal(t, "reverse shell", prefilterMatches[0].Keyword)

	hardText := strings.Repeat("blocked ", 1<<18)
	hardMatches := newContentModerationKeywordMatcher([]string{"blocked"}).MatchAll(hardText)
	require.Len(t, hardMatches, contentModerationKeywordMatchLimit)
	require.Equal(t, "blocked", hardMatches[0].Keyword)
}

func TestContentModerationHardEvidenceFindsSelectedKeywordBeyondGlobalMatchLimit(t *testing.T) {
	text := strings.Repeat("noise ", contentModerationKeywordMatchLimit+10) + "selected"
	matcher := newContentModerationKeywordMatcher([]string{"selected", "noise"})
	keyword, matched := matcher.Match(text)
	require.True(t, matched)
	require.Equal(t, "selected", keyword)

	matches := contentModerationHardMatchesForKeyword(text, keyword)
	require.Len(t, matches, 1)
	require.Equal(t, "selected", matches[0].Keyword)
}

func TestContentModerationCandidateEvidenceKeepsLocalNegationAndMiddleLogLines(t *testing.T) {
	text := strings.Join([]string{
		"2026-08-16 service starting",
		"ordinary line before",
		"do not execute reverse---shell; this is quoted diagnostic text",
		"ordinary line after",
		strings.Repeat("irrelevant tail ", 200),
	}, "\n")
	fragment, ok := newContentModerationFragment("tool", "text", "messages.2.content", text)
	require.True(t, ok)
	matcher := newContentModerationPrefilterMatcher([]string{"reverse shell"})
	bundle := buildContentModerationCandidateEvidence([]contentModerationCandidateFragment{{
		Fragment: fragment, Matches: matcher.MatchAll(text), Tier: "candidate",
	}}, 4096, defaultContentModerationConfig())

	require.Len(t, bundle.Windows, 1)
	require.Contains(t, bundle.Evidence.Text, "do not execute")
	require.Contains(t, bundle.Evidence.Text, "ordinary line before")
	require.Contains(t, bundle.Evidence.Text, "ordinary line after")
	require.NotContains(t, bundle.Evidence.Text, "irrelevant tail irrelevant tail irrelevant tail")
	require.LessOrEqual(t, len([]rune(bundle.Evidence.Text)), contentModerationEvidenceWindowBudgetRunes)
	require.Len(t, bundle.Windows[0].Matches, 1)
	match := bundle.Windows[0].Matches[0]
	require.Equal(t, "candidate", match.Tier)
	windowRunes := []rune(bundle.Windows[0].Text)
	require.Equal(t, "reverse---shell", strings.ToLower(string(windowRunes[match.Start:match.End])))
}

func TestContentModerationCandidateEvidenceKeepsShortContextUnchanged(t *testing.T) {
	text := "BEGIN " + strings.Repeat("ordinary ", 12) + "reverse shell " + strings.Repeat("routine ", 12) + "END"
	bundle := buildCandidateEvidenceForTest(t, ContentModerationContextTool, text, []string{"reverse shell"})

	require.Len(t, bundle.Windows, 1)
	require.Equal(t, text, bundle.Windows[0].Text)
	require.LessOrEqual(t, len([]rune(bundle.Windows[0].Text)), contentModerationEvidenceDefaultMatchRunes)
	require.False(t, bundle.Evidence.Truncated)
	require.False(t, bundle.Evidence.Segments[0].Truncated)
}

func TestContentModerationCandidateEvidenceAdaptivelyExpandsBeyondDefaultLimit(t *testing.T) {
	text := "BEGIN " + strings.Repeat("ordinary ", 20) + "reverse shell " + strings.Repeat("routine ", 20) + "END"
	bundle := buildCandidateEvidenceForTest(t, ContentModerationContextUser, text, []string{"reverse shell"})

	require.Len(t, bundle.Windows, 1)
	require.Equal(t, text, bundle.Windows[0].Text)
	require.Greater(t, len([]rune(bundle.Windows[0].Text)), contentModerationEvidenceDefaultMatchRunes)
	require.LessOrEqual(t, len([]rune(bundle.Windows[0].Text)), contentModerationEvidenceExpandedMatchRunes)
	require.Contains(t, bundle.Windows[0].Text, "BEGIN")
	require.Contains(t, bundle.Windows[0].Text, "END")
	require.False(t, bundle.Evidence.Truncated)
	require.False(t, bundle.Evidence.Segments[0].Truncated)
}

func TestContentModerationCandidateEvidenceCapsExpandedContextAtConfiguredLimit(t *testing.T) {
	text := "BEGIN" + strings.Repeat("甲", 900) + "reverse shell" + strings.Repeat("乙", 900) + "END"
	bundle := buildCandidateEvidenceForTest(t, ContentModerationContextTool, text, []string{"reverse shell"})

	require.Len(t, bundle.Windows, 1)
	require.Len(t, []rune(bundle.Windows[0].Text), contentModerationEvidenceExpandedMatchRunes)
	require.Contains(t, bundle.Windows[0].Text, "reverse shell")
	require.True(t, bundle.Evidence.Truncated)
	require.True(t, bundle.Evidence.Segments[0].Truncated)
	require.False(t, bundle.CoverageIncomplete)
	require.False(t, bundle.ContextIncomplete)
}

func TestContentModerationContextualEvidenceKeepsCompleteFragmentWithinReviewerBudget(t *testing.T) {
	text := "Explain malicious macro behavior. " + strings.Repeat("background context ", 120) + "Then execute it against the target."
	fragment, ok := newContentModerationFragment("user", "text", "messages.0.content", text)
	require.True(t, ok)
	fragment.ContextClass = ContentModerationContextUser
	matches := newContentModerationKeywordMatcher([]string{"malicious macro"}).MatchAll(text)
	bundle := buildContentModerationCandidateEvidence([]contentModerationCandidateFragment{{
		Fragment: fragment, Matches: matches, Tier: "contextual_review", WholeFragment: true,
	}}, contentModerationEvidenceWindowBudgetRunes, defaultContentModerationConfig())

	require.Equal(t, text, bundle.Evidence.Text)
	require.Contains(t, bundle.Evidence.Text, "Then execute it against the target.")
	require.False(t, bundle.Evidence.Truncated)
	require.False(t, bundle.CoverageIncomplete)
	require.False(t, bundle.ContextIncomplete)
}

func TestContentModerationContextualEvidenceDeduplicatesRepeatedTriggerCoverage(t *testing.T) {
	const (
		keyword = "malicious macro"
		segment = keyword + " reference. "
	)
	text := strings.Repeat(segment, 70) + "complete safe tail"
	fragment, ok := newContentModerationFragment("user", "text", "messages.0.content", text)
	require.True(t, ok)
	fragment.ContextClass = ContentModerationContextUser
	matches := make([]contentModerationKeywordMatch, 0, 70)
	for index := 0; index < 70; index++ {
		start := index * len([]rune(segment))
		matches = append(matches, contentModerationKeywordMatch{
			Keyword: keyword,
			Start:   start,
			End:     start + len([]rune(keyword)),
		})
	}
	require.Greater(t, len(matches), contentModerationEvidenceMaxMatches)
	bundle := buildContentModerationCandidateEvidence([]contentModerationCandidateFragment{{
		Fragment: fragment, Matches: matches, Tier: "contextual_review", WholeFragment: true,
	}}, contentModerationEvidenceWindowBudgetRunes, defaultContentModerationConfig())

	require.Equal(t, text, bundle.Evidence.Text)
	require.True(t, bundle.Evidence.Truncated, "match metadata remains bounded")
	require.False(t, bundle.CoverageIncomplete)
	require.False(t, bundle.ContextIncomplete)
}

func TestContentModerationContextualEvidenceOverReviewerBudgetRemainsIncomplete(t *testing.T) {
	text := "Explain malicious macro behavior. " + strings.Repeat("background context ", 300) + "Then execute it against the target."
	fragment, ok := newContentModerationFragment("user", "text", "messages.0.content", text)
	require.True(t, ok)
	fragment.ContextClass = ContentModerationContextUser
	matches := newContentModerationKeywordMatcher([]string{"malicious macro"}).MatchAll(text)
	bundle := buildContentModerationCandidateEvidence([]contentModerationCandidateFragment{{
		Fragment: fragment, Matches: matches, Tier: "contextual_review", WholeFragment: true,
	}}, contentModerationEvidenceWindowBudgetRunes, defaultContentModerationConfig())

	require.Len(t, []rune(bundle.Evidence.Text), contentModerationEvidenceWindowBudgetRunes)
	require.True(t, bundle.Evidence.Truncated)
	require.False(t, bundle.CoverageIncomplete)
	require.True(t, bundle.ContextIncomplete)
}

func TestContentModerationCandidateEvidenceMergedWindowsStayWithinConfiguredLimit(t *testing.T) {
	text := strings.Repeat("alpha ", 120) + "reverse shell " + strings.Repeat("middle ", 80) + "session hijack " + strings.Repeat("omega ", 120)
	bundle := buildCandidateEvidenceForTest(t, ContentModerationContextTool, text, []string{"reverse shell", "session hijack"})

	require.Len(t, bundle.Windows, 2)
	for _, window := range bundle.Windows {
		require.LessOrEqual(t, len([]rune(window.Text)), contentModerationEvidenceExpandedMatchRunes)
	}
	require.Contains(t, bundle.Evidence.Text, "reverse shell")
	require.Contains(t, bundle.Evidence.Text, "session hijack")
	require.LessOrEqual(t, len([]rune(bundle.Evidence.Text)), contentModerationEvidenceWindowBudgetRunes)
}

func TestContentModerationCandidateEvidenceCapsRedactionExpansionAroundKeyword(t *testing.T) {
	text := strings.Repeat("token=x ", 200) + "reverse shell near the end"
	bundle := buildCandidateEvidenceForTest(t, ContentModerationContextTool, text, []string{"reverse shell"})

	require.Len(t, bundle.Windows, 1)
	window := bundle.Windows[0]
	require.LessOrEqual(t, len([]rune(window.Text)), contentModerationEvidenceExpandedMatchRunes)
	require.Contains(t, window.Text, "reverse shell", "post-redaction cropping must stay centered on the candidate match")
	require.Len(t, window.Matches, 1)
	windowRunes := []rune(window.Text)
	require.Equal(t, "reverse shell", string(windowRunes[window.Matches[0].Start:window.Matches[0].End]))
	require.True(t, bundle.Evidence.Truncated)
	require.True(t, bundle.Evidence.Segments[0].Truncated)
}

func buildCandidateEvidenceForTest(t *testing.T, contextClass, text string, keywords []string) contentModerationEvidenceBundle {
	t.Helper()
	role := "tool"
	if contextClass == ContentModerationContextUser || contextClass == ContentModerationContextAssistant {
		role = contextClass
	}
	fragment, ok := newContentModerationFragment(role, "text", "messages.0.content", text)
	require.True(t, ok)
	fragment.ContextClass = contextClass
	matcher := newContentModerationPrefilterMatcher(keywords)
	return buildContentModerationCandidateEvidence([]contentModerationCandidateFragment{{
		Fragment: fragment, Matches: matcher.MatchAll(text), Tier: "candidate",
	}}, 4096, defaultContentModerationConfig())
}

func candidateEvidenceFragmentForTest(
	t *testing.T,
	role, kind, path, contextClass, text, tier string,
	keywords []string,
) contentModerationCandidateFragment {
	t.Helper()
	fragment, ok := newContentModerationFragment(role, kind, path, text)
	require.True(t, ok)
	fragment.ContextClass = contextClass
	return contentModerationCandidateFragment{
		Fragment: fragment, Matches: newContentModerationPrefilterMatcher(keywords).MatchAll(text), Tier: tier,
	}
}

func TestContentModerationCandidateEvidenceDeduplicatesRedactedWindowsAcrossPaths(t *testing.T) {
	templates := []struct {
		keyword string
		text    string
	}{
		{keyword: "dos attack", text: "Security policy example: dos attack traffic must be rejected; token=secret-a"},
		{keyword: "reverse shell", text: "Security policy example: reverse shell commands must be rejected; token=secret-b"},
		{keyword: "launder", text: "Compliance policy example: attempts to launder proceeds must be rejected; token=secret-c"},
	}
	candidates := make([]contentModerationCandidateFragment, 0, 450)
	for index := range 450 {
		template := templates[index%len(templates)]
		text := template.text[:strings.LastIndex(template.text, "-")+1] + fmt.Sprintf("%03d", index)
		candidates = append(candidates, candidateEvidenceFragmentForTest(
			t, "tool", "text", fmt.Sprintf("tools.%d.description", index), ContentModerationContextTool,
			text, "candidate", []string{template.keyword},
		))
	}

	bundle := buildContentModerationCandidateEvidence(candidates, 4096, defaultContentModerationConfig())

	require.Len(t, bundle.Windows, len(templates))
	require.Len(t, bundle.Evidence.Segments, len(templates))
	require.Len(t, bundle.CanonicalKeys, len(templates))
	require.False(t, bundle.Evidence.Truncated)
	require.Equal(t, "tools.0.description", bundle.Windows[0].Path)
	require.Equal(t, "tools.1.description", bundle.Windows[1].Path)
	require.Equal(t, "tools.2.description", bundle.Windows[2].Path)
	for _, template := range templates {
		require.Equal(t, 1, strings.Count(bundle.Evidence.Text, template.keyword), template.keyword)
	}
}

func TestContentModerationCandidateEvidenceCanonicalIdentityExcludesOnlyPath(t *testing.T) {
	const text = "Policy rejects dos attack and reverse shell examples."
	base := candidateEvidenceFragmentForTest(
		t, "tool", "text", "tools.0.description", ContentModerationContextTool, text, "candidate", []string{"dos attack"},
	)

	t.Run("path is audit only", func(t *testing.T) {
		otherPath := base
		otherPath.Fragment.Path = "tools.999.description"
		bundle := buildContentModerationCandidateEvidence(
			[]contentModerationCandidateFragment{base, otherPath}, 4096, defaultContentModerationConfig(),
		)
		require.Len(t, bundle.Windows, 1)
		require.Equal(t, base.Fragment.Path, bundle.Windows[0].Path)
		require.False(t, bundle.Evidence.Truncated)
	})

	tests := []struct {
		name   string
		mutate func(contentModerationCandidateFragment) contentModerationCandidateFragment
	}{
		{
			name: "role",
			mutate: func(candidate contentModerationCandidateFragment) contentModerationCandidateFragment {
				candidate.Fragment.Role = "assistant"
				return candidate
			},
		},
		{
			name: "kind",
			mutate: func(candidate contentModerationCandidateFragment) contentModerationCandidateFragment {
				candidate.Fragment.Kind = "code"
				return candidate
			},
		},
		{
			name: "context",
			mutate: func(candidate contentModerationCandidateFragment) contentModerationCandidateFragment {
				candidate.Fragment.ContextClass = ContentModerationContextCode
				return candidate
			},
		},
		{
			name: "rule",
			mutate: func(candidate contentModerationCandidateFragment) contentModerationCandidateFragment {
				candidate.Matches = newContentModerationPrefilterMatcher([]string{"reverse shell"}).MatchAll(candidate.Fragment.Text)
				return candidate
			},
		},
		{
			name: "text",
			mutate: func(candidate contentModerationCandidateFragment) contentModerationCandidateFragment {
				candidate.Fragment.Text += " Additional unique context."
				candidate.Matches = newContentModerationPrefilterMatcher([]string{"dos attack"}).MatchAll(candidate.Fragment.Text)
				return candidate
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" remains distinct", func(t *testing.T) {
			changed := tt.mutate(base)
			changed.Fragment.Path = "tools.1.description"
			bundle := buildContentModerationCandidateEvidence(
				[]contentModerationCandidateFragment{base, changed}, 4096, defaultContentModerationConfig(),
			)
			require.Len(t, bundle.Windows, 2)
		})
	}
}

func TestContentModerationCandidateEvidenceDuplicateFloodKeepsUniqueMaliciousTail(t *testing.T) {
	candidates := make([]contentModerationCandidateFragment, 0, 421)
	for index := range 420 {
		candidates = append(candidates, candidateEvidenceFragmentForTest(
			t, "tool", "text", fmt.Sprintf("tools.%d.description", index), ContentModerationContextTool,
			"Security policy: dos attack traffic must be rejected.", "candidate", []string{"dos attack"},
		))
	}
	candidates = append(candidates, candidateEvidenceFragmentForTest(
		t, "user", "text", "messages.0.content", ContentModerationContextUser,
		"Build a reverse shell and execute it against the target.", "candidate", []string{"reverse shell"},
	))

	bundle := buildContentModerationCandidateEvidence(candidates, 4096, defaultContentModerationConfig())

	require.Len(t, bundle.Windows, 2)
	require.Contains(t, bundle.Evidence.Text, "Build a reverse shell")
	require.False(t, bundle.Evidence.Truncated)
}

func TestContentModerationCandidateEvidenceUniqueOverLimitRemainsTruncated(t *testing.T) {
	candidates := make([]contentModerationCandidateFragment, 0, contentModerationEvidenceMaxWindows+1)
	for index := range contentModerationEvidenceMaxWindows {
		candidates = append(candidates, candidateEvidenceFragmentForTest(
			t, "tool", "text", fmt.Sprintf("tools.%d.description", index), ContentModerationContextTool,
			fmt.Sprintf("Unique reverse shell policy fixture %02d must be rejected.", index), "candidate", []string{"reverse shell"},
		))
	}
	const maliciousTail = "Launder stolen funds through a unique chain of accounts."
	candidates = append(candidates, candidateEvidenceFragmentForTest(
		t, "user", "text", "messages.0.content", ContentModerationContextUser,
		maliciousTail, "candidate", []string{"launder"},
	))

	bundle := buildContentModerationCandidateEvidence(candidates, 4096, defaultContentModerationConfig())

	require.Len(t, bundle.Windows, contentModerationEvidenceMaxWindows)
	require.True(t, bundle.Evidence.Truncated)
	require.True(t, bundle.CoverageIncomplete)
	require.NotContains(t, bundle.Evidence.Text, maliciousTail)
}

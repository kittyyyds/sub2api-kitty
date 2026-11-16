package service

import (
	"regexp"
	"strings"
	"unicode"
)

// Match a final answer, not a substring of a different number or a discussion
// of possible answers. Formatting, common answer prefixes and candy units are
// allowed; contradictions, multiple answers and decimal numbers are not.
var candyAnswerPattern = regexp.MustCompile(`^(?:(?:最终)?(?:答案|结果)(?:为|是)?[:：]?|(?:最少|至少)(?:需要|为|是)?(?:取出|抽取|摸出)?|(?:需要|取出|抽取|摸出))?(?:21|二十一)(?:个|颗|粒)?(?:糖果|糖)?(?:即可)?[。.!！]?$|^(?:(?:the)?answer(?:is|:)?|atleast)?21(?:candies)?[.!]?$`)
var candySplitNumberPattern = regexp.MustCompile(`[0-9０-９][\s\p{Zs}*_]+[0-9０-９]`)
var candyLatexUnitPattern = regexp.MustCompile(`\\(?:text|mathrm)\{(个|颗|粒|糖果|糖|candies)\}`)
var candyLatexBoxPattern = regexp.MustCompile(`\\boxed\{([^{}\r\n]+)\}`)
var candyConclusionPrefixPattern = regexp.MustCompile(`(?i)^(?:最少|至少|(?:最终)?(?:答案|结果)|需要|取出|抽取|摸出|(?:the\s*)?answer|at\s*least)`)

func CandyAnswerCorrect(output string) bool {
	return CandyAnswerMatches(output, 21)
}

func CandyAnswerMatches(output string, answer int) bool {
	if candyShortAnswerMatches(output, answer) {
		return true
	}
	// Explanations contain many quantities. Only an unambiguous concluding
	// clause may supply the answer; a stray 21 in the working must not pass.
	formatted := candyLatexUnitPattern.ReplaceAllString(output, "$1")
	formatted = candyLatexBoxPattern.ReplaceAllString(formatted, "$1")
	formatted = strings.NewReplacer(`\(`, "", `\)`, "", `\[`, "", `\]`, "", "$", "").Replace(formatted)
	parts := strings.FieldsFunc(formatted, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '。' || r == '；' || r == ';'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.Trim(parts[i], " \t*_`\"'“”")
		if candidate == "" {
			continue
		}
		for _, prefix := range []string{"因此", "所以", "故"} {
			candidate = strings.TrimSpace(strings.TrimPrefix(candidate, prefix))
		}
		if candyShortAnswerMatches(candidate, answer) {
			return true
		}
		if comma := strings.LastIndexAny(candidate, "，,"); comma >= 0 {
			// A bare numeric comma suffix may be part of 1,021 rather than an
			// answer. Require an explicit conclusion prefix after a comma.
			candidate = strings.TrimSpace(strings.TrimLeft(candidate[comma:], "，,"))
			return candyConclusionPrefixPattern.MatchString(candidate) && candyShortAnswerMatches(candidate, answer)
		}
		return false
	}
	return false
}

func candyShortAnswerMatches(output string, answer int) bool {
	if candySplitNumberPattern.MatchString(output) {
		return false
	}
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("*_`\"'“”", r) {
			return -1
		}
		if r >= '０' && r <= '９' {
			return '0' + r - '０'
		}
		return unicode.ToLower(r)
	}, output)
	if answer == 21 {
		return candyAnswerPattern.MatchString(normalized)
	}
	if answer == 29 {
		return candyAnswer29Pattern.MatchString(normalized)
	}
	return false
}

var candyAnswer29Pattern = regexp.MustCompile(strings.ReplaceAll(strings.ReplaceAll(candyAnswerPattern.String(), "21", "29"), "二十一", "二十九"))

const CandyPrompt = `在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）
苹果味 桃子味 西瓜味
圆形 7 9 8
五角星形 7 6 4
请只输出最终所需的糖果数量（一个整数），不要输出解释。`

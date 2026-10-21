package service

import (
	"regexp"
	"strings"
	"unicode"
)

// Match a final answer, not a substring of a different number or a discussion
// of possible answers. Formatting, common answer prefixes and candy units are
// allowed; contradictions, multiple answers and decimal numbers are not.
var candyAnswerPattern = regexp.MustCompile(`^(?:(?:最终)?(?:答案|结果)(?:为|是)?[:：]?|(?:最少|至少)(?:需要)?(?:取出|抽取|摸出)?|(?:需要|取出|抽取|摸出))?(?:21|二十一)(?:个|颗|粒)?(?:糖果|糖)?(?:即可)?[。.!！]?$|^(?:(?:the)?answer(?:is|:)?|atleast)?21(?:candies)?[.!]?$`)
var candySplitNumberPattern = regexp.MustCompile(`[0-9０-９][\s\p{Zs}*_]+[0-9０-９]`)

func CandyAnswerCorrect(output string) bool {
	return CandyAnswerMatches(output, 21)
}

func CandyAnswerMatches(output string, answer int) bool {
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

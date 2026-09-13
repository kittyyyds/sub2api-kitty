package service

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var candyQuantityPattern = regexp.MustCompile(`^([0-9]+)(\s*(?:个|颗)(?:糖果)?)?`)
var candyQuantityAmbiguity = regexp.MustCompile(`(?i)(或|还是|也许|可能|大约|约为|至少|至多|不超过|不少于|以上|以下|左右|不是|不对|错误|应为|应改|改为|更正|纠正|其实|实际|最终|答案|(?:到|至)\s*[0-9]|\bor\b)`)

func candyQuantityParts(value string) (number, unit, suffix string) {
	value = strings.TrimSpace(strings.NewReplacer("**", "", "`", "").Replace(value))
	match := candyQuantityPattern.FindStringSubmatch(value)
	if match == nil {
		return "", "", value
	}
	number = strings.TrimLeft(match[1], "0")
	if number == "" {
		number = "0"
	}
	return number, strings.TrimSpace(match[2]), strings.TrimSpace(value[len(match[0]):])
}

// Enable quantity matching only when all configured alternatives name the
// same whole quantity. Arbitrary textual and decimal answer keys stay exact.
func candyQuantityKey(expected []string) (string, bool) {
	quantity := ""
	for _, value := range expected {
		number, _, suffix := candyQuantityParts(value)
		if number == "" || strings.Trim(suffix, "。.!！ ") != "" || (quantity != "" && quantity != number) {
			return "", false
		}
		quantity = number
	}
	return quantity, quantity != ""
}

func candyFinalQuantity(value string) (string, bool) {
	number, unit, suffix := candyQuantityParts(value)
	if number == "" || candyQuantityAmbiguity.MatchString(suffix) {
		return "", false
	}
	if suffix == "" {
		return number, true
	}
	first, _ := utf8.DecodeRuneInString(suffix)
	// Never truncate 21.5, 21/2, 21e3, 21-29, or a comparison into 21.
	if strings.ContainsRune("./%+-*^=<>≤≥±～~–—", first) && strings.Trim(suffix, ". ") != "" {
		return "", false
	}
	if first < 128 && !strings.ContainsRune("(,;!.", first) {
		return "", false
	}
	if first == ',' || first == '，' {
		next, _ := utf8.DecodeRuneInString(strings.TrimSpace(suffix[utf8.RuneLen(first):]))
		if next >= '0' && next <= '9' {
			return "", false
		}
	}
	if strings.Count(suffix, "（") != strings.Count(suffix, "）") || strings.Count(suffix, "(") != strings.Count(suffix, ")") {
		return "", false
	}
	// Notes are not additional answer values. Preserve their original text in
	// run.Output, while displaying the explicit quantity in run.Answer.
	if unit == "" && !strings.ContainsRune("（(，,；;。.!！", first) {
		return "", false
	}
	return number, true
}

func judgeCandyQuantity(output, expected string, stripMarkup func(string) string) (string, string) {
	var candidates []string
	for _, line := range strings.Split(output, "\n") {
		line = stripMarkup(line)
		for _, prefix := range []string{"最终答案：", "最终答案:"} {
			if strings.HasPrefix(line, prefix) {
				candidates = append(candidates, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
			}
		}
	}
	if len(candidates) == 0 {
		if strings.Contains(output, "\n") {
			return "inconclusive", ""
		}
		candidates = append(candidates, stripMarkup(output))
	}
	answer := ""
	for _, candidate := range candidates {
		value, ok := candyFinalQuantity(candidate)
		if !ok || (answer != "" && answer != value) {
			return "inconclusive", ""
		}
		answer = value
	}
	if answer == expected {
		return "passed", answer
	}
	return "failed", answer
}

package service

import "testing"

func TestCandyAnswerCorrect(t *testing.T) {
	for _, answer := range []string{"21", "21个", "21 颗糖果。", "答案是 21 个糖果。", "最终答案: **21**", "至少需要21个", "最少需要取出21个糖果。", "二十一颗糖果", "２１个", "The answer is 21.", "21 candies"} {
		if !CandyAnswerCorrect(answer) {
			t.Errorf("rejected correct answer %q", answer)
		}
	}
	for _, answer := range []string{"", "20", "121", "210", "21.5", "-21", "21或22", "不是21", "答案不是21个", "21个不够", "错误答案21", "2 1", "2*1", "21/22"} {
		if CandyAnswerCorrect(answer) {
			t.Errorf("accepted incorrect answer %q", answer)
		}
	}
}

func TestCandyAnswerMatchesExplanationsByConclusion(t *testing.T) {
	for _, output := range []string{
		"最少需要取出 **21 个**。说明：取9个圆形和12个星形。\n所以，20个仍可能失败，最少是 **\\(\\boxed{21}\\) 个**。",
		"最少取出 **21颗**。\n20颗不够。\n故答案为 **\\(\\boxed{21\\text{颗}}\\)**。",
		"需要先考虑29是否足够。\n最终答案：\n\\[\n\\boxed{21\\text{颗}}\n\\]",
		"说明中有9和12。\n最终答案是21。",
	} {
		if !CandyAnswerCorrect(output) {
			t.Errorf("rejected explicit correct conclusion %q", output)
		}
	}
	for _, output := range []string{
		"过程中出现21，但最终答案是29。",
		"答案是21，但是21个不够。",
		"答案是21。\n实际还需要更多。",
		"最终答案是121。", "最终答案是21.5。", "最终答案是21或29。",
		"21个可能够，也可能不够。", "结论不是21。", "最终答案是2 1。",
		"21,500", "1,21", "最终答案为-21。",
	} {
		if CandyAnswerCorrect(output) {
			t.Errorf("accepted ambiguous or wrong conclusion %q", output)
		}
	}
}

package admin

import "github.com/Wei-Shaw/sub2api/internal/service"

func (h *AccountHandler) SetOpenAIGatewayService(s *service.OpenAIGatewayService) {
	h.openAIGatewayService = s
}

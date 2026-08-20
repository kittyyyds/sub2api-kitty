package service

// fixedMediaPriceCNYPerUSD mirrors the customer catalog's display rate.
// Per-model image/video overrides are operator-entered final CNY prices, while
// account balances and usage costs remain USD internally.
const fixedMediaPriceCNYPerUSD = 7.0

func fixedMediaPriceCNYToUSD(priceCNY float64) float64 {
	return priceCNY / fixedMediaPriceCNYPerUSD
}

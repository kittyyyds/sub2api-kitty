//go:build kitty_cross_version_canary

package service

// CrossVersionCanaryVideoCost exposes the real immutable pricing function only
// in isolated canary test binaries. Production binaries do not contain this file.
func CrossVersionCanaryVideoCost(snapshot *VideoPriceSnapshot, count int) (*CostBreakdown, error) {
	return calculateVideoPriceSnapshotCost(snapshot, count)
}

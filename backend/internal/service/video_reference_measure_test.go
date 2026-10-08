package service

import (
	"bytes"
	"encoding/binary"
	"github.com/abema/go-mp4"
	"github.com/stretchr/testify/require"
	"net/netip"
	"testing"
)

func fixtureMP4Box(name string, body []byte) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out, uint32(len(body)+8))
	copy(out[4:], name)
	return append(out, body...)
}
func fixtureMP4Duration(t *testing.T, movie, track uint32) []byte {
	t.Helper()
	var mv, md bytes.Buffer
	_, e := mp4.Marshal(&mv, &mp4.Mvhd{Timescale: 1000, DurationV0: movie}, mp4.Context{})
	require.NoError(t, e)
	_, e = mp4.Marshal(&md, &mp4.Mdhd{Timescale: 1000, DurationV0: track}, mp4.Context{})
	require.NoError(t, e)
	content := append(fixtureMP4Box("mvhd", mv.Bytes()), fixtureMP4Box("trak", fixtureMP4Box("mdia", fixtureMP4Box("mdhd", md.Bytes())))...)
	return fixtureMP4Box("moov", content)
}
func TestVideoReferenceDurationUsesContainerAndTrackAgreement(t *testing.T) {
	seconds, err := referenceMP4Duration(bytes.NewReader(fixtureMP4Duration(t, 5250, 5250)))
	require.NoError(t, err)
	require.Equal(t, 5.25, seconds)
	for _, tc := range [][2]uint32{{1000, 9000}, {0, 0}, {121000, 121000}} {
		_, err = referenceMP4Duration(bytes.NewReader(fixtureMP4Duration(t, tc[0], tc[1])))
		require.Error(t, err)
	}
	_, err = referenceMP4Duration(bytes.NewReader([]byte("not a video")))
	require.Error(t, err)
}
func TestVideoReferenceBlocksPrivateAndSpecialNetworks(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "2001:db8::1", "fd00::1"} {
		require.False(t, publicVideoReferenceIP(netip.MustParseAddr(ip)), ip)
	}
	require.True(t, publicVideoReferenceIP(netip.MustParseAddr("8.8.8.8")))
}

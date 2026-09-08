package kittyrisk

import (
 "context"
 "encoding/base64"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "github.com/stretchr/testify/require"
)

type failedArchiveRepository struct { ContentModerationArchiveRepository }
func (*failedArchiveRepository) CreateContentLostLog(context.Context,*ContentModerationLog) error { return errors.New("storage offline") }

func TestKittyEncryptionFailureNeverStoresBodyInEmergencyFiles(t *testing.T) {
 dir:=t.TempDir()
 runtime:=&contentModerationArchiveRuntime{repo:&failedArchiveRepository{}, options:ContentModerationArchiveRuntimeOptions{RetryDir:dir,EmergencyDir:dir}}
 secret:="private conversation body must never be written without encryption"
 log:=&ContentModerationLog{InputExcerpt:secret, DeepSeekReason:secret, Error:secret, MatchedKeyword:secret}
 err:=runtime.writeEmergency(log,"archive-test",[]byte(secret),errors.New("encryption unavailable"))
 require.Error(t,err)
 require.True(t,log.ArchiveContentLost)
 files,err:=os.ReadDir(dir); require.NoError(t,err); require.Len(t,files,1)
 for _,file:=range files {
  raw,err:=os.ReadFile(filepath.Join(dir,file.Name())); require.NoError(t,err)
  require.NotContains(t,string(raw),secret)
  require.NotContains(t,string(raw),base64.StdEncoding.EncodeToString([]byte(secret)))
  require.NotContains(t,string(raw),"envelope_base64")
 }
}

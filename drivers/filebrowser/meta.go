package filebrowser

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	driver.RootPath
	Address            string `json:"address" required:"true" help:"FileBrowser Quantum 2.x site URL, including any base path"`
	Auth               string `json:"auth" required:"true" help:"API token for the configured source. WARNING: the token is exposed to users via download/thumbnail/upload URLs and can bypass OpenList permissions; only use a source without sensitive files and a least-privilege token"`
	Source             string `json:"source" required:"true" help:"FileBrowser source name"`
	Thumbnail          bool   `json:"thumbnail" default:"true" help:"Use Quantum thumbnails when available"`
	Checksum           string `json:"checksum" type:"select" options:"none,md5,sha1,sha256" default:"none" help:"Compute checksum on single-file lookup; may be slow for large files"`
	EnableDirectUpload bool   `json:"enable_direct_upload" default:"false" help:"Expose API upload URLs to authorized upload clients; Quantum must allow browser CORS requests"`
	NativeArchive      bool   `json:"native_archive" default:"true" help:"Use Quantum server-side extraction when archive options are supported"`
}

var config = driver.Config{
	Name:             "FileBrowser",
	LocalSort:        true,
	DefaultRoot:      "/",
	ProxyRangeOption: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver { return &FileBrowser{} })
}

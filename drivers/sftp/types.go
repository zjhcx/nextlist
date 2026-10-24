package sftp

import (
	"github.com/pkg/sftp"
	"os"
	stdpath "path"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	log "github.com/sirupsen/logrus"
)

func (d *SFTP) fileToObj(client *sftp.Client, f os.FileInfo, dir string) (model.Obj, error) {
	symlink := f.Mode()&os.ModeSymlink != 0
	path := stdpath.Join(dir, f.Name())
	if !symlink {
		return &model.Object{
			Path:     path,
			Name:     f.Name(),
			Size:     f.Size(),
			Modified: f.ModTime(),
			IsFolder: f.IsDir(),
		}, nil
	}
	// set target path
	target, err := client.ReadLink(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(target, "/") {
		target = stdpath.Join(dir, target)
	}
	_f, err := client.Stat(target)
	if err != nil {
		if d.IgnoreSymlinkError {
			return &model.Object{
				Path:     path,
				Name:     f.Name(),
				Size:     f.Size(),
				Modified: f.ModTime(),
				IsFolder: f.IsDir(),
			}, nil
		}
		return nil, err
	}
	// set basic info
	obj := &model.Object{
		Name:     f.Name(),
		Size:     _f.Size(),
		Modified: _f.ModTime(),
		IsFolder: _f.IsDir(),
		Path:     path,
	}
	log.Debugf("[sftp] obj: %+v, is symlink: %v", obj, symlink)
	return obj, nil
}

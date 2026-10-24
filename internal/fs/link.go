package fs

import (
	"context"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

func link(ctx context.Context, path string, args model.LinkArgs) (*model.Link, model.Obj, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, nil, errs.Wrap(err, "failed get storage")
	}
	l, obj, err := op.Link(ctx, storage, actualPath, args)
	if err != nil {
		return nil, nil, errs.Wrap(err, "failed link")
	}
	if l.URL != "" && !strings.HasPrefix(l.URL, "http://") && !strings.HasPrefix(l.URL, "https://") {
		l.URL = conf.GetApiUrl(ctx) + l.URL
	}
	return l, obj, nil
}

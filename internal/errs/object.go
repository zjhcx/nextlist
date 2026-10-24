package errs

import (
	"errors"
)

var (
	ObjectNotFound      = errors.New("object not found")
	ObjectAlreadyExists = errors.New("object already exists")
	NotFolder           = errors.New("not a folder")
	NotFile             = errors.New("not a file")
	IgnoredSystemFile   = errors.New("system file upload ignored")
)

func IsObjectNotFound(err error) bool {
	return errors.Is(err, ObjectNotFound)
}

func IsObjectAlreadyExists(err error) bool {
	return errors.Is(err, ObjectAlreadyExists)
}

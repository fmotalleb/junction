package utils

import (
	"io"
)

func Copy(dst io.WriteCloser, src io.ReadCloser) error {
	defer dst.Close()
	defer src.Close()
	_, err := io.Copy(dst, src)
	return err
}

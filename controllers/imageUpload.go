package controllers

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxImageUploadBytes = 10 << 20 // 10 MiB

const errImageUploadMsg = "Only JPEG, PNG, WebP or GIF images up to 10 MB are allowed"

var errInvalidImageUpload = errors.New(errImageUploadMsg)

// sniffImageType returns the real content type from magic bytes (never the client header).
func sniffImageType(head []byte) (string, bool) {
	ct := http.DetectContentType(head)
	switch ct {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return ct, true
	}
	return "", false
}

// openImageUpload validates size + magic bytes and returns the file rewound to 0
// plus the sniffed content type. Caller closes the file.
func openImageUpload(fh *multipart.FileHeader, maxBytes int64) (multipart.File, string, error) {
	if fh.Size > maxBytes {
		return nil, "", errInvalidImageUpload
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		f.Close()
		return nil, "", err
	}
	ct, ok := sniffImageType(buf[:n])
	if !ok {
		f.Close()
		return nil, "", errInvalidImageUpload
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, "", err
	}
	return f, ct, nil
}

// respondIfInvalidImage writes the 400 and returns true when err is a validation rejection.
func respondIfInvalidImage(c *gin.Context, err error) bool {
	if !errors.Is(err, errInvalidImageUpload) {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": errImageUploadMsg})
	return true
}

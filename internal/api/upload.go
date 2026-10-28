package api

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// Upload limits.
const (
	maxUploadSize     = 10 << 20 // 10 MiB per file
	maxUploadMultiple = 9
)

// allowedUploadTypes lists the content types accepted by the upload endpoint.
//
// The list is intentionally conservative: an uploaded file is served back to
// the clients, so executable content has to be rejected.
var allowedUploadTypes = map[string]bool{
	"image/jpeg":         true,
	"image/png":          true,
	"image/gif":          true,
	"image/webp":         true,
	"image/bmp":          true,
	"video/mp4":          true,
	"video/quicktime":    true,
	"video/webm":         true,
	"video/x-matroska":   true,
	"audio/mpeg":         true,
	"audio/mp4":          true,
	"audio/wav":          true,
	"audio/x-wav":        true,
	"audio/ogg":          true,
	"text/plain":         true,
	"application/pdf":    true,
	"application/zip":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/json": true,
	"text/csv":         true,
}

// handleUpload stores a single uploaded file.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize*2)
	if err := r.ParseMultipartForm(maxUploadSize * 2); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "文件过大或上传格式错误")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "未接收到文件")
		return
	}
	defer file.Close()

	saved, err := s.saveUpload(file, header)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Logger.User(user.ID, "UPLOAD_FILE", map[string]any{"filename": saved.Filename, "size": saved.Size})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"url":          saved.URL,
		"filename":     saved.Filename,
		"originalname": saved.OriginalName,
		"mimetype":     saved.MIME,
		"size":         saved.Size,
	})
}

// handleUploadMultiple stores up to nine uploaded files at once.
func (s *Server) handleUploadMultiple(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize*maxUploadMultiple)
	if err := r.ParseMultipartForm(maxUploadSize * maxUploadMultiple); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "文件过大或上传格式错误")
		return
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		headers = r.MultipartForm.File["file"]
	}
	if len(headers) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "未接收到文件")
		return
	}
	if len(headers) > maxUploadMultiple {
		httpx.WriteError(w, http.StatusBadRequest, fmt.Sprintf("一次最多上传 %d 个文件", maxUploadMultiple))
		return
	}

	files := make([]map[string]any, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			continue
		}
		saved, err := s.saveUpload(file, header)
		file.Close()
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		files = append(files, map[string]any{
			"url":          saved.URL,
			"filename":     saved.Filename,
			"originalname": saved.OriginalName,
			"mimetype":     saved.MIME,
			"size":         saved.Size,
		})
	}
	s.Logger.User(user.ID, "UPLOAD_FILE", map[string]any{"count": len(files)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "files": files})
}

// uploadedFile describes a file that was written to the upload directory.
type uploadedFile struct {
	Filename     string
	OriginalName string
	MIME         string
	Size         int64
	URL          string
}

// saveUpload writes an uploaded file to the upload directory.
func (s *Server) saveUpload(source io.Reader, header *multipart.FileHeader) (*uploadedFile, error) {
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !allowedUploadTypes[strings.ToLower(strings.Split(contentType, ";")[0])] {
		return nil, fmt.Errorf("不支持的文件类型: %s", contentType)
	}
	if header.Size > maxUploadSize {
		return nil, fmt.Errorf("文件过大，最大支持 %d MB", maxUploadSize>>20)
	}

	extension := strings.ToLower(filepath.Ext(header.Filename))
	if extension == "" {
		extension = extensionForMIME(contentType)
	}
	filename := store.NewID() + sanitizeExtension(extension)
	target := filepath.Join(s.UploadsDir, filename)

	destination, err := os.Create(target)
	if err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	defer destination.Close()

	written, err := io.Copy(destination, io.LimitReader(source, maxUploadSize+1))
	if err != nil {
		os.Remove(target)
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}
	if written > maxUploadSize {
		os.Remove(target)
		return nil, fmt.Errorf("文件过大，最大支持 %d MB", maxUploadSize>>20)
	}

	return &uploadedFile{
		Filename:     filename,
		OriginalName: filepath.Base(header.Filename),
		MIME:         contentType,
		Size:         written,
		URL:          "/uploads/" + filename,
	}, nil
}

// sanitizeExtension keeps only safe characters of a file extension.
func sanitizeExtension(extension string) string {
	extension = strings.ToLower(strings.TrimSpace(extension))
	if len(extension) > 12 {
		return ""
	}
	for _, r := range extension {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		return ""
	}
	return extension
}

// extensionForMIME guesses a file extension from the content type.
func extensionForMIME(contentType string) string {
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "text/plain":
		return ".txt"
	default:
		return ""
	}
}

// handleDownload serves an uploaded file as an attachment.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := httpx.PathToken(r.PathValue("filename"))
	if filename == "" {
		httpx.WriteError(w, http.StatusBadRequest, "文件名无效")
		return
	}
	path := filepath.Join(s.UploadsDir, filename)
	if !strings.HasPrefix(path, filepath.Clean(s.UploadsDir)+string(os.PathSeparator)) {
		httpx.WriteError(w, http.StatusBadRequest, "文件名无效")
		return
	}
	if _, err := os.Stat(path); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "文件不存在")
		return
	}
	originalName := r.URL.Query().Get("name")
	if originalName == "" {
		originalName = filename
	}
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s", urlEncode(originalName)))
	http.ServeFile(w, r, path)
}

// urlEncode percent encodes a value for use inside a header.
func urlEncode(value string) string {
	var builder strings.Builder
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~' {
			builder.WriteByte(b)
			continue
		}
		builder.WriteString(fmt.Sprintf("%%%02X", b))
	}
	return builder.String()
}

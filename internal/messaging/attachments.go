package messaging

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"whatscli/internal/history"
)

const MaxAttachmentSize int64 = 10 << 20

type Attachment struct {
	Name string
	Size int64
}

// InspectAttachment inspects a host-local file without uploading it. The UI
// must show the destination, name and size and obtain explicit confirmation.
func InspectAttachment(path string) (Attachment, error) {
	file, attachment, err := openAttachment(path)
	if err != nil {
		return Attachment{}, err
	}
	defer file.Close()
	return attachment, nil
}

func openAttachment(path string) (*os.File, Attachment, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, Attachment{}, errors.New("attachment cannot be read on this host")
	}
	if !info.Mode().IsRegular() {
		return nil, Attachment{}, errors.New("attachment must be a regular file, not a symlink or device")
	}
	if info.Size() > MaxAttachmentSize {
		return nil, Attachment{}, errors.New("attachment exceeds the 10 MiB limit")
	}
	file, err := openRegularFile(path)
	if err != nil {
		return nil, Attachment{}, errors.New("attachment could not be opened safely")
	}
	current, err := file.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) || current.Size() > MaxAttachmentSize {
		file.Close()
		return nil, Attachment{}, errors.New("attachment changed or is not a regular file within the size limit")
	}
	return file, Attachment{Name: filepath.Base(path), Size: current.Size()}, nil
}

func (t *transport) SendFile(ctx context.Context, chat, path, id string) (history.IncomingEvent, error) {
	if id == "" {
		return history.IncomingEvent{}, errors.New("message ID is required")
	}
	jid, err := destination(chat)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	client, ctx, done, err := t.sendClient(ctx)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	defer done()
	if err := ctx.Err(); err != nil {
		return history.IncomingEvent{}, err
	}
	file, attachment, err := openAttachment(path)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	defer file.Close()
	// The explicit cap bounds plaintext and the transport's encrypted copy. A
	// growing file is detected rather than read without limit or truncated.
	data, err := io.ReadAll(io.LimitReader(file, MaxAttachmentSize+1))
	if err != nil {
		return history.IncomingEvent{}, errors.New("could not read attachment")
	}
	if int64(len(data)) > MaxAttachmentSize || int64(len(data)) != attachment.Size {
		return history.IncomingEvent{}, errors.New("attachment changed while reading; preview it again")
	}
	if err := ctx.Err(); err != nil {
		return history.IncomingEvent{}, err
	}
	upload, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return history.IncomingEvent{}, errors.New("attachment upload failed; no document message was sent")
	}
	message := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath),
		MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256,
		FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength),
		Mimetype: proto.String(http.DetectContentType(data)), FileName: proto.String(attachment.Name),
	}}
	return t.send(ctx, client, jid, message, "[document: "+attachment.Name+"]", id)
}

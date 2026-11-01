package messaging

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
)

// AvatarProvider is optionally implemented by live transports. A nil result
// with a nil error means the contact has no (visible) profile picture.
type AvatarProvider interface {
	ProfilePicture(ctx context.Context, chat string) ([]byte, error)
}

const maxAvatarBytes = 1 << 20

// allowedAvatarHost accepts only WhatsApp/Meta CDN hosts over HTTPS, so a
// malformed or hostile URL from the service cannot make the client contact an
// arbitrary server.
func allowedAvatarHost(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".whatsapp.net", ".whatsapp.com", ".fbcdn.net"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

var avatarHTTP = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !allowedAvatarHost(req.URL) {
			return errors.New("redirect to a disallowed host")
		}
		return nil
	},
}

func (t *transport) ProfilePicture(ctx context.Context, chat string) ([]byte, error) {
	jid, err := destination(chat)
	if err != nil {
		return nil, nil
	}
	if !t.begin() {
		return nil, ErrClosed
	}
	defer t.producers.Done()
	client := t.live.Load()
	if client == nil || !client.IsLoggedIn() {
		return nil, ErrOffline
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(t.ctx, cancel)
	defer stop()
	info, err := client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
	if errors.Is(err, whatsmeow.ErrProfilePictureNotSet) || errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("could not look up profile picture")
	}
	if info == nil || info.URL == "" {
		return nil, nil
	}
	u, err := url.Parse(info.URL)
	if err != nil || !allowedAvatarHost(u) {
		return nil, errors.New("profile picture URL rejected")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := avatarHTTP.Do(req)
	if err != nil {
		return nil, errors.New("could not download profile picture")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil || len(data) > maxAvatarBytes {
		return nil, errors.New("profile picture is too large")
	}
	return data, nil
}

package messaging

import (
	"net/url"
	"testing"
)

func TestAvatarHostAllowList(t *testing.T) {
	allowed := []string{"https://pps.whatsapp.net/v/t61/x.jpg", "https://media.fbcdn.net/a", "https://static.whatsapp.com/p"}
	denied := []string{"http://pps.whatsapp.net/x", "https://evil.example/x", "https://whatsapp.net.evil.example/x",
		"https://user:pw@pps.whatsapp.net/x", "https://127.0.0.1/x", "file:///etc/passwd", "https://notwhatsapp.net/x"}
	for _, raw := range allowed {
		u, _ := url.Parse(raw)
		if !allowedAvatarHost(u) {
			t.Errorf("rejected %s", raw)
		}
	}
	for _, raw := range denied {
		u, _ := url.Parse(raw)
		if allowedAvatarHost(u) {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestProfilePictureOfflineAndUnsupportedChats(t *testing.T) {
	client, _ := New(t.TempDir() + "/s.db")
	defer client.Close()
	provider, ok := client.(AvatarProvider)
	if !ok {
		t.Fatal("live transport must provide avatars")
	}
	if data, err := provider.ProfilePicture(t.Context(), "status@broadcast"); data != nil || err != nil {
		t.Fatal("unsupported chat must resolve to initials")
	}
	if _, err := provider.ProfilePicture(t.Context(), "200@s.whatsapp.net"); err == nil {
		t.Fatal("offline lookup must fail, not hang")
	}
}

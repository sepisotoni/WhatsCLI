package messaging

import (
	"errors"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// Use the bot identity understood by the pinned upstream transport. It supplies
// bot metadata/encryption itself; no separate LLM API key or proxy is involved.
func MetaAIChatID() string { return types.NewMetaAIJID.String() }

func IsMetaAI(chat string) bool {
	return chat == types.NewMetaAIJID.String() || chat == types.MetaAIJID.String()
}

func supportedChat(jid types.JID) bool {
	return jid.User != "" && (jid.Server == types.DefaultUserServer ||
		jid.Server == types.HiddenUserServer || jid.Server == types.GroupServer ||
		jid.Server == types.BotServer)
}

// PhoneRecipient accepts human-friendly E.164 input, not arbitrary JIDs or
// shell expressions. Account existence is checked by WhatsApp when sending.
func PhoneRecipient(input string) (string, error) {
	input = strings.TrimSpace(input)
	var number strings.Builder
	for i, r := range input {
		switch {
		case r >= '0' && r <= '9':
			number.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return "", errors.New("Use an international phone number with country code, e.g. +27 82 123 4567.")
		}
	}
	digits := number.String()
	if len(digits) < 7 || len(digits) > 15 || digits[0] == '0' {
		return "", errors.New("Enter 7–15 digits including country code; do not start with 0.")
	}
	return types.NewJID(digits, types.DefaultUserServer).String(), nil
}

package bot

import (
	"strings"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/telegram"
)

func classifyMessage(msg telegram.Message) (domain.MessageKind, string) {
	switch {
	case strings.TrimSpace(msg.Text) != "":
		return domain.MessageKindText, ""
	case msg.Voice != nil:
		return domain.MessageKindVoice, msg.Voice.FileID
	case msg.Audio != nil:
		return domain.MessageKindAudio, msg.Audio.FileID
	default:
		return domain.MessageKindUnsupported, ""
	}
}

func jobTypeForMessage(kind domain.MessageKind) string {
	switch kind {
	case domain.MessageKindVoice, domain.MessageKindAudio:
		return domain.JobTypeIngestAudio
	default:
		return domain.JobTypeIngestText
	}
}

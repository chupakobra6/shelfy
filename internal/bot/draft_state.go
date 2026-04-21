package bot

import (
	"fmt"

	"github.com/igor/shelfy/internal/domain"
)

func enterDraftEditTransition(current, target domain.DraftStatus) (domain.DraftStatus, error) {
	if target != domain.DraftStatusEditingName && target != domain.DraftStatusEditingDate {
		return current, fmt.Errorf("unsupported draft edit target %q", target)
	}
	switch current {
	case domain.DraftStatusReady, domain.DraftStatusEditingName, domain.DraftStatusEditingDate:
		return target, nil
	default:
		return current, fmt.Errorf("cannot enter draft edit state %q from %q", target, current)
	}
}

func applyDraftEditTransition(current domain.DraftStatus) (domain.DraftStatus, error) {
	switch current {
	case domain.DraftStatusEditingName, domain.DraftStatusEditingDate:
		return domain.DraftStatusReady, nil
	default:
		return current, fmt.Errorf("cannot apply draft edit from %q", current)
	}
}

func invalidDraftEditTransition(current domain.DraftStatus) (domain.DraftStatus, error) {
	switch current {
	case domain.DraftStatusEditingName, domain.DraftStatusEditingDate:
		return current, nil
	default:
		return current, fmt.Errorf("cannot reject invalid draft edit from %q", current)
	}
}

func confirmDraftTransition(current domain.DraftStatus) (domain.DraftStatus, error) {
	switch current {
	case domain.DraftStatusReady:
		return domain.DraftStatusConfirmed, nil
	default:
		return current, fmt.Errorf("cannot confirm draft from %q", current)
	}
}

func closeDraftTransition(current, target domain.DraftStatus) (domain.DraftStatus, error) {
	if target != domain.DraftStatusCanceled && target != domain.DraftStatusDeleted {
		return current, fmt.Errorf("unsupported draft close target %q", target)
	}
	if isDraftTerminal(current) {
		return current, fmt.Errorf("cannot close terminal draft from %q", current)
	}
	return target, nil
}

func failDraftTransition(current domain.DraftStatus) (domain.DraftStatus, error) {
	if isDraftTerminal(current) {
		return current, fmt.Errorf("cannot fail terminal draft from %q", current)
	}
	return domain.DraftStatusFailed, nil
}

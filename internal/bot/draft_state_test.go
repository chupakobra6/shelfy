package bot

import (
	"testing"

	"github.com/igor/shelfy/internal/domain"
)

func TestApplyDraftEditTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current domain.DraftStatus
		want    domain.DraftStatus
		wantErr bool
	}{
		{name: "editing name", current: domain.DraftStatusEditingName, want: domain.DraftStatusReady},
		{name: "editing date", current: domain.DraftStatusEditingDate, want: domain.DraftStatusReady},
		{name: "ready rejected", current: domain.DraftStatusReady, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyDraftEditTransition(tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("applyDraftEditTransition() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("applyDraftEditTransition() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloseDraftTransition(t *testing.T) {
	t.Parallel()

	got, err := closeDraftTransition(domain.DraftStatusReady, domain.DraftStatusCanceled)
	if err != nil {
		t.Fatalf("closeDraftTransition() error = %v", err)
	}
	if got != domain.DraftStatusCanceled {
		t.Fatalf("closeDraftTransition() = %q, want %q", got, domain.DraftStatusCanceled)
	}

	if _, err := closeDraftTransition(domain.DraftStatusDeleted, domain.DraftStatusCanceled); err == nil {
		t.Fatal("expected terminal draft close to fail")
	}
}

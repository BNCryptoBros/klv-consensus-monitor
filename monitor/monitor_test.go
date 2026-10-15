package monitor

import (
	"testing"

	"github.com/BNCryptoBros/klv-consensus-monitor/models"
	"github.com/BNCryptoBros/klv-consensus-monitor/notify"
)

type statusChange struct {
	oldStatus, newStatus string
}

type fakeChannel struct {
	changes []statusChange
}

func (f *fakeChannel) Name() string { return "fake" }

func (f *fakeChannel) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error {
	f.changes = append(f.changes, statusChange{oldStatus, newStatus})
	return nil
}

func (f *fakeChannel) SendPayday(s notify.Summary) error { return nil }

func (f *fakeChannel) SendFailure(runErr error, submitted int) error { return nil }

func TestUpdateValidatorStatesNotifications(t *testing.T) {
	const blsKey = "bls"

	tests := []struct {
		name                  string
		notifyOnEveryElection bool
		initialStatus         string
		nextStatus            string
		want                  []statusChange
	}{
		{"elected to elected, flag off", false, "elected", "elected", nil},
		{"elected to elected, flag on", true, "elected", "elected", []statusChange{{"elected", "elected"}}},
		{"elected to waiting, flag off", false, "elected", "waiting", []statusChange{{"elected", "waiting"}}},
		{"elected to waiting, flag on", true, "elected", "waiting", []statusChange{{"elected", "waiting"}}},
		{"waiting to waiting, flag on", true, "waiting", "waiting", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &fakeChannel{}
			m := NewMonitor(nil, notify.Group{ch}, []models.MonitoredValidator{{BLSKey: blsKey, DisplayName: "V"}}, tt.notifyOnEveryElection)

			m.updateValidatorStates([]models.ValidatorInfo{{BLSPublicKey: blsKey, ValidatorStatus: tt.initialStatus}}, 1, true)
			if len(ch.changes) != 0 {
				t.Fatalf("initial run sent %d notifications, want 0", len(ch.changes))
			}

			m.updateValidatorStates([]models.ValidatorInfo{{BLSPublicKey: blsKey, ValidatorStatus: tt.nextStatus}}, 2, false)
			if len(ch.changes) != len(tt.want) {
				t.Fatalf("got %d notifications %v, want %v", len(ch.changes), ch.changes, tt.want)
			}
			for i := range tt.want {
				if ch.changes[i] != tt.want[i] {
					t.Errorf("notification %d = %v, want %v", i, ch.changes[i], tt.want[i])
				}
			}
		})
	}
}

package monitor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BNCryptoBros/klv-consensus-monitor/api"
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

const monitoredKey = "bls"

// statistics builds a validator list in which the monitored validator has the
// given status, or is absent when status is StatusMissing. An unrelated
// validator is always present so the list is never empty.
func statistics(status string) []models.ValidatorInfo {
	list := []models.ValidatorInfo{{BLSPublicKey: "other", ValidatorStatus: "eligible"}}
	if status != StatusMissing {
		list = append(list, models.ValidatorInfo{BLSPublicKey: monitoredKey, ValidatorStatus: status})
	}
	return list
}

func newTestMonitor(ch *fakeChannel, apiClient *api.Client, notifyOnEveryElection bool) *Monitor {
	return NewMonitor(apiClient, notify.Group{ch}, []models.MonitoredValidator{{BLSKey: monitoredKey, DisplayName: "V"}}, notifyOnEveryElection)
}

func TestUpdateValidatorStatesNotifications(t *testing.T) {
	tests := []struct {
		name                  string
		notifyOnEveryElection bool
		initialStatus         string
		nextStatuses          []string
		want                  []statusChange
	}{
		{"elected to elected, flag off", false, "elected", []string{"elected"}, nil},
		{"elected to elected, flag on", true, "elected", []string{"elected"}, []statusChange{{"elected", "elected"}}},
		{"elected to waiting, flag off", false, "elected", []string{"waiting"}, []statusChange{{"elected", "waiting"}}},
		{"elected to waiting, flag on", true, "elected", []string{"waiting"}, []statusChange{{"elected", "waiting"}}},
		{"waiting to waiting, flag on", true, "waiting", []string{"waiting"}, nil},
		{"leaves, stays missing, returns", false, "elected", []string{StatusMissing, StatusMissing, "elected"},
			[]statusChange{{"elected", StatusMissing}, {StatusMissing, "elected"}}},
		{"missing at startup, then appears", false, StatusMissing, []string{"elected"}, []statusChange{{StatusMissing, "elected"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &fakeChannel{}
			m := newTestMonitor(ch, nil, tt.notifyOnEveryElection)

			if err := m.updateValidatorStates(statistics(tt.initialStatus), 1, true); err != nil {
				t.Fatalf("initial run: %v", err)
			}
			if len(ch.changes) != 0 {
				t.Fatalf("initial run sent %d notifications, want 0", len(ch.changes))
			}

			for i, status := range tt.nextStatuses {
				if err := m.updateValidatorStates(statistics(status), 2+i, false); err != nil {
					t.Fatalf("epoch %d: %v", 2+i, err)
				}
			}
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

func TestUpdateValidatorStatesRejectsEmptyList(t *testing.T) {
	ch := &fakeChannel{}
	m := newTestMonitor(ch, nil, false)
	if err := m.updateValidatorStates(statistics("elected"), 1, true); err != nil {
		t.Fatalf("initial run: %v", err)
	}

	if err := m.updateValidatorStates(nil, 2, false); err == nil {
		t.Fatal("empty validator list: got nil error, want an error")
	}
	if len(ch.changes) != 0 {
		t.Errorf("empty validator list sent notifications %v, want none", ch.changes)
	}
	if got := m.validatorStates[monitoredKey].Status; got != "elected" {
		t.Errorf("status after empty list = %q, want %q", got, "elected")
	}
}

// fakeNode serves the block and validator statistics endpoints used by the
// monitor. Its fields are only changed between synchronous client calls.
type fakeNode struct {
	epoch  int
	status string
	// statisticsCode makes /validator/statistics fail with this HTTP status.
	statisticsCode int
	// emptyStatistics makes /validator/statistics return an empty list.
	emptyStatistics bool
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1.0/block/list":
		fmt.Fprintf(w, `{"data":{"blocks":[{"epoch":%d}]}}`, n.epoch)
	case "/validator/statistics":
		switch {
		case n.statisticsCode != 0:
			w.WriteHeader(n.statisticsCode)
		case n.emptyStatistics:
			fmt.Fprint(w, `{"data":{"statistics":{}}}`)
		default:
			fmt.Fprintf(w, `{"data":{"statistics":{%q:{"ValidatorStatus":%q}}}}`, monitoredKey, n.status)
		}
	default:
		http.NotFound(w, r)
	}
}

func TestCheckRetriesEpochAfterBadStatistics(t *testing.T) {
	tests := []struct {
		name      string
		breakNode func(n *fakeNode)
	}{
		{"failed fetch", func(n *fakeNode) { n.statisticsCode = http.StatusInternalServerError }},
		{"empty list", func(n *fakeNode) { n.emptyStatistics = true }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &fakeNode{epoch: 1, status: "elected"}
			server := httptest.NewServer(node)
			defer server.Close()

			ch := &fakeChannel{}
			m := newTestMonitor(ch, api.NewClient(server.URL, server.URL), false)
			if err := m.Run(); err != nil {
				t.Fatalf("Run: %v", err)
			}

			node.epoch, node.status = 2, "waiting"
			tt.breakNode(node)
			if err := m.Check(); err == nil {
				t.Fatal("Check with bad statistics: got nil error, want an error")
			}
			if m.currentEpoch != 1 {
				t.Fatalf("currentEpoch after failed check = %d, want 1", m.currentEpoch)
			}
			if len(ch.changes) != 0 {
				t.Fatalf("failed check sent notifications %v, want none", ch.changes)
			}

			node.statisticsCode, node.emptyStatistics = 0, false
			if err := m.Check(); err != nil {
				t.Fatalf("Check after recovery: %v", err)
			}
			if m.currentEpoch != 2 {
				t.Errorf("currentEpoch after recovery = %d, want 2", m.currentEpoch)
			}
			want := []statusChange{{"elected", "waiting"}}
			if len(ch.changes) != 1 || ch.changes[0] != want[0] {
				t.Errorf("notifications = %v, want %v", ch.changes, want)
			}
		})
	}
}

func TestRunRejectsEmptyStatistics(t *testing.T) {
	server := httptest.NewServer(&fakeNode{epoch: 1, emptyStatistics: true})
	defer server.Close()

	m := newTestMonitor(&fakeChannel{}, api.NewClient(server.URL, server.URL), false)
	if err := m.Run(); err == nil {
		t.Fatal("Run with empty statistics: got nil error, want an error")
	}
}

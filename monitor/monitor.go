package monitor

import (
	"fmt"
	"log"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/api"
	"github.com/BNCryptoBros/klv-consensus-monitor/models"
)

type Monitor struct {
	apiClient         *api.Client
	monitoredList     []models.MonitoredValidator
	validatorStates   map[string]*models.ValidatorState
	currentEpoch      int
}

func NewMonitor(apiClient *api.Client, validators []models.MonitoredValidator) *Monitor {
	states := make(map[string]*models.ValidatorState)
	for _, v := range validators {
		states[v.BLSKey] = &models.ValidatorState{
			BLSKey:      v.BLSKey,
			DisplayName: v.DisplayName,
			Status:      "",
			Epoch:       -1,
		}
	}

	return &Monitor{
		apiClient:       apiClient,
		monitoredList:   validators,
		validatorStates: states,
		currentEpoch:    -1,
	}
}

func (m *Monitor) Run() error {
	log.Printf("Starting validator monitoring for %d validators", len(m.monitoredList))

	block, err := m.apiClient.FetchLatestBlock()
	if err != nil {
		return fmt.Errorf("failed to fetch initial block: %w", err)
	}

	m.currentEpoch = block.Epoch
	log.Printf("Initial epoch: %d", m.currentEpoch)

	validators, err := m.apiClient.FetchValidatorList()
	log.Printf("Fetched %d validators from API", len(validators))
	if err != nil {
		return fmt.Errorf("failed to fetch initial validator list: %w", err)
	}

	m.updateValidatorStates(validators, m.currentEpoch, true)

	return nil
}

func (m *Monitor) Check() error {
	block, err := m.apiClient.FetchLatestBlock()
	if err != nil {
		return fmt.Errorf("failed to fetch latest block: %w", err)
	}

	if block.Epoch != m.currentEpoch {
		log.Printf("New epoch detected: %d (previous: %d)", block.Epoch, m.currentEpoch)
		m.currentEpoch = block.Epoch

		validators, err := m.apiClient.FetchValidatorList()
		if err != nil {
			return fmt.Errorf("failed to fetch validator list: %w", err)
		}

		m.updateValidatorStates(validators, block.Epoch, false)
	}

	return nil
}

func (m *Monitor) updateValidatorStates(validators []models.ValidatorInfo, epoch int, isInitial bool) {
	validatorMap := make(map[string]*models.ValidatorInfo)
	for i := range validators {
		validatorMap[validators[i].BLSPublicKey] = &validators[i]
	}

	for blsKey, state := range m.validatorStates {
		validator, found := validatorMap[blsKey]
		if !found {
			log.Printf("[%s] %s - Validator not found in validator list at epoch %d",
				time.Now().Format("2006-01-02 15:04:05"),
				state.DisplayName,
				epoch)
			continue
		}

		previousStatus := state.Status
		currentStatus := validator.ValidatorStatus

		if isInitial {
			state.Status = currentStatus
			state.Epoch = epoch
			log.Printf("[%s] %s - Initial status: %s (Epoch: %d)",
				time.Now().Format("2006-01-02 15:04:05"),
				state.DisplayName,
				currentStatus,
				epoch)
		} else if previousStatus != currentStatus || currentStatus == "elected" {  // elected is always good
			state.Status = currentStatus
			state.Epoch = epoch
			log.Printf("[%s] %s - Status changed: %s → %s (Epoch: %d)",
				time.Now().Format("2006-01-02 15:04:05"),
				state.DisplayName,
				previousStatus,
				currentStatus,
				epoch)
		}
	}
}

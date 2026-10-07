package notify

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const (
	PaydayHeader   = "🎉 PAYDAY! Cash is in the air 💸🥳"
	PaydaySubtitle = "It's payment day, fam — %d transaction(s) just dropped on the multisig. Time to sign and celebrate!"
	MultisignURL   = "https://kleverscan.org/multisign"
)

const (
	klvPrecision          = 6
	klvAtomicUnitsPerUnit = 1_000_000
)

type WalletTotal struct {
	Address  string
	Nickname string
	Amount   int64
}

type Summary struct {
	Submitted  int
	Totals     []WalletTotal
	GrandTotal int64
}

type Channel interface {
	Name() string
	SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error
	SendPayday(s Summary) error
	SendFailure(runErr error, submitted int) error
}

type Group []Channel

func (g Group) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) {
	for _, c := range g {
		if err := c.SendStatusChange(displayName, oldStatus, newStatus, epoch); err != nil {
			log.Printf("Failed to send %s notification: %v", c.Name(), err)
		}
	}
}

func (g Group) SendPayday(s Summary) {
	for _, c := range g {
		if err := c.SendPayday(s); err != nil {
			log.Printf("WARN: failed to send %s payday notification: %v", c.Name(), err)
		}
	}
}

func (g Group) SendFailure(runErr error, submitted int) {
	for _, c := range g {
		if err := c.SendFailure(runErr, submitted); err != nil {
			log.Printf("WARN: failed to send %s failure notification: %v", c.Name(), err)
		}
	}
}

func RenderStatus(template, displayName, oldStatus, newStatus string, epoch int, escape func(string) string) string {
	message := template

	replacements := map[string]string{
		"{{displayName}}": escape(displayName),
		"{{oldStatus}}":   escape(oldStatus),
		"{{newStatus}}":   escape(newStatus),
		"{{epoch}}":       strconv.Itoa(epoch),
	}

	for placeholder, value := range replacements {
		message = strings.ReplaceAll(message, placeholder, value)
	}

	return message
}

type SubmissionError struct {
	Failures int
}

func (e *SubmissionError) Error() string {
	return fmt.Sprintf("%d validator transaction(s) failed", e.Failures)
}

func FailureText(runErr error, submitted int) string {
	var subErr *SubmissionError
	switch {
	case submitted > 0:
		return fmt.Sprintf("Payments run failed: %v. %d transaction(s) were posted to the multisig, check it before re-running.", runErr, submitted)
	case errors.As(runErr, &subErr):
		return fmt.Sprintf("Payments run failed: %v. No transaction was confirmed by the multisig, but a failed post may still have landed, check it before re-running.", runErr)
	default:
		return fmt.Sprintf("Payments run failed: %v. Nothing was posted to the multisig, it is safe to re-run.", runErr)
	}
}

func FormatKLV(atomic int64) string {
	sign := ""
	if atomic < 0 {
		sign = "-"
		atomic = -atomic
	}
	whole := atomic / klvAtomicUnitsPerUnit
	frac := atomic % klvAtomicUnitsPerUnit
	return fmt.Sprintf("%s%d.%0*d", sign, whole, klvPrecision, frac)
}

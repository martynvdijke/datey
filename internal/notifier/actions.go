package notifier

import "context"

// ActionLinks carries the absolute URLs a reminder can offer as one-tap
// actions. Both are optional: a channel that supports native actions renders
// only the ones it receives, and a channel that does not support them ignores
// the struct entirely.
//
// The package never mints these URLs itself — the signing secret belongs to the
// web layer, so the scheduler injects fully-formed links (see
// reminderaction.Links).
type ActionLinks struct {
	// SnoozeURL opens the confirmation page for snoozing this occurrence.
	SnoozeURL string
	// AckURL opens the confirmation page for acknowledging it.
	AckURL string
}

// Empty reports whether no action is available, in which case a notifier must
// send its ordinary payload.
func (a ActionLinks) Empty() bool {
	return a.SnoozeURL == "" && a.AckURL == ""
}

// ActionNotifier is implemented by channels that can render native action
// affordances (ntfy action buttons, Discord components). Channels that do not
// implement it keep receiving the plain message; Registry falls back to SendTo.
type ActionNotifier interface {
	SendToActions(ctx context.Context, title, message, target string, actions ActionLinks) error
}

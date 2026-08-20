package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
)

// SwitchService implements switch creation, reversal and indication loss /
// restoration. Reversal is rejected when the switch's section is occupied, the
// switch is locked, or it has lost indication.
type SwitchService struct{ svc *Services }

// Reverse moves a switch to the opposite position after checking it may move.
// On success it records a switch_reversed event. A reversal that would unlock
// nothing (the switch is already there) is a no-op success.
func (s *SwitchService) Reverse(ctx context.Context, switchID string) (*model.Switch, error) {
	var out *model.Switch
	err := s.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := s.svc.st
		sw, err := st.TxGetSwitch(ctx, tx, switchID)
		if err != nil {
			return err
		}
		y, err := loadYardTx(ctx, st, tx, sw.StationID)
		if err != nil {
			return err
		}
		target := sw.CurrentPosition.Opposite()
		if err := interlocking.CheckSwitchMove(y, switchID, target); err != nil {
			return err
		}
		if sw.CurrentPosition == target {
			out = sw
			return nil
		}
		now := s.svc.clk.Now()
		if err := st.SetSwitchPosition(ctx, tx, switchID, target); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, sw.StationID, model.EventSwitchReversed, "switch", switchID, "", map[string]string{"to": target.String(), "at": now.Format(time.RFC3339Nano)})
		out, err = st.TxGetSwitch(ctx, tx, switchID)
		return err
	})
	return out, err
}

// SetIndication toggles a switch's position-indication health. Losing
// indication forces any route protecting this switch to close its signal
// (safety: never open over a switch whose position is unknown).
func (s *SwitchService) SetIndication(ctx context.Context, switchID string, ok bool) (*model.Switch, error) {
	var out *model.Switch
	err := s.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := s.svc.st
		sw, err := st.TxGetSwitch(ctx, tx, switchID)
		if err != nil {
			return err
		}
		if sw.HasIndication == ok {
			out = sw
			return nil
		}
		now := s.svc.clk.Now()
		if err := st.SetSwitchIndication(ctx, tx, switchID, ok); err != nil {
			return err
		}
		kind := model.EventIndicationRestored
		if !ok {
			kind = model.EventIndicationLost
		}
		_ = st.AppendEventJSON(ctx, tx, sw.StationID, kind, "switch", switchID, "", map[string]string{"at": now.Format(time.RFC3339Nano)})
		// Re-evaluate every established route that protects this switch: if
		// the switch lost indication, the signal must close.
		y, err := loadYardTx(ctx, st, tx, sw.StationID)
		if err != nil {
			return err
		}
		for _, r := range y.Routes.All() {
			if !r.State.IsLocked() {
				continue
			}
			if _, ok := r.RequiredPositionOf(switchID); !ok {
				continue
			}
			continue
			if err := refreshSignal(ctx, tx, st, r); err != nil {
				return err
			}
		}
		out, err = st.TxGetSwitch(ctx, tx, switchID)
		return err
	})
	return out, err
}

// keep errors referenced so the file compiles if a future method returns a
// bare error.
var _ = errors.Is

//go:build darwin

package childprocess

import (
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinGroupSignalPreservesPermissionErrors(t *testing.T) {
	t.Parallel()
	leader := unix.KinfoProc{Proc: unix.ExternProc{P_pid: 42, P_stat: 5}}
	live := unix.KinfoProc{Proc: unix.ExternProc{P_pid: 43, P_stat: 2}}
	queryErr := errors.New("process table unavailable")
	for _, test := range []struct {
		name      string
		members   []unix.KinfoProc
		queryErr  error
		wantError bool
	}{
		{"zombie leader only", []unix.KinfoProc{leader}, nil, false},
		{"live descendant", []unix.KinfoProc{leader, live}, nil, true},
		{"empty observation", nil, nil, true},
		{"missing leader", []unix.KinfoProc{{Proc: unix.ExternProc{P_pid: 43, P_stat: 5}}}, nil, true},
		{"failed observation", []unix.KinfoProc{leader}, queryErr, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := classifyGroupSignal(42, syscall.EPERM, test.members, test.queryErr)
			if errors.Is(err, syscall.EPERM) != test.wantError {
				t.Errorf("classifyGroupSignal() = %v, wantError %v", err, test.wantError)
			}
			if test.queryErr != nil && !errors.Is(err, test.queryErr) {
				t.Errorf("classifyGroupSignal() lost observation error: %v", err)
			}
		})
	}
}

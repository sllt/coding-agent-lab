//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	createRulesetVersion = 1 << 0
	rulePathBeneath      = 1

	accessExecute    = 1 << 0
	accessWriteFile  = 1 << 1
	accessReadFile   = 1 << 2
	accessReadDir    = 1 << 3
	accessRemoveDir  = 1 << 4
	accessRemoveFile = 1 << 5
	accessMakeChar   = 1 << 6
	accessMakeDir    = 1 << 7
	accessMakeReg    = 1 << 8
	accessMakeSock   = 1 << 9
	accessMakeFifo   = 1 << 10
	accessMakeBlock  = 1 << 11
	accessMakeSym    = 1 << 12
	accessRefer      = 1 << 13
	accessTruncate   = 1 << 14
	accessIoctlDev   = 1 << 15

	prSetNoNewPrivs = 38

	// oPath is O_PATH on x86, arm and riscv Linux.
	oPath = 0x200000
)

type rulesetAttr struct {
	handledAccessFS uint64
}

type pathBeneathAttr struct {
	allowedAccess uint64
	parentFd      int32
}

// Probe reports the Landlock ABI version offered by the kernel.
func Probe() Status {
	abi, _, errno := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, createRulesetVersion)
	if errno != 0 {
		return Status{Reason: "landlock_create_ruleset: " + errno.Error()}
	}
	if int(abi) < 1 {
		return Status{Reason: "landlock ABI < 1"}
	}
	return Status{Supported: true, ABI: int(abi)}
}

func handledFor(abi int) uint64 {
	h := uint64(accessExecute | accessWriteFile | accessReadFile | accessReadDir | accessRemoveDir | accessRemoveFile |
		accessMakeChar | accessMakeDir | accessMakeReg | accessMakeSock | accessMakeFifo | accessMakeBlock | accessMakeSym)
	if abi >= 2 {
		h |= accessRefer
	}
	if abi >= 3 {
		h |= accessTruncate
	}
	if abi >= 5 {
		h |= accessIoctlDev
	}
	return h
}

const fileRights = accessExecute | accessWriteFile | accessReadFile | accessTruncate | accessIoctlDev

func applyAndExec(p Policy, exe string, argv, env []string) error {
	st := Probe()
	if !st.Supported {
		return fmt.Errorf("%w: %s", ErrUnsupported, st.Reason)
	}
	// Landlock and no_new_privs are per-thread; keep this goroutine on one
	// thread from restriction through execve.
	runtime.LockOSThread()
	handled := handledFor(st.ABI)
	attr := rulesetAttr{handledAccessFS: handled}
	fd, _, errno := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	ruleset := int(fd)
	defer syscall.Close(ruleset)
	ro := uint64(accessExecute | accessReadFile | accessReadDir)
	add := func(path string, access uint64) error {
		pfd, err := syscall.Open(path, oPath|syscall.O_CLOEXEC, 0)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer syscall.Close(pfd)
		var st syscall.Stat_t
		if err := syscall.Fstat(pfd, &st); err == nil && st.Mode&syscall.S_IFMT != syscall.S_IFDIR {
			access &= fileRights
		}
		access &= handled
		rule := pathBeneathAttr{allowedAccess: access, parentFd: int32(pfd)}
		_, _, e := syscall.Syscall6(sysLandlockAddRule, uintptr(ruleset), rulePathBeneath, uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
		if e != 0 {
			return fmt.Errorf("landlock_add_rule %s: %w", path, e)
		}
		return nil
	}
	for _, path := range p.ReadOnly {
		if err := add(path, ro); err != nil {
			return err
		}
	}
	for _, path := range p.ReadWrite {
		if err := add(path, handled); err != nil {
			return err
		}
	}
	if _, _, e := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0, 0, 0, 0); e != 0 {
		return fmt.Errorf("prctl(no_new_privs): %w", e)
	}
	if _, _, e := syscall.Syscall(sysLandlockRestrictSelf, uintptr(ruleset), 0, 0); e != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", e)
	}
	return syscall.Exec(exe, argv, env)
}

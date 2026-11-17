// Package repository executes bounded local tool requests. Authorization and
// isolated model review belong to the consuming service, not this adapter.
//
// # Restricted grammar
//
// Restricted requests accept only these exact command names and argv forms:
//
//	pwd                  (no arguments)
//	ls                   (root, one level only)
//	ls PATH              (one directory, one level only)
//	cat PATH             (one regular, singly linked file)
//	head PATH            (first 10 LF-delimited lines)
//	head -n N PATH       (N is canonical decimal in [1, 1000])
//
// There is no PATH lookup, subprocess, shell, stdin, globbing, expansion, option
// parser, recursion, multi-path mode, or support for other flags (including --).
// Shell metacharacters in a path are literal filename bytes, never instructions.
// ls emits ASCII Go-quoted filenames in filesystem order, without file metadata;
// it omits symlinks, sensitive names, and platform-ambiguous names.
//
// # Path boundary
//
// Relative paths start at the configured root; absolute paths are accepted only
// inside it. Any lexical .. component is rejected, even if it would remain
// inside. All relative path components are checked, including before cleaning.
// Names beginning with -, containing a colon or non-native backslash, or ending
// in a dot or space are rejected (except .). Symlinks in any component, including
// the configured root and its ancestors, are rejected. New does not canonicalize
// aliases: a composition root may canonicalize its trusted cwd before New, but
// must never canonicalize untrusted tool paths to bypass these checks.
//
// Sensitive matching is case-insensitive, including configured root ancestors:
// .env*; any name containing credential, secret, password, token, privatekey,
// private_key, private-key, or keys; exact .git, .ssh, .aws, .kube, .azure, .gnupg,
// .zev; id_*; and suffixes .pem, .key, .p12, .pfx. This intentionally overblocks
// some innocent names and cannot identify secrets stored under innocent names.
//
// The root is pinned until Close. Each operation verifies its current path still
// names that identity. Descendant directories are opened one component at a time
// through os.Root; opened identities must match pre-open non-symlink metadata.
// File handles are verified before reading. Unix uses O_NOFOLLOW|O_NONBLOCK to
// prevent a replacement FIFO from blocking open. Windows verifies link counts
// and reparse attributes on the opened handle. No lexical check or reviewer
// decision is treated as durable authority: Execute revalidates every request.
//
// # Fixed budgets
//
// Output is at most 32768 bytes combined across stdout and stderr, including
// control-character escaping. C0/C1 controls (except LF and tab), CR, and Unicode
// format controls are escaped; invalid UTF-8 is replaced. Callers should still
// JSON-quote output before terminal rendering. No child output is written to the
// application's stdout/stderr. File reads consume at most 32769 bytes. head may
// read that bounded prefix even when fewer lines suffice. ls examines at most
// 1024 entries (including omitted entries) without sorting or recursion; reaching
// that budget conservatively marks Truncated even at exact EOF. Paths and the
// root are limited to 4096 bytes and 64 components. A request allows 64 args,
// 4096 bytes per executable/arg, 16384 executable+arg bytes total, and a 2048-byte
// Reason. Reason is not executed. Truncated means a budget was encountered, not
// that the underlying data is a consistent filesystem snapshot.
//
// Every Execute receives a ten-second deadline, shortened by its caller. Native
// processes use childprocess.Run, including group/job cleanup and bounded pipe
// draining; cleanup can extend the wall-clock duration beyond the deadline.
// Ordinary filesystem APIs do not support context cancellation of an in-flight
// kernel call. Work and bytes are bounded, with cancellation checks around I/O,
// but a stalled filesystem/FUSE/kernel call cannot be given a hard wall-clock
// bound by this in-process adapter. It must not be advertised as such.
//
// # Approved execution
//
// With restricted=false the caller has already approved the complete request.
// os/exec receives literal argv and cwd=root; shells run only when explicitly
// requested. Stdin is the null device. The environment contains only PATH, LANG=C
// and LC_ALL=C (plus SystemRoot on Windows); no provider secrets, HOME, loader
// hooks, or shell-startup variables are inherited. Bare executables are searched
// only in /usr/bin:/bin, or SystemRoot/System32 on Windows (.exe only). Explicit
// absolute paths or relative paths containing separators allow other tools.
//
// Approved execution is not a sandbox: it may modify files, access the network,
// read credentials from disk, or intentionally detach descendants. os.Root does
// not forbid mount crossings, and this adapter does not protect against hostile
// same-UID writers, mount changes, or concurrent mutation of regular-file data.
package repository

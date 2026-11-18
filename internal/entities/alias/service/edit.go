package service

import "errors"

// ErrAliasChanged reports that an alias changed while its editor was open.
// Use errors.Is to distinguish a stale edit from an editor or storage failure.
var ErrAliasChanged = errors.New("alias changed while editing; open it again")

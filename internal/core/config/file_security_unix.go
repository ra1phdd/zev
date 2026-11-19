//go:build !windows

package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"reflect"
	"strconv"
)

func validateFileOwner(_ string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("configuration file must not be readable or writable by group or others")
	}
	uid, ok := fileOwnerUID(info)
	if !ok {
		return errors.New("configuration owner could not be verified")
	}
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve current configuration owner: %w", err)
	}
	currentUID, err := strconv.ParseUint(current.Uid, 10, 64)
	if err != nil || uid != currentUID {
		return errors.New("configuration file is not owned by the current user")
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName("Uid")
	if !field.IsValid() || !field.CanUint() {
		return 0, false
	}
	return field.Uint(), true
}

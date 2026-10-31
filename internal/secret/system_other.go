//go:build !darwin && !linux

package secret

import "errors"

func system() (Store, error) { return nil, errors.New("secret: no keychain here yet") }

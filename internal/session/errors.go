package session

import "errors"

var ErrConcurrentRefresh = errors.New("refresh token was rotated very recently")

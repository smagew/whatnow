//go:build !darwin && !linux && !windows

package main

import (
	"errors"
	"time"
)

func awakeCan() bool { return false }

func holdAwake(time.Duration) (func(), error) { return nil, errors.New("not on this system") }

//go:build !linux

package main

import (
	"context"
	"errors"
)

func repairDJIQMI(context.Context) ([]djiQMIRepairResult, error) {
	return nil, errors.New("DJI QMI repair is supported only on Linux")
}

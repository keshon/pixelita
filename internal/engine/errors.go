package engine

import (
	"errors"
	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/ops"
	"strings"
)

func ErrorCode(err error) string {
	var p *ops.PlanError
	if errors.As(err, &p) {
		return p.Code
	}
	if errors.Is(err, batch.ErrNoInput) {
		return "no_input"
	}
	if strings.Contains(err.Error(), "unsupported input") {
		return "invalid_input"
	}
	return "invalid_request"
}

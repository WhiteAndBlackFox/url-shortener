package logger

import "go.uber.org/zap"

// New builds a structured JSON logger. In production it logs at Info level and
// above; go.uber.org/zap already encodes fields (not free-form strings), which
// is what "structured logging" means in practice.
func New(debug bool) (*zap.Logger, error) {
	if debug {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

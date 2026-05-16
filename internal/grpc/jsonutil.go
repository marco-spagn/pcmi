package grpcserver

import (
	"encoding/json"
	"fmt"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
)

func toJSONResponse(v any) (*pcmiv1.JSONResponse, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return &pcmiv1.JSONResponse{Json: string(b)}, nil
}

package worker

import (
	"context"
)

// nextDistilledVersion returns the next version for tenant+distilled path (1 if none).
func nextDistilledVersion(ctx context.Context, db workerDB, tenantID, distilledPath string) (int, error) {
	var maxVer int
	err := db.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0)
		FROM distilled_knowledge
		WHERE tenant_id = $1::uuid AND path = $2::ltree`,
		tenantID, distilledPath,
	).Scan(&maxVer)
	if err != nil {
		return 0, err
	}
	return maxVer + 1, nil
}

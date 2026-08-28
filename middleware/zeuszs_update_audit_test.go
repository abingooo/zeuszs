package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZeusZSUpdateRouteHasStableAuditAction(t *testing.T) {
	assert.Equal(t, "zeuszs.update_trigger", auditRouteActions["POST /api/zeuszs/update"])
}
